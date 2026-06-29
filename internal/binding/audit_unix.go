// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package binding

import (
	"fmt"
	"os"
	"syscall"

	"github.com/larksuite/cli/internal/vfs"
)

// checkOwnerUID verifies the file is owned by the current user.
func checkOwnerUID(path, label string) error {
	stat, err := vfs.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: cannot stat %q: %w", label, path, err)
	}
	sysStat, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: cannot retrieve file owner for %q", label, path)
	}
	if sysStat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("%s: path %q is owned by uid %d, expected %d",
			label, path, sysStat.Uid, os.Getuid())
	}
	return nil
}

// auditFilePermissions rejects world/group-writable modes (always) and
// world/group-readable modes (unless allowReadableByOthers is true, which
// exec commands typically need for their usual 755 mode).
func auditFilePermissions(effectivePath string, allowReadableByOthers bool, label string) error {
	info, err := vfs.Stat(effectivePath)
	if err != nil {
		return fmt.Errorf("%s: cannot stat %q: %w", label, effectivePath, err)
	}
	mode := info.Mode().Perm()

	if mode&0o002 != 0 {
		return fmt.Errorf("%s: path %q is world-writable (mode %04o)", label, effectivePath, mode)
	}
	if mode&0o020 != 0 {
		return fmt.Errorf("%s: path %q is group-writable (mode %04o)", label, effectivePath, mode)
	}
	if allowReadableByOthers {
		return nil
	}
	if mode&0o004 != 0 {
		return fmt.Errorf("%s: path %q is world-readable (mode %04o)", label, effectivePath, mode)
	}
	if mode&0o040 != 0 {
		return fmt.Errorf("%s: path %q is group-readable (mode %04o)", label, effectivePath, mode)
	}
	return nil
}
