// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "github.com/larksuite/cli/internal/vfs"

// SetDefaultFS replaces the global filesystem implementation used by internal
// packages. The provided fs must implement the vfs.FS interface. If fs is nil,
// the default OS filesystem is restored.
//
// Call this before Build or Execute to take effect.
func SetDefaultFS(fs vfs.FS) {
	if fs == nil {
		fs = vfs.OsFs{}
	}
	vfs.DefaultFS = fs
}
