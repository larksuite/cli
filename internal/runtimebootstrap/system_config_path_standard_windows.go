// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended && windows

package runtimebootstrap

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func defaultStandardSystemConfigPath() string {
	base, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, windows.KF_FLAG_DEFAULT)
	if err != nil || base == "" {
		base = `C:\ProgramData`
	}
	return filepath.Join(base, "lark-cli", "external-credential.json")
}
