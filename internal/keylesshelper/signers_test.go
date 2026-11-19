// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/vfs"
)

type testMetadataStore struct {
	values map[string]string
	getErr error
}

func (s *testMetadataStore) Get(_, account string) (string, error) {
	return s.values[account], s.getErr
}
func (s *testMetadataStore) Set(_, account, value string) error {
	s.values[account] = value
	return nil
}
func (s *testMetadataStore) Remove(_, account string) error {
	delete(s.values, account)
	return nil
}

type cancelingUnlockStore struct {
	*testMetadataStore
	cancel context.CancelFunc
}

func (s cancelingUnlockStore) Get(service, account string) (string, error) {
	s.cancel()
	return s.testMetadataStore.Get(service, account)
}

func TestSoftwareUnlockProtectsExistingKeys(t *testing.T) {
	directory := isolateSoftwareStorage(t)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "notes.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newSoftwareSigner(kc)
	ctx := context.Background()
	if _, err := signer.EnsureKey(ctx, keysigner.KeyRef{Label: "first"}); err != nil {
		t.Fatalf("unrelated JSON blocked first key: %v", err)
	}
	delete(kc.values, softwareUnlockAccount)
	_, err := signer.EnsureKey(ctx, keysigner.KeyRef{Label: "second"})
	if !errors.Is(err, keysigner.ErrUnlockRequired) || len(kc.values) != 0 {
		t.Fatalf("missing unlock secret for existing key: err=%v writes=%d", err, len(kc.values))
	}
}

func TestSoftwareUnlockStopsAfterCanceledKeychainRead(t *testing.T) {
	for _, tc := range []struct {
		name   string
		getErr error
	}{
		{"missing entry", keychain.ErrNotFound},
		{"read failure", errors.New("keychain read failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			isolateSoftwareStorage(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			kc := &testMetadataStore{values: map[string]string{}, getErr: tc.getErr}
			signer := newSoftwareSigner(cancelingUnlockStore{kc, cancel})
			_, err := signer.EnsureKey(ctx, keysigner.KeyRef{Label: "first"})
			if !errors.Is(err, context.Canceled) || !errors.Is(err, tc.getErr) || len(kc.values) != 0 {
				t.Fatalf("canceled keychain read: err=%v, stored entries=%d", err, len(kc.values))
			}
		})
	}
}

func isolateSoftwareStorage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, dir := range map[string]string{
		"HOME": root, "USERPROFILE": root, "LOCALAPPDATA": filepath.Join(root, "local"),
		"LARKSUITE_CLI_DATA_DIR":   filepath.Join(root, "data"),
		"LARKSUITE_CLI_CONFIG_DIR": filepath.Join(root, "config"),
	} {
		t.Setenv(name, dir)
	}
	directory := filepath.Join(root, "data", "lark-cli")
	if runtime.GOOS == "darwin" {
		directory = filepath.Join(root, "Library", "Application Support", "lark-cli")
	}
	return filepath.Join(directory, "keysigner", "keyless", "software-file")
}

func TestRegistrationSignersSoftwareFallback(t *testing.T) {
	directory := isolateSoftwareStorage(t)
	kc := &testMetadataStore{values: map[string]string{}}
	signers := RegistrationSigners(kc)
	names := append(keysigner.PlatformSignerNames(), keysigner.SoftwareSignerName)
	var got []string
	for _, signer := range signers {
		got = append(got, signer.Name())
	}
	if !slices.Equal(got, names) {
		t.Fatalf("candidates = %v, want %v", got, names)
	}
	if _, err := vfs.Stat(directory); !errors.Is(err, os.ErrNotExist) || len(kc.values) != 0 {
		t.Fatal("constructing signers accessed storage")
	}
	signer := signers[len(signers)-1]
	ctx := context.Background()
	ref := keysigner.KeyRef{Label: "software-test"}
	_, err := signer.EnsureKey(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if signer.SecurityLevel() != keysigner.SecurityLevelL3 || kc.values["keyless:software:unlock:v1"] == "" {
		t.Fatal("software key did not persist its separate protection secret")
	}
	lockDirectory := directory
	if runtime.GOOS == "windows" {
		lockDirectory = filepath.Join(os.Getenv("LOCALAPPDATA"), "lark-cli", "keysigner", "keyless", "software-file")
	}
	if _, err := vfs.Stat(filepath.Join(lockDirectory, "unlock.lock")); err != nil {
		t.Fatalf("keyless unlock lock: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("key files = %v, error = %v", files, err)
	}
	if runtime.GOOS != "windows" {
		for path, mode := range map[string]uint32{directory: 0o700, files[0]: 0o600} {
			info, err := vfs.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if uint32(info.Mode().Perm()) != mode {
				t.Fatalf("mode = %o, want %o", info.Mode().Perm(), mode)
			}
		}
	}
	reopened, err := ResolveSigner(keysigner.SoftwareSignerName, kc)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteKey(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, _, err := reopened.Sign(ctx, ref, nil); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("missing key must not be recreated: %v", err)
	}
	if len(kc.values) != 1 || kc.values[softwareUnlockAccount] == "" {
		t.Fatal("key deletion changed the shared protection secret")
	}
}

func TestResolveSignerRequiresProvider(t *testing.T) {
	if signer, err := ResolveSigner("", &testMetadataStore{}); err == nil || signer != nil {
		t.Fatalf("missing provider resolved to %v, err = %v", signer, err)
	}
}
