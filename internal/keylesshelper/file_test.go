// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keysigner"
)

func writeECPrivateKey(t *testing.T, mode os.FileMode) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	path := "private.pem"
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), mode); err != nil {
		t.Fatal(err)
	}
	return path, key
}

func TestFileSignerRejectsUnsafeFiles(t *testing.T) {
	chdirForFileSignerTest(t)
	path, _ := writeECPrivateKey(t, 0o600)
	if err := os.WriteFile("invalid.pem", []byte("not a private key"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("oversized.pem", make([]byte, maxPrivateKeyFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".", "invalid.pem", "oversized.pem", "missing.pem"} {
		t.Run(name, func(t *testing.T) {
			_, err := (&FileSigner{}).PublicKey(context.Background(), keysigner.KeyRef{Label: name})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryConfig || problem.Subtype != errs.SubtypeInvalidConfig {
				t.Fatalf("PublicKey(%q) = %v, want config/invalid_config", name, err)
			}
		})
	}
	t.Run("denylisted symlink", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, "private.pem")
		if err := os.WriteFile(target, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, "linked.pem"); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := (&FileSigner{}).PublicKey(context.Background(), keysigner.KeyRef{Label: "linked.pem"}); err == nil {
			t.Fatal("signer read a denylisted target through a symlink")
		}
	})
}

func chdirForFileSignerTest(t *testing.T) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Errorf("restore cwd: %v", err)
		}
	})
}

func TestFileSignerES256(t *testing.T) {
	chdirForFileSignerTest(t)
	path, key := writeECPrivateKey(t, 0o600)
	signer := &FileSigner{}
	ref := keysigner.KeyRef{Label: path}
	pub, err := signer.PublicKey(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if pub.(*ecdsa.PublicKey).X.Cmp(key.X) != 0 {
		t.Fatal("public key does not match file")
	}
	input := []byte("header.claims")
	sig, alg, err := signer.Sign(context.Background(), ref, input)
	if err != nil {
		t.Fatal(err)
	}
	if alg != keysigner.AlgES256 || len(sig) != 64 {
		t.Fatalf("signature alg=%q len=%d", alg, len(sig))
	}
	digest := sha256.Sum256(input)
	if !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("file signature did not verify")
	}

	// A reference observes the current file, including rotation and removal.
	_, rotatedKey := writeECPrivateKey(t, 0o600)
	sig, alg, err = signer.Sign(context.Background(), ref, input)
	if err != nil {
		t.Fatal(err)
	}
	if alg != keysigner.AlgES256 ||
		!ecdsa.Verify(&rotatedKey.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signer did not read the replacement private key")
	}
	if err := signer.DeleteKey(context.Background(), ref); !errors.Is(err, ErrLifecycleNotSupported) {
		t.Fatalf("DeleteKey = %v, want unsupported for a user-owned key", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("DeleteKey changed the user-owned file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := signer.Sign(context.Background(), ref, input); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing key error = %v, want preserved not-exist cause", err)
	}
}
