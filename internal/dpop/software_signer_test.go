// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/keysigner"
)

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
	return filepath.Join(directory, "keysigner", "software-file")
}

func TestDefaultKeyStoreSoftwareFallbackLifecycle(t *testing.T) {
	directory := isolateSoftwareStorage(t)
	kc := &testMetadataStore{values: map[string]string{}}
	store := NewKeyStore(kc)
	var want []string
	switch runtime.GOOS {
	case "darwin":
		want = []string{"macos-secure-enclave/L1", "macos-keychain/L2"}
	case "windows":
		want = []string{"windows-platform-ksp/L1", "windows-software-ksp/L2"}
	case "linux":
		want = []string{"linux-tpm/L1"}
	}
	want = append(want, "software-file/L3")
	var got []string
	for _, signer := range store.signers {
		got = append(got, signer.Name()+"/"+string(signer.SecurityLevel()))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("default DPoP signers = %v, want %v", got, want)
	}
	if _, err := os.Stat(directory); !errors.Is(err, os.ErrNotExist) || len(kc.values) != 0 {
		t.Fatal("constructing a store accessed software storage")
	}
	for i := 0; i < len(store.signers)-1; i++ {
		signer := newTestStoreSigner(store.signers[i].Name(), store.signers[i].SecurityLevel())
		signer.ensureErr = keysigner.ErrUnavailable
		store.signers[i] = signer
	}
	ctx := context.Background()
	if err := store.ProbeWritableContext(ctx); err != nil {
		t.Fatal(err)
	}
	if locks, err := filepath.Glob(filepath.Join(directory, "*.json.lock")); err != nil || len(locks) != 0 {
		t.Fatalf("software probe left key locks behind: %v, err = %v", locks, err)
	}
	key, err := store.GenerateContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if key.Provider() != keysigner.SoftwareSignerName || key.SecurityLevel() != keysigner.SecurityLevelL3 {
		t.Fatal("did not select L3 after unavailable platform backends")
	}
	if err := store.SaveContext(ctx, key); err != nil {
		t.Fatal(err)
	}
	lockDirectory := directory
	if runtime.GOOS == "windows" {
		lockDirectory = filepath.Join(os.Getenv("LOCALAPPDATA"), "lark-cli", "keysigner", "software-file")
	}
	if _, err := os.Stat(filepath.Join(lockDirectory, "unlock.lock")); err != nil {
		t.Fatalf("shared unlock lock is outside the signer directory: %v", err)
	}
	jkt, _ := key.Thumbprint()
	reopened := NewKeyStore(kc)
	loaded, err := reopened.LoadContext(ctx, key.ID())
	if err != nil {
		t.Fatal(err)
	}
	gotJKT, _ := loaded.Thumbprint()
	if gotJKT != jkt || loaded.Provider() != keysigner.SoftwareSignerName {
		t.Fatal("reopen changed the key or selected another backend")
	}
	if proof, err := loaded.SignProofContext(ctx, "GET", "https://example.test/resource"); err != nil || proof == "" {
		t.Fatalf("reopened key cannot sign: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(directory, "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("key files = %v, err = %v", files, err)
	}
	if err := reopened.DeleteKeyContext(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Load(key.ID()); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("deleted metadata still loads: %v", err)
	}
	if _, err := reopened.signerByName(keysigner.SoftwareSignerName).PublicKey(ctx, keysigner.KeyRef{Label: key.ID()}); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("deleted physical key still loads: %v", err)
	}
	if len(kc.values) != 1 || kc.values["dpop:software:unlock:v1"] == "" {
		t.Fatal("only the fixed unlock account should remain after key deletion")
	}
}

