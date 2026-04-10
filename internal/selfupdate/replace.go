// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package selfupdate

import (
	"os"
	"path/filepath"
)

// PrepareSelfReplace renames the running binary out of the way on Windows
// so that npm's postinstall script can write the new binary to the original
// path without hitting EBUSY. On non-Windows platforms this is a no-op
// (Unix allows overwriting a running executable via inode semantics).
//
// Returns a restore function. Caller MUST call it on npm install failure.
//
// Platform-specific implementations are in replace_windows.go and replace_unix.go.

// CleanupStaleFiles removes leftover .old files from previous Windows upgrades.
// If the original binary is missing but .old exists (e.g. crash mid-update),
// it restores the .old to recover the installation.
//
// Platform-specific implementations are in replace_windows.go and replace_unix.go.

// currentExePath resolves the running binary's real path.
func currentExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}
