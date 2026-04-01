// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build linux

package keychain

import (
	"fmt"
	"os"
	"path/filepath"
)

// StorageDir returns the storage directory for a given service name.
// Each service gets its own directory for physical isolation.
func StorageDir(service string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// If home is missing, fallback to relative path and print warning.
		// This matches the behavior in internal/core/config.go.
		fmt.Fprintf(os.Stderr, "warning: unable to determine home directory: %v\n", err)
	}
	xdgData := filepath.Join(home, ".local", "share")
	return filepath.Join(xdgData, service)
}

// getMasterKey loads or initializes the per-service master key on Linux.
func getMasterKey(service string) ([]byte, error) {
	// Shared master-key file handling lives in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing key-file setup here.
	return loadOrCreateMasterKeyFile(StorageDir(service))
}

// platformGet reads a service/account secret from the Linux encrypted-file backend.
func platformGet(service, account string) string {
	key, err := getMasterKey(service)
	if err != nil {
		return ""
	}
	// Shared encrypted-file read semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return readEncryptedFile(StorageDir(service), account, key)
}

// platformSet writes a service/account secret through the Linux encrypted-file backend.
func platformSet(service, account, data string) error {
	key, err := getMasterKey(service)
	if err != nil {
		return err
	}
	// Shared encrypted-file write semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return writeEncryptedFile(StorageDir(service), account, data, key)
}

// platformRemove deletes a service/account secret from the Linux encrypted-file backend.
func platformRemove(service, account string) error {
	// Shared encrypted-file cleanup semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return removeEncryptedFile(StorageDir(service), account)
}
