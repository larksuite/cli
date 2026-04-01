// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFallbackStore_EncryptsAndRemovesData verifies fallback values are encrypted at rest and removable.
func TestFallbackStore_EncryptsAndRemovesData(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	service := LarkCliService
	account := "appsecret:cli_test"
	plaintext := "secret123"

	if err := SetFallback(service, account, plaintext); err != nil {
		t.Fatalf("SetFallback: %v", err)
	}

	if got := GetFallback(service, account); got != plaintext {
		t.Fatalf("GetFallback = %q, want %q", got, plaintext)
	}

	encryptedPath := filepath.Join(fallbackStorageDir(service), safeFileName(account))
	data, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", encryptedPath, err)
	}
	if strings.Contains(string(data), plaintext) {
		t.Fatalf("encrypted fallback file unexpectedly contains plaintext %q", plaintext)
	}

	masterKeyPath := filepath.Join(fallbackStorageDir(service), "master.key")
	if info, err := os.Stat(masterKeyPath); err != nil {
		t.Fatalf("master key file missing: %v", err)
	} else if info.Mode().Perm() != 0600 {
		t.Fatalf("master key perm = %v, want 0600", info.Mode().Perm())
	}

	if err := RemoveFallback(service, account); err != nil {
		t.Fatalf("RemoveFallback: %v", err)
	}
	if got := GetFallback(service, account); got != "" {
		t.Fatalf("expected fallback secret to be removed, got %q", got)
	}
}

// TestGetFallback_MissDoesNotCreateStorageArtifacts verifies read misses stay side-effect free.
func TestGetFallback_MissDoesNotCreateStorageArtifacts(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	service := LarkCliService
	account := "missing:account"

	if got := GetFallback(service, account); got != "" {
		t.Fatalf("GetFallback = %q, want empty string for missing account", got)
	}

	fallbackDir := fallbackStorageDir(service)
	if _, err := os.Stat(fallbackDir); !os.IsNotExist(err) {
		t.Fatalf("expected fallback dir to stay absent on read miss, stat err = %v", err)
	}
}

// TestCreateMasterKeyFile_DoesNotReplaceExistingFile verifies master.key creation uses no-replace semantics.
func TestCreateMasterKeyFile_DoesNotReplaceExistingFile(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")
	existingKey := bytes.Repeat([]byte{1}, masterKeyBytes)
	replacementKey := bytes.Repeat([]byte{2}, masterKeyBytes)

	if err := os.WriteFile(keyPath, existingKey, 0600); err != nil {
		t.Fatalf("WriteFile(%s): %v", keyPath, err)
	}

	err := createMasterKeyFile(keyPath, replacementKey)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("createMasterKeyFile error = %v, want %v", err, os.ErrExist)
	}

	got, readErr := os.ReadFile(keyPath)
	if readErr != nil {
		t.Fatalf("ReadFile(%s): %v", keyPath, readErr)
	}
	if !bytes.Equal(got, existingKey) {
		t.Fatalf("master key was replaced: got %x want %x", got, existingKey)
	}
}

// TestLoadOrCreateMasterKeyFile_ReusesExistingKey verifies existing master keys are reused rather than replaced.
func TestLoadOrCreateMasterKeyFile_ReusesExistingKey(t *testing.T) {
	dir := t.TempDir()
	existingKey := bytes.Repeat([]byte{7}, masterKeyBytes)

	if err := os.WriteFile(filepath.Join(dir, "master.key"), existingKey, 0600); err != nil {
		t.Fatalf("WriteFile(master.key): %v", err)
	}

	got, err := loadOrCreateMasterKeyFile(dir)
	if err != nil {
		t.Fatalf("loadOrCreateMasterKeyFile: %v", err)
	}
	if !bytes.Equal(got, existingKey) {
		t.Fatalf("loadOrCreateMasterKeyFile returned %x, want %x", got, existingKey)
	}
}

// TestSafeFileName_EncodesFullAccountWithoutCollision verifies account keys map to collision-free filenames.
func TestSafeFileName_EncodesFullAccountWithoutCollision(t *testing.T) {
	accountA := "appsecret:cli_test"
	accountB := "appsecret/cli_test"

	gotA := safeFileName(accountA)
	gotB := safeFileName(accountB)
	if gotA == gotB {
		t.Fatalf("safeFileName collision: %q and %q both mapped to %q", accountA, accountB, gotA)
	}
	if want := base64.RawURLEncoding.EncodeToString([]byte(accountA)) + ".enc"; gotA != want {
		t.Fatalf("safeFileName(%q) = %q, want %q", accountA, gotA, want)
	}
}

// TestGetFallbackWithError_ReturnsDecryptFailure verifies corrupt ciphertext is surfaced as a decrypt error.
func TestGetFallbackWithError_ReturnsDecryptFailure(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	service := LarkCliService
	account := "appsecret:cli_test"
	dir := fallbackStorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "master.key"), bytes.Repeat([]byte{1}, masterKeyBytes), 0600); err != nil {
		t.Fatalf("WriteFile(master.key): %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, safeFileName(account)), []byte("corrupt"), 0600); err != nil {
		t.Fatalf("WriteFile(ciphertext): %v", err)
	}

	got, err := GetFallbackWithError(service, account)
	if err == nil {
		t.Fatal("expected GetFallbackWithError to report decrypt failure")
	}
	if got != "" {
		t.Fatalf("GetFallbackWithError returned %q, want empty string on error", got)
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Fatalf("expected decrypt context in error, got %v", err)
	}
}
