// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"os"
	"path/filepath"
	"regexp"

	"github.com/larksuite/cli/internal/configdir"
	"github.com/larksuite/cli/internal/validate"
)

const masterKeyBytes = 32
const ivBytes = 12
const tagBytes = 16

var safeFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func safeFileName(account string) string {
	return safeFileNameRe.ReplaceAllString(account, "_") + ".enc"
}

func fallbackStorageDir(service string) string {
	// Keep config dir resolution centralized in internal/configdir.
	// New code should reuse configdir.Get() instead of duplicating env/home logic.
	return filepath.Join(configdir.Get(), "keychain", service)
}

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

func loadOrCreateMasterKeyFile(dir string) ([]byte, error) {
	keyPath := filepath.Join(dir, "master.key")

	key, err := loadMasterKeyFile(dir)
	if err == nil {
		return key, nil
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}

	key = make([]byte, masterKeyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := validate.AtomicWrite(keyPath, key, 0600); err != nil {
		if existingKey, readErr := loadMasterKeyFile(dir); readErr == nil {
			return existingKey, nil
		}
		return nil, err
	}
	return key, nil
}

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

func readEncryptedFile(dir, account string, key []byte) string {
	data, err := os.ReadFile(filepath.Join(dir, safeFileName(account)))
	if err != nil {
		return ""
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return ""
	}
	return plaintext
}

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

func removeEncryptedFile(dir, account string) error {
	err := os.Remove(filepath.Join(dir, safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func GetFallback(service, account string) string {
	dir := fallbackStorageDir(service)
	key, err := loadMasterKeyFile(dir)
	if err != nil {
		return ""
	}
	return readEncryptedFile(dir, account, key)
}

func SetFallback(service, account, data string) error {
	dir := fallbackStorageDir(service)
	key, err := loadOrCreateMasterKeyFile(dir)
	if err != nil {
		return err
	}
	return writeEncryptedFile(dir, account, data, key)
}

func RemoveFallback(service, account string) error {
	return removeEncryptedFile(fallbackStorageDir(service), account)
}
