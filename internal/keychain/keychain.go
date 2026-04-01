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

var errNotInitialized = errors.New("keychain not initialized")

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
	if err == nil {
		return nil
	}
	msg := fmt.Sprintf("keychain %s failed: %v", op, err)
	hint := "Check if the OS keychain/credential manager is locked or accessible. If running inside a sandbox or CI environment, please ensure the process has the necessary permissions to access the keychain."

	if errors.Is(err, errNotInitialized) {
		hint = "The keychain master key may have been cleaned up or deleted. Please reconfigure the CLI by running `lark-cli config init`."
	}

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
