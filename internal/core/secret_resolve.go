// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/vfs"
)

const secretKeyPrefix = "appsecret:"

func secretAccountKey(appId string) string {
	return secretKeyPrefix + appId
}

// ResolveSecretInput resolves a SecretInput to a plain string.
// SecretRef objects are resolved by source (file / keychain).
func ResolveSecretInput(s SecretInput, kc keychain.KeychainAccess) (string, error) {
	if s.Ref == nil {
		return s.Plain, nil
	}
	switch s.Ref.Source {
	case "file":
		data, err := vfs.ReadFile(s.Ref.ID)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file %s: %w", s.Ref.ID, err)
		}
		return strings.TrimSpace(string(data)), nil
	case "keychain":
		return kc.Get(keychain.LarkCliService, s.Ref.ID)
	default:
		return "", fmt.Errorf("unknown secret source: %s", s.Ref.Source)
	}
}

// ForStorage determines how to store a secret in config.json.
// - SecretRef → preserved as-is
// - Plain text → stored in keychain, returns keychain SecretRef
// Returns error if keychain is unavailable (no silent plaintext fallback).
func ForStorage(appId string, input SecretInput, kc keychain.KeychainAccess) (SecretInput, error) {
	if !input.IsPlain() {
		return input, nil // SecretRef → keep as-is
	}
	key := secretAccountKey(appId)
	if err := kc.Set(keychain.LarkCliService, key, input.Plain); err != nil {
		return SecretInput{}, fmt.Errorf("keychain unavailable: %w\nhint: use file: reference in config to bypass keychain", err)
	}
	return SecretInput{Ref: &SecretRef{Source: "keychain", ID: key}}, nil
}

// ValidateSecretKeyMatch checks that the appSecret keychain key references the
// expected appId. This prevents silent mismatches when config.json is edited by
// hand (e.g. appId changed but appSecret.id still points to the old app).
// Only applicable when appSecret is a keychain SecretRef; other forms are skipped.
func ValidateSecretKeyMatch(appId string, secret SecretInput) error {
	if secret.Ref == nil || secret.Ref.Source != "keychain" {
		return nil
	}
	expected := secretAccountKey(appId)
	if secret.Ref.ID != expected {
		return fmt.Errorf(
			"appSecret keychain key %q does not match appId %q (expected %q); "+
				"please run `lark-cli config init` to reconfigure",
			secret.Ref.ID, appId, expected,
		)
	}
	return nil
}

// RemoveSecretStore cleans up keychain entries when an app is removed.
// Errors are intentionally ignored — cleanup is best-effort.
func RemoveSecretStore(input SecretInput, kc keychain.KeychainAccess) {
	if input.IsSecretRef() && input.Ref.Source == "keychain" {
		_ = kc.Remove(keychain.LarkCliService, input.Ref.ID)
	}
}
