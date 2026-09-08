// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

import "github.com/larksuite/cli/internal/vfs"

// replaceResumeArtifact uses rename's atomic replacement semantics on Unix.
func replaceResumeArtifact(partialPath, targetPath string) error {
	return vfs.Rename(partialPath, targetPath)
}
