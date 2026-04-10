// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package selfupdate

// PrepareSelfReplace is a no-op on Unix.
// Unix allows overwriting a running executable via inode semantics.
func PrepareSelfReplace() (restore func(), err error) {
	return func() {}, nil
}

// CleanupStaleFiles is a no-op on Unix (no .old files are created).
func CleanupStaleFiles() {}
