// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
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

func resolveMasterKey(
	getFn func() (string, error),
	setFn func(string) error,
	generateFn func() ([]byte, error),
) ([]byte, error) {
	encodedKey, err := getFn()
	switch {
	case err == nil:
		key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
		if decodeErr != nil {
			return nil, WrapUnavailable(fmt.Errorf("decode master key: %w", decodeErr))
		}
		if len(key) != masterKeyBytes {
			return nil, WrapUnavailable(fmt.Errorf("invalid master key length: %d", len(key)))
		}
		return key, nil
	case errors.Is(err, keyring.ErrNotFound):
		key, genErr := generateFn()
		if genErr != nil {
			return nil, genErr
		}
		if setErr := setFn(base64.StdEncoding.EncodeToString(key)); setErr != nil {
			return nil, WrapUnavailable(setErr)
		}
		return key, nil
	default:
		return nil, WrapUnavailable(err)
	}
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
		key, err := resolveMasterKey(
			func() (string, error) { return keyring.Get(service, "master.key") },
			func(encoded string) error { return keyring.Set(service, "master.key", encoded) },
			func() ([]byte, error) {
				key := make([]byte, masterKeyBytes)
				if _, randErr := rand.Read(key); randErr != nil {
					return nil, randErr
				}
				return key, nil
			},
		)
		resCh <- result{key: key, err: err}
	}()

	select {
	case res := <-resCh:
		return res.key, res.err
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
