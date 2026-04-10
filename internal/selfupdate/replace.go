// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package selfupdate handles platform-specific binary replacement
// for the CLI self-update flow.
package selfupdate

import "github.com/larksuite/cli/internal/vfs"

// Updater manages self-update operations.
// Platform-specific methods are in replace_unix.go and replace_windows.go.
type Updater struct{}

// New creates an Updater.
func New() *Updater { return &Updater{} }

// resolveExe returns the resolved path of the current running binary.
func (u *Updater) resolveExe() (string, error) {
	exe, err := vfs.Executable()
	if err != nil {
		return "", err
	}
	return vfs.EvalSymlinks(exe)
}
