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
