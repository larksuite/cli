// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
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
	t.Run("long validated path", func(t *testing.T) {
		path := filepath.Join(strings.Repeat("a", 128), strings.Repeat("b", 128), "private.pem")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		der, err := x509.MarshalECPrivateKey(rotatedKey)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, alg, err := signer.Sign(context.Background(), keysigner.KeyRef{Label: path}, input); err != nil || alg != keysigner.AlgES256 {
			t.Fatalf("long imported-key path: algorithm=%q error=%v", alg, err)
		}
	})
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

func TestFileSignerAlgorithms(t *testing.T) {
	chdirForFileSignerTest(t)
	for _, tc := range []struct {
		algorithm string
		curve     elliptic.Curve
	}{
		{keysigner.AlgES384, elliptic.P384()},
		{keysigner.AlgES512, elliptic.P521()},
		{keysigner.AlgEdDSA, nil},
		{keysigner.AlgRS256, nil},
	} {
		t.Run(tc.algorithm, func(t *testing.T) {
			var key crypto.Signer
			var err error
			switch tc.algorithm {
			case keysigner.AlgEdDSA:
				_, key, err = ed25519.GenerateKey(rand.Reader)
			case keysigner.AlgRS256:
				key, err = rsa.GenerateKey(rand.Reader, 2048)
			default:
				key, err = ecdsa.GenerateKey(tc.curve, rand.Reader)
			}
			if err != nil {
				t.Fatal(err)
			}
			der, err := x509.MarshalPKCS8PrivateKey(key)
			if err != nil {
				t.Fatal(err)
			}
			path := tc.algorithm + ".pem"
			if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
				t.Fatal(err)
			}
			signer := &FileSigner{}
			input := []byte("header.claims")
			for _, requested := range []string{"", tc.algorithm} {
				ref := keysigner.KeyRef{Label: path, Algorithm: requested}
				public, err := signer.PublicKey(context.Background(), ref)
				if err != nil {
					t.Fatal(err)
				}
				sig, alg, err := signer.Sign(context.Background(), ref, input)
				if err != nil || alg != tc.algorithm {
					t.Fatalf("requested %q: algorithm=%q error=%v", requested, alg, err)
				}
				switch public := public.(type) {
				case *ecdsa.PublicKey:
					var digest []byte
					if alg == keysigner.AlgES384 {
						sum := sha512.Sum384(input)
						digest = sum[:]
					} else {
						sum := sha512.Sum512(input)
						digest = sum[:]
					}
					size := (public.Curve.Params().BitSize + 7) / 8
					if len(sig) != 2*size || !ecdsa.Verify(public, digest, new(big.Int).SetBytes(sig[:size]), new(big.Int).SetBytes(sig[size:])) {
						t.Fatal("ECDSA JOSE signature failed independent verification")
					}
				case ed25519.PublicKey:
					if !ed25519.Verify(public, input, sig) {
						t.Fatal("EdDSA signature failed independent verification")
					}
				case *rsa.PublicKey:
					digest := sha256.Sum256(input)
					if err := rsa.VerifyPKCS1v15(public, crypto.SHA256, digest[:], sig); err != nil {
						t.Fatal(err)
					}
				}
			}
			ref := keysigner.KeyRef{Label: path, Algorithm: keysigner.AlgES256}
			if _, err := signer.PublicKey(context.Background(), ref); !errors.Is(err, keysigner.ErrUnsupportedAlgorithm) {
				t.Fatalf("PublicKey accepted algorithm mismatch: %v", err)
			}
			if _, _, err := signer.Sign(context.Background(), ref, input); !errors.Is(err, keysigner.ErrUnsupportedAlgorithm) {
				t.Fatalf("Sign accepted algorithm mismatch: %v", err)
			}
		})
	}
}
