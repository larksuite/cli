// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/configdir"
	"github.com/larksuite/cli/internal/validate"
)

const masterKeyBytes = 32
const ivBytes = 12
const tagBytes = 16

// safeFileName maps an account key to a collision-free filesystem name.
func safeFileName(account string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(account)) + ".enc"
}

// fallbackStorageDir returns the per-service directory used by the encrypted file fallback store.
func fallbackStorageDir(service string) string {
	// Keep config dir resolution centralized in internal/configdir.
	// New code should reuse configdir.Get() instead of duplicating env/home logic.
	return filepath.Join(configdir.Get(), "keychain", service)
}

// loadMasterKeyFile reads an existing fallback master key without creating new storage.
func loadMasterKeyFile(dir string) ([]byte, error) {
	key, err := os.ReadFile(filepath.Join(dir, "master.key"))
	if err != nil {
		return nil, err
	}
	if len(key) != masterKeyBytes {
		return nil, os.ErrInvalid
	}
	return key, nil
}

// loadOrCreateMasterKeyFile returns the fallback master key, creating it only when absent.
func loadOrCreateMasterKeyFile(dir string) ([]byte, error) {
	keyPath := filepath.Join(dir, "master.key")

	key, err := loadMasterKeyFile(dir)
	if err == nil {
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	key = make([]byte, masterKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := createMasterKeyFile(keyPath, key); err != nil {
		if os.IsExist(err) {
			return loadMasterKeyFile(dir)
		}
		if existingKey, readErr := loadMasterKeyFile(dir); readErr == nil {
			return existingKey, nil
		}
		return nil, err
	}
	return key, nil
}

// createMasterKeyFile creates master.key with no-replace semantics.
func createMasterKeyFile(path string, key []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	success := false
	defer func() {
		if success {
			return
		}
		file.Close()
		os.Remove(path)
	}()

	if _, err := file.Write(key); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	success = true
	return nil
}

// encryptData seals plaintext with AES-GCM using the provided master key.
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

// decryptData opens AES-GCM ciphertext produced by encryptData.
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

// readEncryptedFile reads an encrypted fallback file and suppresses read or decrypt errors.
func readEncryptedFile(dir, account string, key []byte) string {
	plaintext, err := readEncryptedFileWithError(dir, account, key)
	if err != nil {
		return ""
	}
	return plaintext
}

// readEncryptedFileWithError reads and decrypts an encrypted fallback file while preserving error detail.
func readEncryptedFileWithError(dir, account string, key []byte) (string, error) {
	path := filepath.Join(dir, safeFileName(account))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return "", fmt.Errorf("decrypt fallback file %s: %w", path, err)
	}
	return plaintext, nil
}

// writeEncryptedFile encrypts and atomically writes fallback data for an account key.
func writeEncryptedFile(dir, account, data string, key []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encrypted, err := encryptData(data, key)
	if err != nil {
		return err
	}
	return validate.AtomicWrite(filepath.Join(dir, safeFileName(account)), encrypted, 0600)
}

// removeEncryptedFile deletes the encrypted fallback file for an account key.
func removeEncryptedFile(dir, account string) error {
	err := os.Remove(filepath.Join(dir, safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// GetFallback reads fallback data and returns an empty string when the value is absent or unreadable.
func GetFallback(service, account string) string {
	value, err := GetFallbackWithError(service, account)
	if err != nil {
		return ""
	}
	return value
}

// GetFallbackWithError reads fallback data and preserves not-found or decrypt errors for callers that need diagnostics.
func GetFallbackWithError(service, account string) (string, error) {
	dir := fallbackStorageDir(service)
	path := filepath.Join(dir, safeFileName(account))
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	key, err := loadMasterKeyFile(dir)
	if err != nil {
		return "", fmt.Errorf("load fallback master key for %s: %w", path, err)
	}
	return readEncryptedFileWithError(dir, account, key)
}

// SetFallback stores fallback data for a service/account pair.
func SetFallback(service, account, data string) error {
	dir := fallbackStorageDir(service)
	key, err := loadOrCreateMasterKeyFile(dir)
	if err != nil {
		return err
	}
	return writeEncryptedFile(dir, account, data, key)
}

// RemoveFallback removes fallback data for a service/account pair.
func RemoveFallback(service, account string) error {
	return removeEncryptedFile(fallbackStorageDir(service), account)
}
