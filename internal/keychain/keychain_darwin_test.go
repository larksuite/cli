// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/zalando/go-keyring"
)

// TestPlatformSetFallsBackToFileMasterKey verifies writes fall back to a file master key
// when the system keychain cannot create the master key.
func TestPlatformSetFallsBackToFileMasterKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		return "", keyring.ErrNotFound
	}
	keyringSet = func(service, user, password string) error {
		return errors.New("blocked")
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"
	account := "test-account"
	secret := "secret-value"

	if err := platformSet(service, account, secret); err != nil {
		t.Fatalf("platformSet() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(StorageDir(service), fileMasterKeyName)); err != nil {
		t.Fatalf("file master key not created: %v", err)
	}

	got, err := platformGet(service, account)
	if err != nil {
		t.Fatalf("platformGet() error = %v", err)
	}
	if got != secret {
		t.Fatalf("platformGet() = %q, want %q", got, secret)
	}
}

// TestPlatformGetPrefersFileMasterKey verifies reads prefer the file-based master key
// before trying the system keychain master key.
func TestPlatformGetPrefersFileMasterKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	fileKey := make([]byte, masterKeyBytes)
	for i := range fileKey {
		fileKey[i] = byte(i + 1)
	}
	keychainKey := make([]byte, masterKeyBytes)
	for i := range keychainKey {
		keychainKey[i] = byte(i + 33)
	}

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		return base64.StdEncoding.EncodeToString(keychainKey), nil
	}
	keyringSet = func(service, user, password string) error {
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"
	account := "test-account"
	secret := "secret-value"

	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileMasterKeyName), fileKey, 0600); err != nil {
		t.Fatalf("WriteFile(master key) error = %v", err)
	}
	encrypted, err := encryptData(secret, fileKey)
	if err != nil {
		t.Fatalf("encryptData() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeFileName(account)), encrypted, 0600); err != nil {
		t.Fatalf("WriteFile(secret) error = %v", err)
	}

	got, err := platformGet(service, account)
	if err != nil {
		t.Fatalf("platformGet() error = %v", err)
	}
	if got != secret {
		t.Fatalf("platformGet() = %q, want %q", got, secret)
	}
}

// TestDowngradeAlreadyDoneIsIdempotent verifies that re-running downgrade
// when master.key.file already exists is a no-op and reports AlreadyDone
// without touching the system keychain.
func TestDowngradeAlreadyDoneIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		t.Fatalf("keyringGet should not be called when master.key.file is already valid")
		return "", nil
	}
	keyringSet = func(service, user, password string) error {
		t.Fatalf("keyringSet should not be called when master.key.file is already valid")
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	preExisting := make([]byte, masterKeyBytes)
	for i := range preExisting {
		preExisting[i] = byte(i + 7)
	}
	keyPath := filepath.Join(dir, fileMasterKeyName)
	if err := os.WriteFile(keyPath, preExisting, 0600); err != nil {
		t.Fatalf("WriteFile(master key) error = %v", err)
	}

	result, err := DowngradeMasterKeyToFile(service)
	if err != nil {
		t.Fatalf("DowngradeMasterKeyToFile() error = %v", err)
	}
	if result != DowngradeAlreadyDone {
		t.Fatalf("result = %v, want DowngradeAlreadyDone", result)
	}

	after, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytesEqual(after, preExisting) {
		t.Fatalf("master.key.file content changed; want preserved")
	}
}

