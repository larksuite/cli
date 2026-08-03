// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package auth

import (
	"path/filepath"

	"github.com/larksuite/cli/internal/keychain"
)

func refreshLockDir() string {
	return filepath.Join(keychain.StorageDir(keychain.LarkCliService), "locks")
}
