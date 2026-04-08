// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package fileio

import "errors"

// ErrPathValidation indicates the path failed security validation
// (traversal, absolute, control chars, symlink escape, etc.).
var ErrPathValidation = errors.New("path validation failed")

// PathValidationError wraps a path validation error.
// errors.Is(err, ErrPathValidation) returns true.
// errors.Is(err, <original OS error>) also works via the chain.
type PathValidationError struct {
	Err error // original error
}

func (e *PathValidationError) Error() string { return e.Err.Error() }
func (e *PathValidationError) Unwrap() []error {
	return []error{ErrPathValidation, e.Err}
}

// MkdirError indicates parent directory creation failed.
// Use errors.As(err, &fileio.MkdirError{}) to match.
type MkdirError struct {
	Err error
}

func (e *MkdirError) Error() string { return e.Err.Error() }
func (e *MkdirError) Unwrap() error { return e.Err }

// WriteError indicates file write failed.
// Use errors.As(err, &fileio.WriteError{}) to match.
type WriteError struct {
	Err error
}

func (e *WriteError) Error() string { return e.Err.Error() }
func (e *WriteError) Unwrap() error { return e.Err }
