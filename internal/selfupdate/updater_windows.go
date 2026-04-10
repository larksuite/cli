// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package selfupdate

import (
	"fmt"

	"github.com/larksuite/cli/internal/vfs"
)

// PrepareSelfReplace renames the running .exe to .old so that npm's
// postinstall script can write the new binary without hitting EBUSY.
// Returns a restore function that undoes the rename on failure.
func (u *Updater) PrepareSelfReplace() (restore func(), err error) {
	noop := func() {}

	exe, err := u.resolveExe()
	if err != nil {
		return noop, nil // best-effort; don't block update
	}

	oldPath := exe + ".old"

	// Clean up stale .old from a previous upgrade.
	vfs.Remove(oldPath)

	// Rename running.exe → running.exe.old (Windows allows rename of locked files).
	if err := vfs.Rename(exe, oldPath); err != nil {
		return noop, fmt.Errorf("cannot rename binary for update: %w", err)
	}

	// Restore: remove any partial file npm may have left, then move .old back.
	restore = func() {
		vfs.Remove(exe)
		vfs.Rename(oldPath, exe)
	}

	return restore, nil
}

// CleanupStaleFiles removes leftover .old files from previous upgrades.
// If the original binary is missing but .old exists (crash mid-update),
// it restores the .old to recover the installation.
func (u *Updater) CleanupStaleFiles() {
	exe, err := u.resolveExe()
	if err != nil {
		return
	}
	oldPath := exe + ".old"

	if _, err := vfs.Stat(oldPath); err != nil {
		return // no .old file
	}

	if _, err := vfs.Stat(exe); err != nil {
		// Original missing, .old exists — restore to recover.
		vfs.Rename(oldPath, exe)
		return
	}

	// Both exist — .old is stale, clean up.
	vfs.Remove(oldPath)
}

// CanRestorePreviousVersion reports whether PrepareSelfReplace created a
// restorable backup for the current update attempt.
func (u *Updater) CanRestorePreviousVersion() bool {
	if u.RestoreAvailableOverride != nil {
		return u.RestoreAvailableOverride()
	}
	return true
}
