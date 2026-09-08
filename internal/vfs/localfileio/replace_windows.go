// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import "golang.org/x/sys/windows"

// replaceResumeArtifact uses MoveFileEx(REPLACE_EXISTING), which keeps the
// existing target in place when the replacement cannot be completed.
func replaceResumeArtifact(partialPath, targetPath string) error {
	return windows.Rename(partialPath, targetPath)
}
