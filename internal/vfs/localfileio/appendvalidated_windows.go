// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"errors"
	"io/fs"
	"os"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
)

// openAppendValidated applies the same pre-open identity and regular-file
// checks as Open. Windows has no O_NOFOLLOW equivalent in the os package, so
// inspectOpenedFile ties the opened handle back to the pre-open identity.
func openAppendValidated(path string, perm os.FileMode) (*os.File, error) {
	pre, err := vfs.Stat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if errors.Is(err, fs.ErrNotExist) {
		pre = nil
	}

	f, err := vfs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, perm)
	if err != nil {
		return nil, err
	}
	if err := inspectOpenedFile(f, pre); err != nil {
		_ = f.Close()
		return nil, &fileio.PathValidationError{Err: err}
	}
	return f, nil
}
