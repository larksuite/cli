//go:build linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestTPMSignerUsesCallerDirectoryWithoutAccessingTPMForMissingKeys(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "keys")
	calls := 0
	var directoryErr error
	signer := NewSigner(LinuxTPMSignerName, func(name string) (string, error) {
		if name != LinuxTPMSignerName {
			t.Fatalf("directory resolver received signer name %q", name)
		}
		calls++
		return directory, directoryErr
	})
	if signer == nil || signer.Name() != "linux-tpm" || signer.SecurityLevel() != SecurityLevelL1 {
		t.Fatalf("unexpected Linux TPM signer: %v", signer)
	}
	if calls != 0 {
		t.Fatal("constructing a signer resolved its directory")
	}
	ref := KeyRef{Label: "caller-owned"}
	if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("missing key = %v, want ErrKeyNotFound before accessing the TPM", err)
	}
	if calls != 1 {
		t.Fatalf("directory resolutions = %d, want 1", calls)
	}
	if err := signer.DeleteKey(context.Background(), ref); err != nil {
		t.Fatalf("deleting a missing key = %v, want nil", err)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-key lookup created local state: %v", err)
	}
	directoryErr = errors.New("directory unavailable")
	if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, directoryErr) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("directory failure = %v, want original hard error", err)
	}
}

func TestTPMKeyRejectsInvalidSigningInputsBeforeDeviceAccess(t *testing.T) {
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := &tpmKey{public: &private.PublicKey}
	for _, tc := range []struct {
		name   string
		digest []byte
		opts   crypto.SignerOpts
	}{
		{name: "nil options", digest: make([]byte, crypto.SHA256.Size())},
		{name: "wrong hash", digest: make([]byte, crypto.SHA512.Size()), opts: crypto.SHA512},
		{name: "short digest", digest: make([]byte, crypto.SHA256.Size()-1), opts: crypto.SHA256},
		{name: "PSS", digest: make([]byte, crypto.SHA256.Size()), opts: &rsa.PSSOptions{Hash: crypto.SHA256}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := key.Sign(nil, tc.digest, tc.opts); !errors.Is(err, ErrUnsupportedAlgorithm) {
				t.Fatalf("Sign() error = %v, want ErrUnsupportedAlgorithm", err)
			}
		})
	}
	key.public = struct{}{}
	if _, err := key.Sign(nil, make([]byte, crypto.SHA256.Size()), crypto.SHA256); !errors.Is(err, ErrUnsupportedAlgorithm) {
		t.Fatalf("unsupported public key error = %v", err)
	}
}

func TestTPMErrorClassificationOnlyPermitsKnownFallbacks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		path        string
		cause       error
		unavailable bool
	}{
		{"missing TPM", "/dev/tpmrm0", syscall.ENOENT, true},
		{"permission denied", "/dev/tpmrm0", syscall.EACCES, true},
		{"operation not permitted", "/dev/tpmrm0", syscall.EPERM, true},
		{"I/O failure", "/dev/tpmrm0", syscall.EIO, false},
		{"missing key file", "/keys/test-key", syscall.ENOENT, false},
		{"key file permission denied", "/keys/test-key", syscall.EACCES, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pathErr := &fs.PathError{Op: "open", Path: tc.path, Err: tc.cause}
			wrapped := fmt.Errorf("native backend: %w", pathErr)
			err := classifyTPMError(wrapped)
			if !errors.Is(err, wrapped) || !errors.Is(err, tc.cause) {
				t.Fatal("classification lost native error")
			}
			if errors.Is(err, ErrUnavailable) != tc.unavailable || errors.Is(err, ErrKeyNotFound) {
				t.Fatalf("classification = %v, want unavailable=%v without rebinding", err, tc.unavailable)
			}
		})
	}
}
