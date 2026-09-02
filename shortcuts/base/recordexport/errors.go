// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package recordexport

import "fmt"

// detailError is an intermediate conversion/encoding error. The Base command
// boundary classifies it as a typed invalid-response error before it can reach
// the user.
type detailError struct {
	message string
	cause   error
}

func (e *detailError) Error() string { return e.message }

func (e *detailError) Unwrap() error { return e.cause }

func newDetailErrorf(format string, args ...any) error {
	return &detailError{message: fmt.Sprintf(format, args...)}
}

func wrapDetailError(message string, cause error) error {
	return &detailError{message: message + ": " + cause.Error(), cause: cause}
}

func wrapDetailErrorf(cause error, format string, args ...any) error {
	return wrapDetailError(fmt.Sprintf(format, args...), cause)
}
