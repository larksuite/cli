// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keychain provides cross-platform secure storage for secrets.
// macOS uses the system Keychain; Linux uses AES-256-GCM encrypted files; Windows uses DPAPI + registry.
package keychain

import "errors"

var ErrNotFound = errors.New("credential_not_found")

const (
	// LarkCliService is the unified keychain service name for all secrets
	// (both AppSecret and UAT). Entries are distinguished by account key format:
	//   - AppSecret: "appsecret:<appId>"
	//   - UAT:       "<appId>:<userOpenId>"
	LarkCliService = "lark-cli"
)

// KeychainAccess abstracts keychain Get/Set/Remove for dependency injection.
// Used by AppSecret operations (ForStorage, ResolveSecretInput, RemoveSecretStore).
// UAT operations in token_store.go use the package-level Get/Set/Remove directly.
type KeychainAccess interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
	Remove(service, account string) error
}

// Get retrieves a value from the keychain.
// Returns empty string if the entry does not exist.
func Get(service, account string) string {
	return platformGet(service, account)
}

// Set stores a value in the keychain, overwriting any existing entry.
func Set(service, account, data string) error {
	return platformSet(service, account, data)
}

// Remove deletes an entry from the keychain. No error if not found.
func Remove(service, account string) error {
	return platformRemove(service, account)
}

// ListKeys lists all keys in the keychain for a given service.
// This is primarily used for token management and cleanup.
func ListKeys(service string) ([]string, error) {
	return platformListKeys(service)
}
