// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package keychain provides cross-platform secure storage for secrets.
// macOS uses the system Keychain; Linux uses AES-256-GCM encrypted files; Windows uses DPAPI + registry.
package keychain

import (
	"errors"
	"fmt"

	"github.com/larksuite/cli/internal/output"
)

var (
	// ErrNotFound is returned when the requested credential is not found.
	ErrNotFound = errors.New("keychain: item not found")

	// errNotInitialized is an internal error indicating the master key is missing or invalid.
	errNotInitialized = errors.New("keychain not initialized")
)

const (
	// LarkCliService is the unified keychain service name for all secrets
	// (both AppSecret and UAT). Entries are distinguished by account key format:
	//   - AppSecret: "appsecret:<appId>"
	//   - UAT:       "<appId>:<userOpenId>"
	LarkCliService = "lark-cli"
)

// wrapError is a helper to wrap underlying errors into output.ExitError.
// It formats the error message and provides a hint for troubleshooting keychain access issues.
func wrapError(op string, err error) error {
	if err == nil || errors.Is(err, ErrNotFound) {
		return err
	}

	msg := fmt.Sprintf("keychain %s failed: %v", op, err)
	hint := "Check if the OS keychain/credential manager is locked or accessible. If running inside a sandbox or CI environment, please ensure the process has the necessary permissions to access the keychain, you can try running this outside the sandbox."

	if errors.Is(err, errNotInitialized) {
		hint = "The keychain master key may have been cleaned up or deleted. If running inside a sandbox or CI environment, please ensure the process has the necessary permissions to access the keychain, you can try running this outside the sandbox. Otherwise, please reconfigure the CLI by running lark-cli config init."
	}

	func() {
		defer func() { recover() }()
		LogAuthError("keychain", op, fmt.Errorf("keychain %s error: %w", op, err))
	}()

	return output.ErrWithHint(output.ExitAPI, "config", msg, hint)
}

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
func Get(service, account string) (string, error) {
	val, err := platformGet(service, account)
	return val, wrapError("Get", err)
}

// Set stores a value in the keychain, overwriting any existing entry.
func Set(service, account, data string) error {
	return wrapError("Set", platformSet(service, account, data))
}

// Remove deletes an entry from the keychain. No error if not found.
func Remove(service, account string) error {
	return wrapError("Remove", platformRemove(service, account))
}
