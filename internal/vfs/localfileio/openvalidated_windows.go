// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package localfileio

import (
	"fmt"
	"os"
	"syscall"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
)

// openValidated opens a path that passed the built-in path policy. Windows has
// no O_NOFOLLOW, so tying the opened handle back to the policy decision rests
// entirely on os.SameFile, which compares volume and file index.
func openValidated(path string) (*os.File, error) {
	pre, err := vfs.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := vfs.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	if err := inspectOpenedFile(f, pre); err != nil {
		f.Close()
		// An unusable target is a bad argument, not an internal fault: callers
		// map ErrPathValidation to a typed validation error, and the fd checks
		// are the same verdict the path checks make, one layer later.
		return nil, &fileio.PathValidationError{Err: err}
	}
	return f, nil
}

// inspectOpenedFile validates the opened handle. pre is nil when there is no
// prior Stat to compare against.
func inspectOpenedFile(f *os.File, pre os.FileInfo) error {
	post, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat opened file: %w", err)
	}
	if pre != nil && !os.SameFile(pre, post) {
		return fmt.Errorf("file changed between validation and open")
	}
	if !post.Mode().IsRegular() {
		return fmt.Errorf("not a regular file (directories, devices, FIFOs, and sockets are refused)")
	}
	var handleInfo syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &handleInfo); err != nil {
		return fmt.Errorf("cannot inspect opened file links: %w", err)
	}
	if handleInfo.NumberOfLinks > 1 {
		return fmt.Errorf("file has multiple hard links, so the other names it can be reached by " +
			"cannot be checked (hint: copy the file and use the copy instead)")
	}
	return nil
}
