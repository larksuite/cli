// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package validate

import (
	"io"
	"os"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

// AtomicWrite writes data to path atomically.
// Delegates to localfileio.AtomicWrite.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return localfileio.AtomicWrite(path, data, perm)
}

// AtomicWriteFromReader atomically copies reader contents into path.
// Delegates to localfileio.AtomicWriteFromReader.
func AtomicWriteFromReader(path string, reader io.Reader, perm os.FileMode) (int64, error) {
	return localfileio.AtomicWriteFromReader(path, reader, perm)
}
