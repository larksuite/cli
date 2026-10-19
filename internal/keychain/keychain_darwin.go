// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin

package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const keychainTimeout = 5 * time.Second
const masterKeyBytes = 32
const ivBytes = 12
const tagBytes = 16

// StorageDir returns the storage directory for a given service name on macOS.
func StorageDir(service string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".lark-cli", "keychain", service)
	}
	return filepath.Join(home, "Library", "Application Support", service)
}

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

func unsafeFileName(safeName string) string {
	// Remove the .enc extension
	unsafeName := strings.TrimSuffix(safeName, ".enc")
	// Replace underscores back to the original characters (we don't have reverse mapping)
	// For simplicity in our case, we'll just remove underscores which were added by safeFileNameRe
	return strings.ReplaceAll(unsafeName, "_", "")
}

func defaultFileBackend() *FileCredentialBackend {
	return NewFileCredentialBackend(filepath.Dir(StorageDir(LarkCliService)), systemMasterKeyring{})
}
func platformCredentialBackend() KeychainAccess { return defaultFileBackend() }
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

func platformGet(service, account string) string {
	value, _ := defaultFileBackend().Get(service, account)
	return value
}
func platformSet(service, account, data string) error {
	return defaultFileBackend().Set(service, account, data)
}
func platformRemove(service, account string) error {
	return defaultFileBackend().Remove(service, account)
}

// platformListKeys lists all keys in the keychain for a given service.
// This is used for enumerating stored tokens.
func platformListKeys(service string) ([]string, error) {
	dir := StorageDir(service)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var keys []string
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".enc") {
			// Get the original account name by removing the .enc suffix
			accountName := strings.TrimSuffix(entry.Name(), ".enc")
			keys = append(keys, accountName)
		}
	}

	return keys, nil
}
