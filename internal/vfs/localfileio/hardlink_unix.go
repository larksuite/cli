// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

import (
	"os"
	"syscall"
)

// hasExtraHardLinks reports whether the file at path carries more than one
// name. It reports false when the count cannot be determined: callers treat it
// as one signal among several rather than as the sole guard.
func hasExtraHardLinks(_ string, info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1
}
