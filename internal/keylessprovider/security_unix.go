// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build darwin || linux

package keylessprovider

import (
	"fmt"
	"os"
	"syscall"

	"github.com/larksuite/cli/internal/vfs"
)

func validateProviderObject(path string, wantDir bool) error {
	return validateOwnedObject(path, wantDir)
}

func validateInspectExecutable(path string) error {
	info, err := vfs.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect OpenClaw executable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("OpenClaw executable must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("OpenClaw executable is group/world writable")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() && stat.Uid != 0 {
		return fmt.Errorf("OpenClaw executable is not owned by the current user or root")
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("OpenClaw executable must have exactly one hard link")
	}
	return nil
}

func validateOwnedObject(path string, wantDir bool) error {
	info, err := vfs.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect provider object %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("provider object must not be a symlink: %s", path)
	}
	if wantDir != info.IsDir() || (!wantDir && !info.Mode().IsRegular()) {
		return fmt.Errorf("provider object has unexpected type: %s", path)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("provider object is group/world writable: %s (mode %o)", path, info.Mode().Perm())
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("provider object is not owned by the current user: %s", path)
	}
	if !wantDir && stat.Nlink != 1 {
		return fmt.Errorf("provider file must have exactly one hard link: %s", path)
	}
	return nil
}
