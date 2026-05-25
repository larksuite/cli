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

	// ErrOrphanedCredentials signals that encrypted credentials exist on disk
	// but cannot be decrypted with any available master key. This happens when
	// the master key that originally encrypted them has been lost — typically
	// because the OS Keychain entry was deleted (e.g., via Keychain Access)
	// after credentials were stored, leaving the on-disk .enc files
	// permanently unreadable. The only recovery is `lark-cli config init`.
	//
	// Returned by:
	//   - DowngradeMasterKeyToFile (preemptive: would orphan future reads)
	//   - platformGet (diagnostic: this read can never succeed)
	ErrOrphanedCredentials = errors.New("encrypted credentials cannot be decrypted with any available master key (original key appears to be lost)")
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

	switch {
	case errors.Is(err, ErrOrphanedCredentials):
		// Override the keychain-centric message — the issue is not keychain
		// access but lost data. Do NOT append extraHint(): the
		// keychain-downgrade suggestion would mislead the user (downgrade
		// cannot recover the lost key, and would either no-op via
		// AlreadyDone or refuse via the orphan guard).
		msg = fmt.Sprintf("cannot read stored credential: %v", err)
		hint = "The master key that encrypted this credential is no longer available, so the on-disk data cannot be decrypted. This typically happens after the OS Keychain entry was deleted (manually via Keychain Access, by system maintenance, or by a botched migration). Run `lark-cli config init` to reconfigure from scratch — previously stored credentials cannot be recovered without the original master key."
	case errors.Is(err, errNotInitialized):
		hint = "The keychain master key may have been cleaned up or deleted. If running inside a sandbox or CI environment, please ensure the process has the necessary permissions to access the keychain, you can try running this outside the sandbox. Otherwise, please reconfigure the CLI by running lark-cli config init."
		hint += extraHint(err)
	default:
		hint += extraHint(err)
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
