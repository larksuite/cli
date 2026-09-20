// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	maxPrivateKeyFileBytes = 1 << 20
	FileSignerName         = "file"
)

var ErrLifecycleNotSupported = errors.New("signing key lifecycle operation is unsupported")

// FileSigner reads a host-local credential referenced by keysigner.KeyRef.Label
// through vfs. It is independent of workspace FileIO and never copies the key.
type FileSigner struct{}

func (s *FileSigner) load(ref keysigner.KeyRef) (crypto.Signer, error) {
	data, err := readPrivateKeyData(ref.Label)
	if err != nil {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig, "cannot read private key file: %v", err).WithCause(err)
	}
	key, err := parsePrivateKey(data)
	if err != nil {
		return nil, errs.NewConfigError(errs.SubtypeInvalidConfig, "invalid private key file: %v", err).WithCause(err)
	}
	return key, nil
}

func (s *FileSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	return s.PublicKey(ctx, ref)
}

func (*FileSigner) Name() string { return FileSignerName }

func (*FileSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

func (s *FileSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if ref.Label == "" {
		return nil, fmt.Errorf("keysigner: private key file path is empty")
	}
	key, err := s.load(ref)
	if err != nil {
		return nil, err
	}
	public := key.Public()
	if err := validateRequestedAlgorithm(ref, public); err != nil {
		return nil, err
	}
	return public, nil
}

func (s *FileSigner) Sign(ctx context.Context, ref keysigner.KeyRef, signingInput []byte) ([]byte, string, error) {
	if ctx != nil && ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if ref.Label == "" {
		return nil, "", fmt.Errorf("keysigner: private key file path is empty")
	}
	key, err := s.load(ref)
	if err != nil {
		return nil, "", err
	}
	if err := validateRequestedAlgorithm(ref, key.Public()); err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(signingInput)
	switch k := key.(type) {
	case *rsa.PrivateKey:
		sig, err := rsa.SignPKCS1v15(rand.Reader, k, crypto.SHA256, digest[:])
		if err != nil {
			return nil, "", fmt.Errorf("keysigner: sign with RSA private key file: %w", err)
		}
		return sig, keysigner.AlgRS256, nil
	case *ecdsa.PrivateKey:
		alg, err := keysigner.AlgForKey(&k.PublicKey)
		if err != nil {
			return nil, "", err
		}
		var ecDigest []byte
		switch alg {
		case keysigner.AlgES256:
			ecDigest = digest[:]
		case keysigner.AlgES384:
			sum := sha512.Sum384(signingInput)
			ecDigest = sum[:]
		case keysigner.AlgES512:
			sum := sha512.Sum512(signingInput)
			ecDigest = sum[:]
		}
		der, err := ecdsa.SignASN1(rand.Reader, k, ecDigest)
		if err != nil {
			return nil, "", fmt.Errorf("keysigner: sign with EC private key file: %w", err)
		}
		sig, err := keysigner.ECDSASignatureToJOSE(&k.PublicKey, der)
		if err != nil {
			return nil, "", err
		}
		return sig, alg, nil
	case ed25519.PrivateKey:
		sig, err := k.Sign(rand.Reader, signingInput, crypto.Hash(0))
		if err != nil {
			return nil, "", fmt.Errorf("keysigner: sign with Ed25519 private key file: %w", err)
		}
		return sig, keysigner.AlgEdDSA, nil
	default:
		return nil, "", fmt.Errorf("keysigner: unsupported private key type %T", key)
	}
}

func (s *FileSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	if ref.Label == "" {
		return fmt.Errorf("keysigner: private key file path is empty")
	}
	return ErrLifecycleNotSupported
}

func validateRequestedAlgorithm(ref keysigner.KeyRef, public crypto.PublicKey) error {
	if ref.Algorithm == "" {
		return nil
	}
	algorithm, err := keysigner.AlgForKey(public)
	if err != nil {
		return err
	}
	if ref.Algorithm != algorithm {
		return fmt.Errorf("%w: requested %s for %s key", keysigner.ErrUnsupportedAlgorithm, ref.Algorithm, algorithm)
	}
	return nil
}

// ResolvePrivateKeyFilePath validates a local credential path and resolves it
// before persistence so changing the working directory cannot change the key.
func ResolvePrivateKeyFilePath(path string) (string, error) {
	if _, err := validate.LocalInputPath(path); err != nil {
		return "", err
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		dir, err := vfs.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
	}
	if !filepath.IsAbs(path) {
		if filepath.VolumeName(path) != "" {
			return "", fmt.Errorf("private key path must not be drive-relative")
		}
		dir, err := vfs.Getwd()
		if err != nil {
			return "", err
		}
		path = filepath.Join(dir, path)
	}
	resolved, err := vfs.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return validate.LocalInputPath(resolved)
}

func readPrivateKeyData(path string) ([]byte, error) {
	path, err := ResolvePrivateKeyFilePath(path)
	if err != nil {
		return nil, fmt.Errorf("keysigner: resolve private key file: %w", err)
	}
	info, err := vfs.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("keysigner: stat private key file: %w", err)
	}
	if err := validatePrivateKeyFileInfo(info.Size(), info.Mode()); err != nil {
		return nil, err
	}
	f, err := vfs.OpenFile(path, privateKeyOpenFlags, 0)
	if err != nil {
		return nil, fmt.Errorf("keysigner: open private key file: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("keysigner: stat opened private key file: %w", err)
	}
	if err := validatePrivateKeyFileInfo(opened.Size(), opened.Mode()); err != nil {
		return nil, err
	}
	return readBoundedPrivateKey(f)
}

func validatePrivateKeyFileInfo(size int64, mode fs.FileMode) error {
	if !mode.IsRegular() || size <= 0 || size > maxPrivateKeyFileBytes {
		return fmt.Errorf("keysigner: private key file must be a non-empty regular file no larger than %d bytes", maxPrivateKeyFileBytes)
	}
	return nil
}

func readBoundedPrivateKey(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxPrivateKeyFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("keysigner: read private key file: %w", err)
	}
	if len(data) > maxPrivateKeyFileBytes {
		return nil, fmt.Errorf("keysigner: private key file exceeds %d bytes", maxPrivateKeyFileBytes)
	}
	return data, nil
}

func parsePrivateKey(data []byte) (crypto.Signer, error) {
	block, rest := pem.Decode(data)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("keysigner: private key file must contain exactly one PEM block")
	}

	var (
		parsed any
		err    error
	)
	switch block.Type {
	case "PRIVATE KEY":
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		parsed, err = x509.ParseECPrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("keysigner: unsupported PEM block type %q", block.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("keysigner: parse private key file: %w", err)
	}
	key, ok := parsed.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("keysigner: parsed private key type %T cannot sign", parsed)
	}
	if rsaKey, ok := key.(*rsa.PrivateKey); ok && rsaKey.N.BitLen() < 2048 {
		return nil, fmt.Errorf("keysigner: RSA private key must be at least 2048 bits")
	}
	if _, err := keysigner.AlgForKey(key.Public()); err != nil {
		return nil, err
	}
	return key, nil
}
