// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"os"
	"syscall"

	"github.com/larksuite/cli/internal/vfs"
)

// hasExtraHardLinks reports whether the file at path carries more than one
// name. The count lives behind a handle on Windows, so the file is opened just
// to ask; it reports false when the count cannot be determined, since callers
// treat it as one signal among several rather than as the sole guard.
func hasExtraHardLinks(path string, _ os.FileInfo) bool {
	f, err := vfs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer f.Close()

	var handleInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &handleInfo); err != nil {
		return false
	}
	return handleInfo.NumberOfLinks > 1
}
