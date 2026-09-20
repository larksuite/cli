//go:build darwin || linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"os"
	"syscall"

	"github.com/larksuite/cli/internal/keychain"
)

// Refuse a final symlink swap and avoid blocking if a regular file becomes a FIFO.
const privateKeyOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

func signerStorageDir() (string, error) {
	return keychain.StorageDir(keychain.LarkCliService), nil
}

func unlockSecretLockDirectory(directory string) (string, error) {
	return directory, nil
}
