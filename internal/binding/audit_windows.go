// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build windows

package binding

import (
	"fmt"

	"github.com/larksuite/cli/internal/vfs"
)

// checkOwnerUID is a no-op on Windows where Unix UID semantics don't apply.
func checkOwnerUID(path, label string) error {
	return nil
}

// auditFilePermissions skips POSIX permission-bit auditing on Windows because
// Go synthesizes mode bits from file attributes rather than NTFS ACLs.
func auditFilePermissions(effectivePath string, allowReadableByOthers bool, label string) error {
	if _, err := vfs.Stat(effectivePath); err != nil {
		return fmt.Errorf("%s: cannot stat %q: %w", label, effectivePath, err)
	}
	return nil
}
