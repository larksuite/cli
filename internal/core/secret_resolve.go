// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/keychain"
)

const secretKeyPrefix = "appsecret:"

func secretAccountKey(appId string) string {
	return secretKeyPrefix + appId
}

// ResolveSecretInput resolves a SecretInput to a plain string.
// SecretRef objects are resolved by source (file / keychain / encrypted_file).
func ResolveSecretInput(s SecretInput, kc keychain.KeychainAccess) (string, error) {
	if s.Ref == nil {
		return s.Plain, nil
	}
	switch s.Ref.Source {
	case "file":
		data, err := os.ReadFile(s.Ref.ID)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file %s: %w", s.Ref.ID, err)
		}
		return strings.TrimSpace(string(data)), nil
	case "encrypted_file":
		value := keychain.GetFallback(keychain.LarkCliService, s.Ref.ID)
		if value == "" {
			return "", fmt.Errorf("failed to read encrypted fallback secret %s", s.Ref.ID)
		}
		return value, nil
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
		return SecretInput{}, fmt.Errorf("store secret in keychain: %w", err)
	}
	return SecretInput{Ref: &SecretRef{Source: "keychain", ID: key}}, nil
}

// ForStorageWithEncryptedFallback stores a plain secret in keychain when available,
// or falls back to the shared encrypted file store.
func ForStorageWithEncryptedFallback(appId string, input SecretInput, kc keychain.KeychainAccess) (SecretInput, error) {
	if !input.IsPlain() {
		return input, nil
	}
	key := secretAccountKey(appId)
	if err := kc.Set(keychain.LarkCliService, key, input.Plain); err == nil {
		return SecretInput{Ref: &SecretRef{Source: "keychain", ID: key}}, nil
	} else if !keychain.ShouldUseFallback(err) {
		return SecretInput{}, fmt.Errorf("store secret in keychain: %w", err)
	}
	if err := keychain.SetFallback(keychain.LarkCliService, key, input.Plain); err != nil {
		return SecretInput{}, fmt.Errorf("store secret encrypted fallback: %w", err)
	}
	return SecretInput{Ref: &SecretRef{Source: "encrypted_file", ID: key}}, nil
}

// RemoveSecretStore cleans up keychain entries when an app is removed.
// Errors are intentionally ignored — cleanup is best-effort.
func RemoveSecretStore(input SecretInput, kc keychain.KeychainAccess) {
	if !input.IsSecretRef() {
		return
	}
	switch input.Ref.Source {
	case "file":
		return
	case "keychain":
		_ = kc.Remove(keychain.LarkCliService, input.Ref.ID)
	case "encrypted_file":
		_ = keychain.RemoveFallback(keychain.LarkCliService, input.Ref.ID)
	}
}
