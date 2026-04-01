// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/keychain"
)

type erroringSetKeychain struct {
	err error
}

func (e *erroringSetKeychain) Get(service, account string) (string, error) { return "", nil }
func (e *erroringSetKeychain) Set(service, account, value string) error    { return e.err }
func (e *erroringSetKeychain) Remove(service, account string) error        { return nil }

func TestForStorageWithEncryptedFallback_DoesNotFallbackOnGenericSetError(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	_, err := ForStorageWithEncryptedFallback(
		"cli_test",
		PlainSecret("secret123"),
		&erroringSetKeychain{err: errors.New("boom")},
	)
	if err == nil {
		t.Fatal("expected ForStorageWithEncryptedFallback to return the keychain write error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected original keychain write error, got %v", err)
	}
	if got := keychain.GetFallback(keychain.LarkCliService, "appsecret:cli_test"); got != "" {
		t.Fatalf("expected no encrypted fallback to be written for generic keychain error, got %q", got)
	}
}

func TestSecretInput_UnmarshalAcceptsFileSource(t *testing.T) {
	var input SecretInput
	data := []byte(`{"source":"file","id":"/tmp/app-secret.txt"}`)

	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if input.Ref == nil {
		t.Fatal("expected secret ref")
	}
	if input.Ref.Source != "file" {
		t.Fatalf("source = %q, want file", input.Ref.Source)
	}
	if input.Ref.ID != "/tmp/app-secret.txt" {
		t.Fatalf("id = %q, want /tmp/app-secret.txt", input.Ref.ID)
	}
}

func TestResolveSecretInput_FileSourceReadsSecretFile(t *testing.T) {
	secretFile := filepath.Join(t.TempDir(), "app-secret.txt")
	if err := os.WriteFile(secretFile, []byte("secret123\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	secret, err := ResolveSecretInput(SecretInput{
		Ref: &SecretRef{Source: "file", ID: secretFile},
	}, &erroringSetKeychain{})
	if err != nil {
		t.Fatalf("ResolveSecretInput: %v", err)
	}
	if secret != "secret123" {
		t.Fatalf("secret = %q, want secret123", secret)
	}
}

func TestResolveSecretInput_EncryptedFallbackIncludesUnderlyingError(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	serviceDir := filepath.Join(configDir, "keychain", keychain.LarkCliService)
	if err := os.MkdirAll(serviceDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "master.key"), []byte("12345678901234567890123456789012"), 0600); err != nil {
		t.Fatalf("WriteFile(master.key): %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "YXBwc2VjcmV0OmNsaV90ZXN0.enc"), []byte("corrupt"), 0600); err != nil {
		t.Fatalf("WriteFile(ciphertext): %v", err)
	}

	_, err := ResolveSecretInput(SecretInput{
		Ref: &SecretRef{Source: "encrypted_file", ID: "appsecret:cli_test"},
	}, &erroringSetKeychain{})
	if err == nil {
		t.Fatal("expected ResolveSecretInput to report fallback decrypt failure")
	}
	if !strings.Contains(err.Error(), "failed to decrypt encrypted fallback secret") {
		t.Fatalf("expected decrypt-specific error, got %v", err)
	}
}
