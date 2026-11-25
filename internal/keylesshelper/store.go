// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/signingstore"
)

// withKeyStoreLockContext is not reentrant; locked callbacks use private helpers.
func withKeyStoreLockContext(ctx context.Context, fn func() error) error {
	directory, err := signerStorageDir()
	if err != nil {
		return err
	}
	return signingstore.WithStoreLock(ctx, filepath.Join(directory, "keysigner", "key_store.lock"), fn)
}

// KeyStore selects a signing backend for new private_key_jwt_local_keypair registrations.
type KeyStore struct {
	signers []keysigner.Signer
}

// NewKeyStore uses the supplied signing backends in order.
func NewKeyStore(signers ...keysigner.Signer) *KeyStore {
	return &KeyStore{signers: append([]keysigner.Signer(nil), signers...)}
}

// Key holds the selected provider and a snapshot of its public key.
type Key struct {
	id       string
	public   crypto.PublicKey
	signer   keysigner.Signer
	provider string
}

func newKey(id string, public crypto.PublicKey, signer keysigner.Signer) (*Key, error) {
	if _, err := keysigner.AlgForKey(public); err != nil {
		return nil, err
	}
	// PKIX round-tripping copies both RSA and EC public keys without sharing
	// mutable big.Int values with the backend.
	encoded, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	cloned, err := x509.ParsePKIXPublicKey(encoded)
	if err != nil {
		return nil, err
	}
	return &Key{id: id, public: cloned, signer: signer, provider: signer.Name()}, nil
}

func (k *Key) ID() string {
	return k.id
}
func (k *Key) Provider() string {
	return k.provider
}
func (k *Key) Signer() keysigner.Signer {
	return k.signer
}
func (k *Key) Thumbprint() (string, error) {
	if k == nil || k.public == nil {
		return "", keysigner.ErrKeyNotFound
	}
	return keysigner.PublicKeyThumbprint(k.public)
}

func (s *KeyStore) ensureContext(ctx context.Context, id string) (*Key, error) {
	if id == "" {
		return nil, fmt.Errorf("keyless: empty key identifier")
	}
	if s == nil || len(s.signers) == 0 {
		return nil, keysigner.ErrUnavailable
	}
	signer, public, err := keysigner.EnsureKeyWithFallback(ctx, s.signers, keysigner.KeyRef{Label: id})
	if err != nil {
		return nil, err
	}
	key, err := newKey(id, public, signer)
	if err != nil {
		cleanupCtx := ctx
		if cleanupCtx == nil {
			cleanupCtx = context.Background()
		} else {
			cleanupCtx = context.WithoutCancel(cleanupCtx)
		}
		return nil, errors.Join(err, signer.DeleteKey(cleanupCtx, keysigner.KeyRef{Label: id}))
	}
	return key, nil
}

func (k *Key) DeleteContext(ctx context.Context) error {
	if k == nil || k.id == "" || k.signer == nil {
		return nil
	}
	return deleteKeyContext(ctx, k.signer, k.id)
}

func deleteKeyContext(ctx context.Context, signer keysigner.Signer, id string) error {
	return withKeyStoreLockContext(ctx, func() error {
		err := signer.DeleteKey(ctx, keysigner.KeyRef{Label: id})
		if errors.Is(err, keysigner.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("delete keyless key with signer %q: %w", signer.Name(), err)
		}
		return nil
	})
}

func (s *KeyStore) ProbeWritableContext(ctx context.Context) error {
	if s == nil || len(s.signers) == 0 {
		return keysigner.ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return withKeyStoreLockContext(ctx, func() error {
		return keysigner.ProbeSigners(ctx, s.signers, func(signer keysigner.Signer) error {
			label, err := keysigner.NewKeyLabel("larksuite-cli-probe-")
			if err != nil {
				return err
			}
			return keysigner.ProbeSigner(ctx, signer, keysigner.KeyRef{Label: label, Algorithm: keysigner.AlgES256}, []byte("lark-cli key signer probe"))
		})
	})
}

// CreateAttestationContext holds one lock across backend selection, key creation
// and signing. It returns the key on signing failure so the caller can clean up.
func (s *KeyStore) CreateAttestationContext(ctx context.Context, id, nonce string, now time.Time) (key *Key, assertion string, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.ensureContext(ctx, id)
		if err != nil {
			return err
		}
		assertion, err = jwt.SignAttestation(ctx, key.signer, keysigner.KeyRef{Label: id}, key.public, nonce, now)
		return err
	})
	return key, assertion, err
}

// SignClientAssertionContext signs with the bound signer under the managed key store lock.
func SignClientAssertionContext(ctx context.Context, signer keysigner.Signer, id, clientID, audience string, now time.Time) (assertion string, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		if id == "" {
			return fmt.Errorf("keyless: empty key identifier")
		}
		if signer == nil {
			return keysigner.ErrUnavailable
		}
		var err error
		assertion, err = jwt.SignClientAssertion(ctx, signer, keysigner.KeyRef{Label: id}, clientID, audience, now)
		return err
	})
	return assertion, err
}

// ProbeKeyContext checks that the bound signer can sign with the existing key.
func ProbeKeyContext(ctx context.Context, signer keysigner.Signer, id string) (key *Key, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		if id == "" {
			return fmt.Errorf("keyless: empty key identifier")
		}
		if signer == nil {
			return keysigner.ErrUnavailable
		}
		ref := keysigner.KeyRef{Label: id}
		public, err := signer.PublicKey(ctx, ref)
		if err != nil {
			return fmt.Errorf("open keyless key with signer %q: %w", signer.Name(), err)
		}
		key, err = newKey(id, public, signer)
		if err != nil {
			return err
		}
		ref.Algorithm, err = keysigner.AlgForKey(key.public)
		if err != nil {
			return err
		}
		_, signedAlgorithm, err := signer.Sign(ctx, ref, []byte("lark-cli signer health check"))
		if err != nil {
			return err
		}
		if signedAlgorithm != ref.Algorithm {
			return fmt.Errorf("keyless: signer returned %s, want %s", signedAlgorithm, ref.Algorithm)
		}
		return nil
	})
	return key, err
}
