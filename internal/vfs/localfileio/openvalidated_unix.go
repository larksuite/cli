// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !windows

package localfileio

import (
	"fmt"
	"os"
	"syscall"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/vfs"
)

// hardenedOpenFlags pins the final path component and keeps a FIFO from
// wedging the process before the fd can be inspected: O_NOFOLLOW makes the
// kernel refuse a symlink swapped in after validation, O_NONBLOCK makes the
// open of a writer-less FIFO return instead of blocking. Blocking mode is
// restored once the fd is known to be a regular file.
const hardenedOpenFlags = os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK

// openValidated opens a path that passed the built-in path policy. The
// pre-open Stat exists so the fd can be compared against it: policy decided
// on a path, and SameFile is what ties the opened object back to that
// decision. Hard links are refused here because an extra link is how a
// denylisted file gets smuggled into an allowed directory.
//
// The link check is deliberately blunt: it refuses any file with more than one
// name, including one whose every name sits inside an allowed root, because
// the filesystem offers no way to enumerate the other names. Content-addressed
// package layouts (pnpm, nix) link into a shared store outside the allowlist
// and are refused for the same reason. Narrowing this to "another name is
// inside a deny root" needs an index of deny-root inodes; until then the
// refusal is accepted and the message says how to work around it.
func openValidated(path string) (*os.File, error) {
	pre, err := vfs.Stat(path)
	if err != nil {
		return nil, err
	}
	f, err := vfs.OpenFile(path, hardenedOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	if err := inspectOpenedFile(f, pre, true); err != nil {
		f.Close()
		// An unusable target is a bad argument, not an internal fault: callers
		// map ErrPathValidation to a typed validation error, and the fd checks
		// are the same verdict the path checks make, one layer later.
		return nil, &fileio.PathValidationError{Err: err}
	}
	return f, nil
}

// inspectOpenedFile validates the opened fd and restores blocking mode. pre is
// nil when there is no prior Stat to compare against.
func inspectOpenedFile(f *os.File, pre os.FileInfo, rejectHardLinks bool) error {
	post, err := f.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat opened file: %w", err)
	}
	if pre != nil && !os.SameFile(pre, post) {
		return fmt.Errorf("file changed between validation and open")
	}
	if !post.Mode().IsRegular() {
		return fmt.Errorf("not a regular file (directories, devices, FIFOs, and sockets are refused)")
	}
	if rejectHardLinks {
		if st, ok := post.Sys().(*syscall.Stat_t); ok && st.Nlink > 1 {
			return fmt.Errorf("file has multiple hard links, so the other names it can be reached by " +
				"cannot be checked (hint: copy the file and use the copy instead)")
		}
	}
	if err := syscall.SetNonblock(int(f.Fd()), false); err != nil {
		return fmt.Errorf("cannot restore blocking mode: %w", err)
	}
	return nil
}
