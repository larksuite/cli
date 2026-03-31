// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
