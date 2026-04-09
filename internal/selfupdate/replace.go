// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// ReplaceSelf replaces the currently running binary with newBinary.
//
// On Unix: atomic os.Rename (same filesystem) or copy fallback.
// On Windows: the running .exe is locked, so we:
//  1. Rename running.exe → running.exe.old (Windows allows rename of locked files)
//  2. Copy new binary → running.exe
//  3. Best-effort delete .old (may fail if still locked; cleaned up on next run)
func ReplaceSelf(newBinary string) error {
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current binary path: %w", err)
	}
	current, err = filepath.EvalSymlinks(current)
	if err != nil {
		return fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	if err := os.Chmod(newBinary, 0755); err != nil {
		return fmt.Errorf("cannot set permissions on new binary: %w", err)
	}

	if runtime.GOOS == "windows" {
		return replaceWindows(current, newBinary)
	}
	return replaceUnix(current, newBinary)
}

func replaceUnix(current, newBinary string) error {
	// Try atomic rename first (works on same filesystem).
	if err := os.Rename(newBinary, current); err == nil {
		return nil
	}
	// Cross-device fallback: copy.
	return copyFile(newBinary, current)
}

func replaceWindows(current, newBinary string) error {
	oldPath := current + ".old"

	// Clean up stale .old from previous upgrades.
	os.Remove(oldPath)

	// Step 1: Rename running exe to .old (Windows allows this).
	if err := os.Rename(current, oldPath); err != nil {
		return fmt.Errorf("cannot rename current binary: %w", err)
	}

	// Step 2: Copy new binary into place.
	if err := copyFile(newBinary, current); err != nil {
		// Rollback: restore the old binary.
		os.Rename(oldPath, current)
		return fmt.Errorf("cannot install new binary: %w", err)
	}

	// Step 3: Best-effort cleanup.
	os.Remove(oldPath)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// CleanupStaleFiles removes leftover .old files from previous Windows upgrades.
func CleanupStaleFiles() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	exe, _ = filepath.EvalSymlinks(exe)
	os.Remove(exe + ".old")
}
