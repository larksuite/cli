// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"

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
