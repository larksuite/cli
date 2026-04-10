// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package selfupdate

import (
	"fmt"
	"os"
)

// PrepareSelfReplace renames the running .exe to .old so that npm's
// postinstall script can write the new binary without hitting EBUSY.
// Returns a restore function that undoes the rename on failure.
func PrepareSelfReplace() (restore func(), err error) {
	noop := func() {}

	exe, err := currentExePath()
	if err != nil {
		return noop, nil // best-effort; don't block update
	}

	oldPath := exe + ".old"

	// Clean up stale .old from a previous upgrade.
	os.Remove(oldPath)

	// Rename running.exe → running.exe.old (Windows allows rename of locked files).
	if err := os.Rename(exe, oldPath); err != nil {
		return noop, fmt.Errorf("cannot rename binary for update: %w", err)
	}

	// Restore: remove any partial file npm may have left, then move .old back.
	restore = func() {
		os.Remove(exe)
		os.Rename(oldPath, exe)
	}

	return restore, nil
}

// CleanupStaleFiles removes leftover .old files from previous upgrades.
// If the original binary is missing but .old exists (crash mid-update),
// it restores the .old to recover the installation.
func CleanupStaleFiles() {
	exe, err := currentExePath()
	if err != nil {
		return
	}
	oldPath := exe + ".old"

	if _, err := os.Stat(oldPath); err != nil {
		return // no .old file
	}

	if _, err := os.Stat(exe); err != nil {
		// Original missing, .old exists — restore to recover.
		os.Rename(oldPath, exe)
		return
	}

	// Both exist — .old is stale, clean up.
	os.Remove(oldPath)
}
