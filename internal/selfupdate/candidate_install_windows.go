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
var replaceFilePath = callReplaceFilePath

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
	if err := replaceFilePath(c.target, c.path, backup); err != nil {
		return nil, c.recoverFailedWindowsInstall(backup, err)
	}
	c.path = ""
	return func() { _ = vfs.Remove(backup) }, nil
}

func callReplaceFilePath(targetPath, replacementPath, backupPath string) error {
	target, err := windows.UTF16PtrFromString(targetPath)
	if err != nil {
		return err
	}
	replacement, err := windows.UTF16PtrFromString(replacementPath)
	if err != nil {
		return err
	}
	var backup uintptr
	if backupPath != "" {
		ptr, err := windows.UTF16PtrFromString(backupPath)
		if err != nil {
			return err
		}
		backup = uintptr(unsafe.Pointer(ptr))
	}
	if result, _, callErr := replaceFile.Call(
		uintptr(unsafe.Pointer(target)),
		uintptr(unsafe.Pointer(replacement)),
		backup,
		0, 0, 0,
	); result == 0 {
		return fmt.Errorf("ReplaceFileW: %w", callErr)
	}
	return nil
}

// recoverFailedWindowsInstall restores the old executable before returning an
// install error. If Windows cannot restore it, preserve both recovery files so
// Candidate.Cleanup cannot remove the only usable copy.
func (c *Candidate) recoverFailedWindowsInstall(backup string, installErr error) error {
	if _, err := vfs.Stat(backup); err == nil {
		var restoreErr error
		if _, targetErr := vfs.Stat(c.target); errors.Is(targetErr, fs.ErrNotExist) {
			restoreErr = vfs.Rename(backup, c.target)
		} else if targetErr != nil {
			restoreErr = targetErr
		} else {
			restoreErr = replaceFilePath(c.target, backup, "")
		}
		if restoreErr == nil {
			return installErr
		}
		return c.windowsRecoveryRequired(backup, installErr, restoreErr)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return c.windowsRecoveryRequired(backup, installErr, err)
	}
	if _, err := vfs.Stat(c.target); err != nil {
		return c.windowsRecoveryRequired(backup, installErr, err)
	}
	return installErr
}

func (c *Candidate) windowsRecoveryRequired(backup string, installErr, recoveryErr error) error {
	candidate := c.path
	c.path = "" // preserve the candidate for manual recovery
	return fmt.Errorf("%w; automatic recovery failed: %v; preserved backup %q and candidate %q", installErr, recoveryErr, backup, candidate)
}
