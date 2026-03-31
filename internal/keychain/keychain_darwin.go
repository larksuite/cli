// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"time"

	"github.com/zalando/go-keyring"
)

const keychainTimeout = 5 * time.Second

// StorageDir returns the storage directory for a given service name on macOS.
func StorageDir(service string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".lark-cli", "keychain", service)
	}
	return filepath.Join(home, "Library", "Application Support", service)
}

func getMasterKey(service string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), keychainTimeout)
	defer cancel()

	type result struct {
		key []byte
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		defer func() { recover() }()

		encodedKey, err := keyring.Get(service, "master.key")
		if err == nil {
			key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
			if decodeErr == nil && len(key) == masterKeyBytes {
				resCh <- result{key: key, err: nil}
				return
			}
		}

		// Generate new master key if not found or invalid
		key := make([]byte, masterKeyBytes)
		if _, randErr := rand.Read(key); randErr != nil {
			resCh <- result{key: nil, err: randErr}
			return
		}

		encodedKey = base64.StdEncoding.EncodeToString(key)
		setErr := keyring.Set(service, "master.key", encodedKey)
		resCh <- result{key: key, err: setErr}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return nil, WrapUnavailable(res.err)
		}
		return res.key, nil
	case <-ctx.Done():
		return nil, WrapUnavailable(ctx.Err())
	}
}

func platformGet(service, account string) string {
	key, err := getMasterKey(service)
	if err != nil {
		return ""
	}
	// Shared encrypted-file read semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return readEncryptedFile(StorageDir(service), account, key)
}

func platformSet(service, account, data string) error {
	key, err := getMasterKey(service)
	if err != nil {
		return err
	}
	// Shared encrypted-file write semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return writeEncryptedFile(StorageDir(service), account, data, key)
}

func platformRemove(service, account string) error {
	// Shared encrypted-file cleanup semantics live in file_encrypted_store.go.
	// New code should reuse that helper layer instead of reimplementing file I/O here.
	return removeEncryptedFile(StorageDir(service), account)
}
