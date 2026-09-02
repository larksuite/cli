// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package selfupdate

import (
	"errors"
	"fmt"
	"io/fs"
	"unsafe"

	"github.com/larksuite/cli/internal/vfs"
	"golang.org/x/sys/windows"
)

var replaceFile = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

// install uses ReplaceFileW so replacing a running executable and creating its
// backup are one filesystem operation; a crash cannot leave the target absent.
func (c *Candidate) install() (func(), error) {
	backup := c.target + ".old"
	if _, err := vfs.Stat(c.target); errors.Is(err, fs.ErrNotExist) {
		if err := vfs.Rename(c.path, c.target); err != nil {
			return nil, err
		}
		c.path = ""
		return func() {}, nil
	} else if err != nil {
		return nil, err
	}
	if err := vfs.Remove(backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove stale binary backup: %w", err)
	}
	target, err := windows.UTF16PtrFromString(c.target)
	if err != nil {
		return nil, err
	}
	replacement, err := windows.UTF16PtrFromString(c.path)
	if err != nil {
		return nil, err
	}
	old, err := windows.UTF16PtrFromString(backup)
	if err != nil {
		return nil, err
	}
	if result, _, callErr := replaceFile.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(unsafe.Pointer(replacement)),
		uintptr(unsafe.Pointer(old)),
		0, 0, 0,
	); result == 0 {
		return nil, fmt.Errorf("replace binary: %w", callErr)
	}
	c.path = ""
	return func() { _ = vfs.Remove(backup) }, nil
}
