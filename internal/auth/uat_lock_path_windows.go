// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package auth

import (
	"os"
	"path/filepath"
)

func refreshLockDir() string {
	baseDir, err := os.UserCacheDir()
	if err != nil || baseDir == "" {
		baseDir, err = os.UserHomeDir()
		if err != nil || baseDir == "" {
			baseDir = ".lark-cli"
		}
	}
	return filepath.Join(baseDir, "lark-cli", "credential-locks")
}