// TestDowngradeCopiesKeychainKeyToFile verifies the happy path: a keychain
// key exists, the file does not, and downgrade copies the bytes verbatim
// so that existing .enc files (encrypted with the keychain key) remain
// readable via the file fallback.
func TestDowngradeCopiesKeychainKeyToFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	keychainKey := make([]byte, masterKeyBytes)
	for i := range keychainKey {
		keychainKey[i] = byte(i + 11)
	}

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		return base64.StdEncoding.EncodeToString(keychainKey), nil
	}
	keyringSet = func(service, user, password string) error {
		t.Fatalf("keyringSet should not be called when keychain already has a master key")
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"

	result, err := DowngradeMasterKeyToFile(service)
	if err != nil {
		t.Fatalf("DowngradeMasterKeyToFile() error = %v", err)
	}
	if result != DowngradeUsedKeychainKey {
		t.Fatalf("result = %v, want DowngradeUsedKeychainKey", result)
	}

	got, err := os.ReadFile(MasterKeyFilePath(service))
	if err != nil {
		t.Fatalf("ReadFile(master.key.file) error = %v", err)
	}
	if !bytesEqual(got, keychainKey) {
		t.Fatalf("file key bytes do not match keychain key; existing .enc files would become unreadable")
	}
}

// TestDowngradeCreatesNewKeyWhenStorageEmpty verifies the "fresh user"
// path: keychain is empty and no .enc files exist, so we generate a new
// random key and write it to the file fallback. The OS Keychain is NOT
// modified (regression guard for the side-effecting getMasterKey(_, true)
// call we used to make).
func TestDowngradeCreatesNewKeyWhenStorageEmpty(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		return "", keyring.ErrNotFound
	}
	keyringSet = func(service, user, password string) error {
		t.Fatalf("keyringSet must not be called; keychain-downgrade never writes to the system Keychain")
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"

	result, err := DowngradeMasterKeyToFile(service)
	if err != nil {
		t.Fatalf("DowngradeMasterKeyToFile() error = %v", err)
	}
	if result != DowngradeCreatedNewKey {
		t.Fatalf("result = %v, want DowngradeCreatedNewKey", result)
	}

	fileKey, err := os.ReadFile(MasterKeyFilePath(service))
	if err != nil {
		t.Fatalf("ReadFile(master.key.file) error = %v", err)
	}
	if len(fileKey) != masterKeyBytes {
		t.Fatalf("file key length = %d, want %d", len(fileKey), masterKeyBytes)
	}
}

// TestDowngradeDoesNotClobberConcurrentlyWrittenKey is the regression guard
// for the TOCTOU between the initial existence check and the final write.
// Race trace the fix closes:
//
//	T0 proc A: ReadFile(keyPath) → ErrNotExist        (initial check passes)
//	T1 proc B: platformSet → getFileMasterKey(_, true) creates keyPath with K_B
//	           then writes .enc encrypted with K_B
//	T2 proc A: rand.Read → K_A; would overwrite K_B and orphan B's .enc
//
// We simulate proc B's interleaving by performing the concurrent file write
// inside the keyringGet hook — by the time DowngradeMasterKeyToFile gets back
// to the final OpenFile call, the file already exists, the O_EXCL branch
// fires, and the concurrent key is preserved verbatim.
func TestDowngradeDoesNotClobberConcurrentlyWrittenKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	service := "test-service"
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	concurrentKey := make([]byte, masterKeyBytes)
	for i := range concurrentKey {
		concurrentKey[i] = byte(i + 77)
	}

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(svc, user string) (string, error) {
		if err := os.WriteFile(filepath.Join(dir, fileMasterKeyName), concurrentKey, 0600); err != nil {
			t.Fatalf("simulated concurrent write failed: %v", err)
		}
		return "", keyring.ErrNotFound
	}
	keyringSet = func(svc, user, password string) error {
		t.Fatalf("keyringSet must not be called; keychain-downgrade never writes to the system Keychain")
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	result, err := DowngradeMasterKeyToFile(service)
	if err != nil {
		t.Fatalf("DowngradeMasterKeyToFile() error = %v", err)
	}
	if result != DowngradeAlreadyDone {
		t.Fatalf("result = %v, want DowngradeAlreadyDone (concurrent write must be preserved)", result)
	}
	got, err := os.ReadFile(filepath.Join(dir, fileMasterKeyName))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !bytesEqual(got, concurrentKey) {
		t.Fatalf("master.key.file was clobbered; concurrent platformSet's encrypted credentials would be orphaned")
	}
}

