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

// ExclusiveWriteFromReader copies reader contents into path only when path does
// not already exist, and reports an error satisfying errors.Is(err, fs.ErrExist)
// when it does.
//
// Content is written to a temp file in the same directory and committed with
// Link, which combines both guarantees this call has to make:
//
//   - No-clobber. Link fails with EEXIST instead of replacing an existing
//     target, so the refusal is decided by the commit itself. A preceding
//     existence check cannot do this: another writer may create the file while
//     this one is still copying.
//   - Whole-file visibility. The target name appears only once the content is
//     complete and synced. Writing directly to the final name with O_EXCL would
//     satisfy no-clobber but publish a partial file for the duration of the
//     copy, and a killed process would leave that partial file behind as a
//     phantom target for the next attempt.
//
// Rename cannot serve as the commit step because it replaces an existing target
// unconditionally.
func ExclusiveWriteFromReader(path string, reader io.Reader, perm os.FileMode) (int64, error) {
	dir := filepath.Dir(path)
	tmp, err := vfs.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	closed := false
	defer func() {
		if !closed {
			tmp.Close()
		}
		// The temp name is removed either way: on failure it is the partial
		// artifact, on success the link has already published the content.
		vfs.Remove(tmpName)
	}()

	if err := tmp.Chmod(perm); err != nil {
		return 0, err
	}
	copied, err := io.Copy(tmp, reader)
	if err != nil {
		return 0, err
	}
	if err := tmp.Sync(); err != nil {
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	closed = true
	if err := vfs.Link(tmpName, path); err != nil {
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
