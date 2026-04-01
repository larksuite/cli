// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
)

type failingTokenKeychain struct{}

func (f *failingTokenKeychain) Get(service, account string) (string, error) { return "", nil }
func (f *failingTokenKeychain) Set(service, account, value string) error {
	return keychain.WrapUnavailable(errors.New("sandbox denied"))
}
func (f *failingTokenKeychain) Remove(service, account string) error { return nil }

type genericFailingTokenKeychain struct{}

func (f *genericFailingTokenKeychain) Get(service, account string) (string, error) { return "", nil }
func (f *genericFailingTokenKeychain) Set(service, account, value string) error {
	return errors.New("boom")
}
func (f *genericFailingTokenKeychain) Remove(service, account string) error { return nil }

type removeFailingTokenKeychain struct{}

func (f *removeFailingTokenKeychain) Get(service, account string) (string, error) { return "", nil }
func (f *removeFailingTokenKeychain) Set(service, account, value string) error    { return nil }
func (f *removeFailingTokenKeychain) Remove(service, account string) error {
	return errors.New("remove failed")
}

func TestSetStoredToken_FallsBackToManagedFileWhenKeychainUnavailable(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	prev := tokenKeychain
	tokenKeychain = &failingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		Scope:            "offline_access",
		GrantedAt:        1709996400000,
	}

	usedFallback, err := SetStoredToken(token)
	if err != nil {
		t.Fatalf("SetStoredToken returned error: %v", err)
	}
	if !usedFallback {
		t.Fatal("expected SetStoredToken to report fallback usage")
	}

	if fallback, err := keychain.GetFallbackWithError(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); err != nil || fallback == "" {
		t.Fatal("expected encrypted fallback token to be stored")
	}

	stored := GetStoredToken(token.AppId, token.UserOpenId)
	if stored == nil {
		t.Fatal("expected GetStoredToken to read file-backed token")
	}
	if stored.AccessToken != token.AccessToken {
		t.Fatalf("stored access token = %q, want %q", stored.AccessToken, token.AccessToken)
	}
	if stored.RefreshToken != token.RefreshToken {
		t.Fatalf("stored refresh token = %q, want %q", stored.RefreshToken, token.RefreshToken)
	}
}

func TestRemoveStoredToken_RemovesManagedFileFallback(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	prev := tokenKeychain
	tokenKeychain = &failingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		GrantedAt:        1709996400000,
	}

	if _, err := SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken returned error: %v", err)
	}

	if fallback, err := keychain.GetFallbackWithError(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); err != nil || fallback == "" {
		t.Fatal("expected encrypted fallback token to exist before removal")
	}

	if err := RemoveStoredToken(token.AppId, token.UserOpenId); err != nil {
		t.Fatalf("RemoveStoredToken returned error: %v", err)
	}

	if fallback, err := keychain.GetFallbackWithError(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); err == nil && fallback != "" {
		t.Fatalf("expected encrypted fallback token to be removed, got %q", fallback)
	}
}

func TestGetStoredToken_UsesEncryptedFallbackWhenPrimaryStoreMisses(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	prev := tokenKeychain
	tokenKeychain = &failingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		GrantedAt:        1709996400000,
	}
	if _, err := SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken returned error: %v", err)
	}

	stored := GetStoredToken(token.AppId, token.UserOpenId)
	if stored == nil {
		t.Fatal("expected GetStoredToken to read encrypted fallback token")
	}
	if stored.AccessToken != token.AccessToken {
		t.Fatalf("stored access token = %q, want %q", stored.AccessToken, token.AccessToken)
	}
}

func TestGetStoredToken_ReadsLegacyManagedTokenFile(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	prev := tokenKeychain
	tokenKeychain = &failingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		GrantedAt:        1709996400000,
	}
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyManagedTokenFilePath(token.AppId, token.UserOpenId)), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(legacyManagedTokenFilePath(token.AppId, token.UserOpenId), data, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stored := GetStoredToken(token.AppId, token.UserOpenId)
	if stored == nil {
		t.Fatal("expected GetStoredToken to read legacy managed token file")
	}
	if stored.AccessToken != token.AccessToken {
		t.Fatalf("stored access token = %q, want %q", stored.AccessToken, token.AccessToken)
	}
}

func TestRemoveStoredToken_ReturnsKeychainErrorWhenFallbackIsAbsent(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	prev := tokenKeychain
	tokenKeychain = &removeFailingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	err := RemoveStoredToken("cli_test", "ou_test_user")
	if err == nil {
		t.Fatal("expected RemoveStoredToken to return keychain removal error")
	}
	if !strings.Contains(err.Error(), "remove failed") {
		t.Fatalf("expected keychain removal error, got %v", err)
	}
}

func TestSetStoredToken_DoesNotFallbackOnGenericKeychainError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	prev := tokenKeychain
	tokenKeychain = &genericFailingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		GrantedAt:        1709996400000,
	}

	usedFallback, err := SetStoredToken(token)
	if err == nil {
		t.Fatal("expected SetStoredToken to return the keychain write error")
	}
	if usedFallback {
		t.Fatal("expected generic keychain error to avoid fallback")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected original keychain write error, got %v", err)
	}
	if fallback, readErr := keychain.GetFallbackWithError(keychain.LarkCliService, accountKey(token.AppId, token.UserOpenId)); readErr == nil && fallback != "" {
		t.Fatalf("expected no encrypted fallback token for generic keychain error, got %q", fallback)
	}
}

func TestGetStoredToken_DoesNotFallBackToLegacyWhenEncryptedFallbackIsCorrupt(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	prev := tokenKeychain
	tokenKeychain = &failingTokenKeychain{}
	t.Cleanup(func() { tokenKeychain = prev })

	token := &StoredUAToken{
		UserOpenId:       "ou_test_user",
		AppId:            "cli_test",
		AccessToken:      "legacy-access-token",
		RefreshToken:     "legacy-refresh-token",
		ExpiresAt:        1710000000000,
		RefreshExpiresAt: 1710003600000,
		GrantedAt:        1709996400000,
	}
	data, err := json.Marshal(token)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(legacyManagedTokenFilePath(token.AppId, token.UserOpenId)), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(legacyManagedTokenFilePath(token.AppId, token.UserOpenId), data, 0600); err != nil {
		t.Fatalf("WriteFile(legacy): %v", err)
	}

	serviceDir := filepath.Join(configDir, "keychain", keychain.LarkCliService)
	if err := os.MkdirAll(serviceDir, 0700); err != nil {
		t.Fatalf("MkdirAll(serviceDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "master.key"), []byte("12345678901234567890123456789012"), 0600); err != nil {
		t.Fatalf("WriteFile(master.key): %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "Y2xpX3Rlc3Q6b3VfdGVzdF91c2Vy.enc"), []byte("corrupt"), 0600); err != nil {
		t.Fatalf("WriteFile(ciphertext): %v", err)
	}

	stored := GetStoredToken(token.AppId, token.UserOpenId)
	if stored != nil {
		t.Fatalf("expected corrupt encrypted fallback to stop legacy fallback, got %#v", stored)
	}
}
