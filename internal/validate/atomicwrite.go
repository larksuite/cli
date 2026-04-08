// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"os"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// AtomicWrite delegates to localfileio.AtomicWrite.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return localfileio.AtomicWrite(path, data, perm)
}
