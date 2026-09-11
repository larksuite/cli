//go:build linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"context"
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
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing-key lookup created local state: %v", err)
	}
	directoryErr = errors.New("directory unavailable")
	if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, directoryErr) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("directory failure = %v, want original hard error", err)
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
