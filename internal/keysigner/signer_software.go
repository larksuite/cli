// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"golang.org/x/crypto/argon2"

	"github.com/larksuite/cli/internal/validate"
)

// The version-1 format fixes KDF cost before reading any untrusted data.
const softwareKDF = "argon2id-m65536-t3-p1-aes256gcm"

type softwareSigner struct {
	directory string
	unlock    func(context.Context) ([]byte, error)
}

type softwareEnvelope struct {
	KDF        string `json:"kdf"`
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// NewSoftwareSigner explicitly enables L3 in a caller-selected local directory.
// Unlock returns a fresh 16..1024-byte secret buffer, cleared after derivation.
// Use a strong passphrase or random secret kept apart from key files.
// No system key store, environment secret, or automatic L3 fallback is used.
func NewSoftwareSigner(directory string, unlock func(context.Context) ([]byte, error)) (Signer, error) {
	if unlock == nil {
		return nil, ErrUnlockRequired
	}
	directory, err := validate.SafeEnvDirPath(directory, "software signer directory")
	if err != nil {
		return nil, err
	}
	return &softwareSigner{directory: directory, unlock: unlock}, nil
}

func (*softwareSigner) Name() string                 { return SoftwareSignerName }
func (*softwareSigner) SecurityLevel() SecurityLevel { return SecurityLevelL3 }

func (s *softwareSigner) EnsureKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error) {
	return s.ensureKey(ctx, ref, false)
}

func (s *softwareSigner) CreateKey(ctx context.Context, ref KeyRef) (crypto.PublicKey, error) {
	return s.ensureKey(ctx, ref, true)
}

func (s *softwareSigner) ensureKey(ctx context.Context, ref KeyRef, createOnly bool) (public crypto.PublicKey, err error) {
	err = withKeyFile(ctx, s.directory, ref, func(path string, algorithm signingAlgorithm) error {
		if err := keyFileAbsent(path); err != nil {
			if createOnly || !errors.Is(err, ErrKeyExists) {
				return err
			}
			private, err := s.loadPrivate(ctx, path, ref, algorithm)
			if err != nil {
				return err
			}
			defer algorithm.clearPrivateKey(private)
			public = private.Public()
			return nil
		}
		private, err := algorithm.generateKey()
		if err != nil {
			return err
		}
		defer algorithm.clearPrivateKey(private)
		der, err := x509.MarshalPKCS8PrivateKey(private)
		if err != nil {
			return err
		}
		defer clear(der)
		publicDER, err := x509.MarshalPKIXPublicKey(private.Public())
		if err != nil {
			return err
		}
		record := keyFileRecord{Version: 1, Label: ref.Label, Backend: s.Name(), PublicKey: publicDER}
		envelope := softwareEnvelope{KDF: softwareKDF, Salt: make([]byte, 16), Nonce: make([]byte, 12)}
		if _, err := rand.Read(envelope.Salt); err != nil {
			return err
		}
		if _, err := rand.Read(envelope.Nonce); err != nil {
			return err
		}
		aead, err := s.fileCipher(ctx, envelope.Salt)
		if err != nil {
			return err
		}
		aad, err := record.aad()
		if err != nil {
			return err
		}
		envelope.Ciphertext = aead.Seal(nil, envelope.Nonce, der, aad)
		record.Data, err = json.Marshal(envelope)
		if err != nil {
			return err
		}
		if err := writeKeyFile(ctx, path, record); err != nil {
			return err
		}
		public = private.Public()
		return nil
	})
	return public, err
}

func (s *softwareSigner) PublicKey(ctx context.Context, ref KeyRef) (public crypto.PublicKey, err error) {
	err = withKeyFile(ctx, s.directory, ref, func(path string, algorithm signingAlgorithm) error {
		private, err := s.loadPrivate(ctx, path, ref, algorithm)
		if err != nil {
			return err
		}
		defer algorithm.clearPrivateKey(private)
		public = private.Public()
		return nil
	})
	return public, err
}

func (s *softwareSigner) Sign(ctx context.Context, ref KeyRef, input []byte) (signature []byte, algorithm string, err error) {
	err = withKeyFile(ctx, s.directory, ref, func(path string, selected signingAlgorithm) error {
		private, err := s.loadPrivate(ctx, path, ref, selected)
		if err != nil {
			return err
		}
		defer selected.clearPrivateKey(private)
		algorithm = selected.name()
		signature, err = selected.sign(private, input)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return signature, algorithm, nil
}

func (s *softwareSigner) DeleteKey(ctx context.Context, ref KeyRef) error {
	return withKeyFile(ctx, s.directory, ref, func(path string, algorithm signingAlgorithm) error {
		if _, err := readKeyFile(path, ref, s.Name(), algorithm); err != nil {
			if errors.Is(err, ErrKeyNotFound) {
				return nil
			}
			return err
		}
		return removeKeyFile(path)
	})
}

func (s *softwareSigner) loadPrivate(ctx context.Context, path string, ref KeyRef, algorithm signingAlgorithm) (crypto.Signer, error) {
	record, err := readKeyFile(path, ref, s.Name(), algorithm)
	if err != nil {
		return nil, err
	}
	var envelope softwareEnvelope
	if err := decodeKeyJSON(record.Data, &envelope); err != nil {
		return nil, err
	}
	// ponytail: 2 KiB covers the P-256 and RSA-2048 keys we generate; review
	// this bound before adding larger generated keys. The file stays version 1.
	if envelope.KDF != softwareKDF || len(envelope.Salt) != 16 || len(envelope.Nonce) != 12 ||
		len(envelope.Ciphertext) < 16 || len(envelope.Ciphertext) > 2048 {
		return nil, ErrCorrupt
	}
	aead, err := s.fileCipher(ctx, envelope.Salt)
	if err != nil {
		return nil, err
	}
	aad, err := record.aad()
	if err != nil {
		return nil, err
	}
	der, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnlock, err)
	}
	defer clear(der)
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	private, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, ErrCorrupt
	}
	if err := algorithm.validatePublicKey(private.Public()); err != nil {
		algorithm.clearPrivateKey(private)
		return nil, ErrCorrupt
	}
	public, err := x509.MarshalPKIXPublicKey(private.Public())
	if err != nil || !slices.Equal(public, record.PublicKey) {
		algorithm.clearPrivateKey(private)
		return nil, ErrCorrupt
	}
	return private, nil
}

func (s *softwareSigner) fileCipher(ctx context.Context, salt []byte) (cipher.AEAD, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	secret, err := s.unlock(ctx)
	defer clear(secret)
	if err != nil {
		return nil, err
	}
	if len(secret) < 16 || len(secret) > 1024 {
		return nil, ErrUnlockRequired
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	derived := argon2.IDKey(secret, salt, 3, 64*1024, 1, 32)
	defer clear(derived)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(derived)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
