// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const testUnlockSecret = "placeholder-placeholder"

func TestSoftwareSignerLifecycle(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	unlock := func(context.Context) ([]byte, error) { return []byte(testUnlockSecret), nil }
	signer, err := NewSoftwareSigner(dir, unlock)
	if err != nil {
		t.Fatal(err)
	}
	if signer.Name() != SoftwareSignerName || signer.SecurityLevel() != SecurityLevelL3 {
		t.Fatalf("unexpected signer identity: %s/%s", signer.Name(), signer.SecurityLevel())
	}

	for _, ref := range []KeyRef{
		{Label: "es256"},
		{Label: "rs256", Algorithm: AlgRS256},
	} {
		t.Run(ref.Label, func(t *testing.T) {
			public, err := signer.EnsureKey(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := NewSoftwareSigner(dir, unlock)
			if err != nil {
				t.Fatal(err)
			}
			got, err := reopened.PublicKey(ctx, ref)
			if err != nil || !publicKeysEqual(public, got) {
				t.Fatalf("reopened key changed identity: %v", err)
			}
			input := []byte("header.payload")
			signature, algorithm, err := reopened.Sign(ctx, ref, input)
			if err != nil {
				t.Fatal(err)
			}
			verifySignature(t, public, algorithm, input, signature)
			if _, err := reopened.(KeyCreator).CreateKey(ctx, ref); !errors.Is(err, ErrKeyExists) {
				t.Fatalf("duplicate creation: %v", err)
			}

			wrong := KeyRef{Label: ref.Label, Algorithm: AlgRS256}
			if ref.Algorithm == AlgRS256 {
				wrong.Algorithm = AlgES256
			}
			if _, err := reopened.PublicKey(ctx, wrong); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("algorithm substitution: %v", err)
			}
			if err := reopened.DeleteKey(ctx, ref); err != nil {
				t.Fatal(err)
			}
			if _, _, err := reopened.Sign(ctx, ref, input); !errors.Is(err, ErrKeyNotFound) {
				t.Fatalf("missing key was recreated: %v", err)
			}
			if files, err := os.ReadDir(dir); err != nil || len(files) != 0 {
				t.Fatalf("key lifecycle left files behind: %v, err = %v", files, err)
			}
		})
	}
}

func TestSoftwareSignerRejectsKeyFileTampering(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	unlock := func(context.Context) ([]byte, error) { return []byte(testUnlockSecret), nil }
	signer, err := NewSoftwareSigner(dir, unlock)
	if err != nil {
		t.Fatal(err)
	}
	ref := KeyRef{Label: "tamper-test"}
	if _, err := signer.EnsureKey(ctx, ref); err != nil {
		t.Fatal(err)
	}
	path := keyFilePath(dir, ref.Label)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record keyFileRecord
	if err := json.Unmarshal(original, &record); err != nil {
		t.Fatal(err)
	}
	var envelope softwareEnvelope
	if err := json.Unmarshal(record.Data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.KDF != softwareKDF || len(envelope.Salt) != 16 || len(envelope.Nonce) != 12 {
		t.Fatalf("unexpected encrypted envelope: %+v", envelope)
	}
	if _, err := x509.ParsePKCS8PrivateKey(envelope.Ciphertext); err == nil {
		t.Fatal("key file contains plaintext PKCS8")
	}

	wrong, err := NewSoftwareSigner(dir, func(context.Context) ([]byte, error) {
		return []byte("different-synthetic-unlock-secret"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := wrong.Sign(ctx, ref, nil); !errors.Is(err, ErrUnlock) {
		t.Fatalf("wrong unlock secret: %v", err)
	}

	t.Run("authenticated identity", func(t *testing.T) {
		moved := record
		moved.Label = "other-valid-label"
		payload, err := json.Marshal(moved)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(keyFilePath(dir, moved.Label), payload, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := signer.Sign(ctx, KeyRef{Label: moved.Label}, nil); !errors.Is(err, ErrUnlock) {
			t.Fatalf("ciphertext accepted a different identity: %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(*keyFileRecord)
		want   error
	}{
		{"ciphertext", func(changed *keyFileRecord) {
			var encrypted softwareEnvelope
			if err := json.Unmarshal(changed.Data, &encrypted); err != nil {
				t.Fatal(err)
			}
			encrypted.Ciphertext[0] ^= 1
			changed.Data, _ = json.Marshal(encrypted)
		}, ErrUnlock},
		{"public key", func(changed *keyFileRecord) { changed.PublicKey[0] ^= 1 }, ErrCorrupt},
		{"backend", func(changed *keyFileRecord) { changed.Backend = "other" }, ErrCorrupt},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := record
			tc.mutate(&changed)
			payload, err := json.Marshal(changed)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, payload, 0600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Error(err)
				}
			}()
			if _, _, err := signer.Sign(ctx, ref, nil); !errors.Is(err, tc.want) {
				t.Fatalf("Sign() error = %v, want %v", err, tc.want)
			}
		})
	}

	unknownField := bytes.Replace(original, []byte(`"version":1`), []byte(`"version":1,"unknown":true`), 1)
	if err := os.WriteFile(path, unknownField, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := signer.Sign(ctx, ref, nil); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("unknown key-file field: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "target.json")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := signer.Sign(ctx, ref, nil); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("symlinked key file: %v", err)
	}
}

func TestSoftwareSignerClearsAndValidatesUnlockSecret(t *testing.T) {
	cause := errors.New("unlock failed")
	for _, tc := range []struct {
		name   string
		secret []byte
		err    error
		want   error
	}{
		{name: "success", secret: []byte(testUnlockSecret)},
		{name: "provider error", secret: []byte(testUnlockSecret), err: cause, want: cause},
		{name: "too short", secret: make([]byte, 15), want: ErrUnlockRequired},
		{name: "too long", secret: make([]byte, 1025), want: ErrUnlockRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			signer, err := NewSoftwareSigner(t.TempDir(), func(context.Context) ([]byte, error) {
				return tc.secret, tc.err
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := signer.EnsureKey(context.Background(), KeyRef{Label: "unlock"}); !errors.Is(err, tc.want) {
				t.Fatalf("EnsureKey() error = %v, want %v", err, tc.want)
			}
			assertCleared(t, tc.secret)
		})
	}
	if _, err := NewSoftwareSigner(t.TempDir(), nil); !errors.Is(err, ErrUnlockRequired) {
		t.Fatalf("nil unlock provider: %v", err)
	}
}

func keyFilePath(directory, label string) string {
	return filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(label))))
}
