// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"io/fs"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

// StatLocalFile returns metadata for a path in the process filesystem namespace.
// It is intended for advisory validation; callers must validate the opened file
// again before using its contents.
func StatLocalFile(path string) (fs.FileInfo, error) {
	localPath, err := validate.LocalInputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	return vfs.Stat(localPath)
}

// OpenLocalFile opens a path in the process filesystem namespace.
// Absolute and relative paths are accepted. It is the shared replacement for
// direct os.Open/os.ReadFile use in commands that intentionally read local
// paths outside the workspace sandbox. Callers inspect the returned descriptor
// before reading so validation and use apply to the same opened file.
func OpenLocalFile(path string) (fs.File, error) {
	localPath, err := validate.LocalInputPath(path)
	if err != nil {
		return nil, &fileio.PathValidationError{Err: err}
	}
	return vfs.Open(localPath)
}
