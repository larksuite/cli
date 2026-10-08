// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package signingstore

import "github.com/larksuite/cli/internal/keychain"

func StorageDir() (string, error) {
	return keychain.StorageDir(keychain.LarkCliService), nil
}

func UnlockLockDirectory(directory, _, _ string) (string, error) {
	return directory, nil
}
