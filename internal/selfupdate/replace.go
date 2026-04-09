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
// Returns a cleanup function that restores the original if the update fails.
// Caller MUST call cleanup on error to avoid leaving the installation broken.
//
// Usage:
//
//	cleanup, err := selfupdate.PrepareSelfReplace()
//	if err != nil { ... }
//	if err := runNpmInstall(...); err != nil {
//	    cleanup() // restore original
//	    return err
//	}
func PrepareSelfReplace() (cleanup func(), err error) {
	noop := func() {}

	if runtime.GOOS != "windows" {
		return noop, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return noop, nil // best-effort; don't block update
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return noop, nil
	}

	oldPath := exe + ".old"

	// Clean up stale .old from a previous upgrade.
	os.Remove(oldPath)

	// Rename running.exe → running.exe.old (Windows allows this).
	if err := os.Rename(exe, oldPath); err != nil {
		return noop, fmt.Errorf("cannot rename binary for update: %w", err)
	}

	// Cleanup: restore the original if npm install fails.
	restore := func() {
		os.Rename(oldPath, exe)
	}

	return restore, nil
}

// CleanupStaleFiles removes leftover .old files from previous Windows upgrades.
// Safe to call on any platform (no-op if no .old file exists).
func CleanupStaleFiles() {
	if runtime.GOOS != "windows" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	os.Remove(exe + ".old")
}
