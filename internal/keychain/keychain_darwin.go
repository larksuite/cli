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
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

const keychainTimeout = 5 * time.Second
const masterKeyBytes = 32
const ivBytes = 12
const tagBytes = 16
const goKeyringEncodedPrefix = "go-keyring-encoded:"
const goKeyringBase64Prefix = "go-keyring-base64:"

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

func decodeMasterKeyValue(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}

	candidates := []string{value}
	switch {
	case strings.HasPrefix(value, goKeyringBase64Prefix):
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, goKeyringBase64Prefix))
		if err == nil {
			candidates = append(candidates, strings.TrimSpace(string(decoded)))
		}
	case strings.HasPrefix(value, goKeyringEncodedPrefix):
		decoded, err := hex.DecodeString(strings.TrimPrefix(value, goKeyringEncodedPrefix))
		if err == nil {
			candidates = append(candidates, strings.TrimSpace(string(decoded)))
		}
	}

	for _, candidate := range candidates {
		key, err := base64.StdEncoding.DecodeString(candidate)
		if err == nil && len(key) == masterKeyBytes {
			return key, true
		}
	}
	return nil, false
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
			if key, ok := decodeMasterKeyValue(encodedKey); ok {
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
		return res.key, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
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

func platformGet(service, account string) string {
	key, err := getMasterKey(service)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil {
		return ""
	}
	plaintext, err := decryptData(data, key)
	if err != nil {
		return ""
	}
	return plaintext
}

func platformSet(service, account, data string) error {
	key, err := getMasterKey(service)
	if err != nil {
		return err
	}
	dir := StorageDir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	encrypted, err := encryptData(data, key)
	if err != nil {
		return err
	}

	targetPath := filepath.Join(dir, safeFileName(account))
	tmpPath := filepath.Join(dir, safeFileName(account)+"."+uuid.New().String()+".tmp")
	defer os.Remove(tmpPath)

	if err := os.WriteFile(tmpPath, encrypted, 0600); err != nil {
		return err
	}

	// Atomic rename to prevent file corruption during multi-process writes
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	return nil
}

func platformRemove(service, account string) error {
	err := os.Remove(filepath.Join(StorageDir(service), safeFileName(account)))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
