// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/zalando/go-keyring"
)

const keychainTimeout = 5 * time.Second
const masterKeyBytes = 32
const ivBytes = 12
const tagBytes = 16

// StorageDir returns the storage directory for a given service name on macOS.
func StorageDir(service string) string {
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".lark-cli", "keychain", service)
	}
	return filepath.Join(home, "Library", "Application Support", service)
}

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// safeFileName sanitizes an account name to be used as a safe file name.
func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

// getMasterKey retrieves the master key from the system keychain.
// If allowCreate is true, it generates and stores a new master key if one doesn't exist.
func getMasterKey(service string, allowCreate bool) ([]byte, error) {
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
			// Key is found but invalid or corrupted
			resCh <- result{key: nil, err: errors.New("keychain is corrupted")}
			return
		} else if !errors.Is(err, keyring.ErrNotFound) {
			// Not ErrNotFound, which means access was denied or blocked by the system
			resCh <- result{key: nil, err: errors.New("keychain access blocked")}
			return
		}

		// If ErrNotFound, check if we are allowed to create a new key
		if !allowCreate {
			// Creation not allowed (e.g., during Get operation), return error
			resCh <- result{key: nil, err: errNotInitialized}
			return
		}

		// It's the first time and creation is allowed (Set operation), generate a new key
		key := make([]byte, masterKeyBytes)
		if _, randErr := rand.Read(key); randErr != nil {
			resCh <- result{key: nil, err: randErr}
			return
		}

		encodedKeyStr := base64.StdEncoding.EncodeToString(key)
		setErr := keyring.Set(service, "master.key", encodedKeyStr)
		if setErr != nil {
			resCh <- result{key: nil, err: setErr}
			return
		}
		resCh <- result{key: key, err: nil}
	}()

	select {
	case res := <-resCh:
		return res.key, res.err
	case <-ctx.Done():
		// Timeout is usually caused by ignored/blocked permission prompts
		return nil, errors.New("keychain access blocked")
	}
}

// encryptData encrypts data using AES-GCM.
func encryptData(plaintext string, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	iv := make([]byte, ivBytes)
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}

	ciphertext := aesGCM.Seal(nil, iv, []byte(plaintext), nil)
	result := make([]byte, 0, ivBytes+len(ciphertext))
	result = append(result, iv...)
	result = append(result, ciphertext...)
	return result, nil
}

// decryptData decrypts data using AES-GCM.
func decryptData(data []byte, key []byte) (string, error) {
	if len(data) < ivBytes+tagBytes {
		return "", os.ErrInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	iv := data[:ivBytes]
	ciphertext := data[ivBytes:]
	plaintext, err := aesGCM.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// platformGet retrieves a value from the macOS keychain.
func platformGet(service, account string) (string, error) {
	path := filepath.Join(StorageDir(service), safeFileName(account))
	data, err := vfs.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	key, err := getMasterKey(service, false)
	if err != nil {
		return "", err
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return "", err
	}
	return plaintext, nil
}

// platformSet stores a value in the macOS keychain.
func platformSet(service, account, data string) error {
	key, err := getMasterKey(service, true)
	if err != nil {
		return err
	}
	dir := StorageDir(service)
	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encrypted, err := encryptData(data, key)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(dir, safeFileName(account))
	tmpPath := filepath.Join(dir, safeFileName(account)+"."+uuid.New().String()+".tmp")
	defer vfs.Remove(tmpPath)

	if err := vfs.WriteFile(tmpPath, encrypted, 0600); err != nil {
		return err
	}

	// Atomic rename to prevent file corruption during multi-process writes
	if err := vfs.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

// platformRemove deletes a value from the macOS keychain.
func platformRemove(service, account string) error {
	err := vfs.Remove(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
