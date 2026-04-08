// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package fileio

import "errors"

// Sentinel errors for FileIO operations. Callers can use errors.Is to
// distinguish error categories and wrap with their own messages.

// ErrPathValidation indicates the path failed security validation
// (traversal, absolute, control chars, symlink escape, etc.).
var ErrPathValidation = errors.New("path validation failed")

// ErrMkdir indicates parent directory creation failed.
var ErrMkdir = errors.New("directory creation failed")

// PathValidationError wraps a path validation error with ErrPathValidation.
// Both the sentinel (ErrPathValidation) and the original error are
// reachable via errors.Is / errors.As.
type PathValidationError struct {
	Err error
}

func (e *PathValidationError) Error() string { return e.Err.Error() }

// Unwrap returns both the sentinel and the original error so that
// errors.Is(err, ErrPathValidation) and errors.Is(err, os.ErrPermission)
// (or any OS error in the chain) both work.
func (e *PathValidationError) Unwrap() []error {
	return []error{ErrPathValidation, e.Err}
}

// MkdirError wraps a directory creation error with ErrMkdir.
type MkdirError struct {
	Err error
}

func (e *MkdirError) Error() string { return e.Err.Error() }

func (e *MkdirError) Unwrap() []error {
	return []error{ErrMkdir, e.Err}
}
