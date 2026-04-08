// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package localfileio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/vfs"
)

// AtomicWrite writes data to path atomically via temp file + rename.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, perm, func(tmp *os.File) error {
		_, err := tmp.Write(data)
		return err
	})
}

// AtomicWriteFromReader atomically copies reader contents into path.
func AtomicWriteFromReader(path string, reader io.Reader, perm os.FileMode) (int64, error) {
	var copied int64
	err := atomicWrite(path, perm, func(tmp *os.File) error {
		n, err := io.Copy(tmp, reader)
		copied = n
		return err
	})
	if err != nil {
		return 0, err
	}
	return copied, nil
}

func atomicWrite(path string, perm os.FileMode, writeFn func(tmp *os.File) error) error {
	dir := filepath.Dir(path)
	tmp, err := vfs.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	closed := false
	success := false
	defer func() {
		if !success {
			if !closed {
				tmp.Close()
			}
			vfs.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := writeFn(tmp); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	closed = true
	if err := vfs.Rename(tmpName, path); err != nil {
		return err
	}
	success = true
	return nil
}