func TestSoftwareSignerDoesNotReplaceMissingOrCorruptUnlockSecret(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrupt", true: "missing"}[missing], func(t *testing.T) {
			directory := isolateSoftwareStorage(t)
			kc := &testMetadataStore{values: map[string]string{}}
			signer := NewKeyStore(kc).signerByName(keysigner.SoftwareSignerName)
			ctx := context.Background()
			ref := keysigner.KeyRef{Label: "existing"}
			if _, err := signer.EnsureKey(ctx, ref); err != nil {
				t.Fatal(err)
			}
			if kc.values[softwareUnlockAccount] == "" {
				t.Fatal("unlock secret was not stored in keychain")
			}
			want := keysigner.ErrCorrupt
			kc.values[softwareUnlockAccount] = "invalid-base64"
			if missing {
				want = keysigner.ErrUnlockRequired
				delete(kc.values, softwareUnlockAccount)
			}
			if _, _, err := signer.Sign(ctx, ref, []byte("input")); !errors.Is(err, want) {
				t.Fatalf("sign error = %v, want %v", err, want)
			}
			for _, label := range []string{"existing", "another"} {
				if _, err := signer.EnsureKey(ctx, keysigner.KeyRef{Label: label}); !errors.Is(err, want) || errors.Is(err, keysigner.ErrUnavailable) {
					t.Fatalf("ensure error = %v, want hard %v", err, want)
				}
			}
			files, _ := filepath.Glob(filepath.Join(directory, "*.json"))
			if len(files) != 1 || missing && len(kc.values) != 0 || !missing && kc.values[softwareUnlockAccount] != "invalid-base64" {
				t.Fatal("failure replaced secret or wrote another encrypted key")
			}
		})
	}
}

func TestSoftwareSignerConcurrentInitialization(t *testing.T) {
	directory := isolateSoftwareStorage(t)
	kc := &testMetadataStore{values: map[string]string{}}
	signers := make(map[string]keysigner.Signer)
	for _, label := range []string{"first", "second"} {
		keyDirectory := directory
		if runtime.GOOS == "windows" {
			// Data-dir overrides still share the same registry unlock account.
			keyDirectory = filepath.Join(directory, label)
		}
		signer, err := keysigner.NewSoftwareSigner(keyDirectory, func(ctx context.Context) ([]byte, error) {
			return (softwareSigner{keychain: kc}).unlock(ctx, keyDirectory, true)
		})
		if err != nil {
			t.Fatal(err)
		}
		signers[label] = signer
	}
	var wg sync.WaitGroup
	for _, label := range []string{"first", "second"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			signer := signers[label]
			ref := keysigner.KeyRef{Label: label}
			if _, err := signer.EnsureKey(context.Background(), ref); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(kc.values) != 1 || kc.values["dpop:software:unlock:v1"] == "" {
		t.Fatal("concurrent initialization did not share one unlock secret")
	}
	for _, label := range []string{"first", "second"} {
		if _, err := signers[label].PublicKey(context.Background(), keysigner.KeyRef{Label: label}); err != nil {
			t.Fatalf("concurrent initialization orphaned a key: %v", err)
		}
	}
}

func TestSoftwareSignerKeychainFailureIsHard(t *testing.T) {
	for _, operation := range []string{"read", "write"} {
		t.Run(operation, func(t *testing.T) {
			directory := isolateSoftwareStorage(t)
			cause := errors.New("injected keychain failure")
			kc := &testMetadataStore{values: map[string]string{}}
			if operation == "read" {
				kc.getErr = cause
			} else {
				kc.setErr = cause
			}
			signer := NewKeyStore(kc).signerByName(keysigner.SoftwareSignerName)
			_, err := signer.EnsureKey(context.Background(), keysigner.KeyRef{Label: "new"})
			if !errors.Is(err, cause) || errors.Is(err, keysigner.ErrUnavailable) {
				t.Fatalf("unlock error = %v, want original hard failure", err)
			}
			files, _ := filepath.Glob(filepath.Join(directory, "*.json"))
			if len(files) != 0 || len(kc.values) != 0 {
				t.Fatal("failed unlock initialization persisted a key or secret")
			}
		})
	}
}