// TestPlatformGetSurfacesKeychainBlocked verifies that "keychain access blocked"
// (the sandbox case) propagates as errKeychainBlocked through platformGet, so
// the wrapError hint chain can attach the keychain-downgrade suggestion.
func TestPlatformGetSurfacesKeychainBlocked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		return "", errors.New("sandbox denied keychain access")
	}
	keyringSet = func(service, user, password string) error {
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"
	account := "test-account"
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	lostKey := make([]byte, masterKeyBytes)
	for i := range lostKey {
		lostKey[i] = byte(i + 55)
	}
	encrypted, err := encryptData("secret", lostKey)
	if err != nil {
		t.Fatalf("encryptData() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeFileName(account)), encrypted, 0600); err != nil {
		t.Fatalf("WriteFile(.enc) error = %v", err)
	}

	_, err = platformGet(service, account)
	if !errors.Is(err, errKeychainBlocked) {
		t.Fatalf("err = %v, want errKeychainBlocked", err)
	}
}

// TestWrapErrorHintMentionsDowngradeForRecoverableCases is the regression
// guard for the bug where `lark-cli api ...` inside a sandbox surfaced
// "keychain access blocked" but the hint did NOT mention keychain-downgrade
// — the very command meant to recover from that exact situation. Root cause:
// the blocked path used an anonymous errors.New string, so the extraHint
// `errors.Is` check (only matched errNotInitialized) couldn't recognize it.
//
// Asserts the full wrapError → typed APIError hint pipeline:
//   - errKeychainBlocked + errNotInitialized → hint mentions keychain-downgrade
//   - "keychain is corrupted" (downgrade would re-read the same bad bytes) → no mention
//   - generic errors → no mention
//
// Add new cases here whenever extraHint's matcher widens, to keep the
// promise that the hint is suggested iff downgrade can actually help.
func TestWrapErrorHintMentionsDowngradeForRecoverableCases(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantHint bool
	}{
		{"access blocked (sandbox / denied prompt / timeout)", errKeychainBlocked, true},
		{"not initialized (missing master key)", errNotInitialized, true},
		{"corrupted (downgrade would re-read the same bad bytes)", errors.New("keychain is corrupted"), false},
		{"unrelated generic error", errors.New("something else entirely"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := wrapError("Get", tc.err)
			var apiErr *errs.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("wrapError returned %#v; expected *errs.APIError", err)
			}
			got := strings.Contains(apiErr.Hint, "keychain-downgrade")
			if got != tc.wantHint {
				t.Fatalf("hint mentions keychain-downgrade = %v, want %v\n  full hint: %q", got, tc.wantHint, apiErr.Hint)
			}
		})
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPlatformSetPrefersExistingFileMasterKey verifies writes stay on the file-based
// master key path once the fallback master key already exists.
func TestPlatformSetPrefersExistingFileMasterKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	origGet := keyringGet
	origSet := keyringSet
	keyringGet = func(service, user string) (string, error) {
		t.Fatalf("keyringGet should not be called when file master key exists")
		return "", nil
	}
	keyringSet = func(service, user, password string) error {
		t.Fatalf("keyringSet should not be called when file master key exists")
		return nil
	}
	t.Cleanup(func() {
		keyringGet = origGet
		keyringSet = origSet
	})

	service := "test-service"
	account := "test-account"
	secret := "secret-value"

	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	fileKey := make([]byte, masterKeyBytes)
	for i := range fileKey {
		fileKey[i] = byte(i + 1)
	}
	if err := os.WriteFile(filepath.Join(dir, fileMasterKeyName), fileKey, 0600); err != nil {
		t.Fatalf("WriteFile(master key) error = %v", err)
	}

	if err := platformSet(service, account, secret); err != nil {
		t.Fatalf("platformSet() error = %v", err)
	}

	got, err := platformGet(service, account)
	if err != nil {
		t.Fatalf("platformGet() error = %v", err)
	}
	if got != secret {
		t.Fatalf("platformGet() = %q, want %q", got, secret)
	}
}
