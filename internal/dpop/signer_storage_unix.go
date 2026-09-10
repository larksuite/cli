// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package dpop

import "github.com/larksuite/cli/internal/keychain"

func signerStorageDir() (string, error) {
	return keychain.StorageDir(keychain.LarkCliService), nil
}

func softwareUnlockLockDir(directory string) (string, error) {
	return directory, nil
}
