// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/keysigner"
)

type memorySecrets struct{ values map[string]string }

func (s *memorySecrets) Get(_, account string) (string, error) { return s.values[account], nil }
func (s *memorySecrets) Set(_, account, value string) error    { s.values[account] = value; return nil }
func (s *memorySecrets) Remove(_, account string) error        { delete(s.values, account); return nil }

func TestResolveSecretPreservesExistingAndReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name, encoded    string
		create, existing bool
		want             error
	}{
		{"read-only", "", false, false, keysigner.ErrUnlockRequired},
		{"lost-secret", "", true, true, keysigner.ErrUnlockRequired},
		{"corrupt", "!", true, false, keysigner.ErrCorrupt},
		{"short", base64.StdEncoding.EncodeToString([]byte("short")), true, false, keysigner.ErrCorrupt},
		{"existing", base64.StdEncoding.EncodeToString(make([]byte, 32)), false, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			if tc.existing {
				if err := os.WriteFile(filepath.Join(directory, "key.json"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			kc := &memorySecrets{values: map[string]string{}}
			policy := SecretPolicy{Account: "first", Description: "software unlock secret", IsKeyFile: func(name string) bool { return strings.HasSuffix(name, ".json") }, Missing: func(string) error { return keysigner.ErrUnlockRequired }}
			secret, err := ResolveSecret(kc, directory, tc.encoded, tc.create, policy)
			defer clear(secret)
			if !errors.Is(err, tc.want) || len(kc.values) != 0 {
				t.Fatalf("secret=%d error=%v writes=%v", len(secret), err, kc.values)
			}
		})
	}
}

func TestResolveSecretSeparatesAccounts(t *testing.T) {
	kc := &memorySecrets{values: map[string]string{}}
	for _, account := range []string{"dpop:software:unlock:v1", "keyless:software:unlock:v1"} {
		secret, err := ResolveSecret(kc, t.TempDir(), "", true, SecretPolicy{Account: account, IsKeyFile: func(string) bool { return false }})
		clear(secret)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(kc.values) != 2 || kc.values["dpop:software:unlock:v1"] == kc.values["keyless:software:unlock:v1"] {
		t.Fatal("unlock namespaces were merged")
	}
}

func TestStoreLockIsNonReentrantAndPathScoped(t *testing.T) {
	first, second := filepath.Join(t.TempDir(), "key_store.lock"), filepath.Join(t.TempDir(), "key_store.lock")
	if err := WithStoreLock(context.Background(), first, func() error {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := WithStoreLock(ctx, first, func() error { t.Fatal("entered held lock"); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		return WithStoreLock(context.Background(), second, func() error { return nil })
	}); err != nil {
		t.Fatal(err)
	}
}
