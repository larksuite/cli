// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// PrepareSelfReplace renames the running binary out of the way on Windows
// so that npm's postinstall script can write the new binary to the original
// path without hitting EBUSY. On non-Windows platforms this is a no-op
// (Unix allows overwriting a running executable via inode semantics).
//
// Returns a restore function. Caller MUST call it on npm install failure.
func PrepareSelfReplace() (restore func(), err error) {
	noop := func() {}

	if runtime.GOOS != "windows" {
		return noop, nil
	}

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
		os.Remove(exe)              // remove partial/corrupt file if any
		os.Rename(oldPath, exe)     // restore original
	}

	return restore, nil
}

// CleanupStaleFiles removes leftover .old files from previous Windows upgrades.
// If the original binary is missing but .old exists (e.g. crash mid-update),
// it restores the .old to recover the installation.
func CleanupStaleFiles() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := currentExePath()
	if err != nil {
		return
	}
	oldPath := exe + ".old"

	if _, err := os.Stat(oldPath); err != nil {
		return // no .old file, nothing to do
	}

	if _, err := os.Stat(exe); err != nil {
		// Original is missing but .old exists — restore to recover.
		os.Rename(oldPath, exe)
		return
	}

	// Both exist — normal case, .old is stale.
	os.Remove(oldPath)
}

func currentExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
