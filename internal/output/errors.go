// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ExitError is a structured error that carries an exit code and optional detail.
// It is propagated up the call chain and handled by main.go to produce
// a JSON error envelope on stderr and the correct exit code.
type ExitError struct {
	Code   int
	Detail *ErrDetail
	Err    error
	Raw    bool // when true, skip enrichment (e.g. enrichPermissionError) and preserve original error
}

func (e *ExitError) Error() string {
	if e.Detail != nil {
		return e.Detail.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit %d", e.Code)
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

// WriteErrorEnvelope writes a JSON error envelope for the given ExitError to w.
func WriteErrorEnvelope(w io.Writer, err *ExitError, identity string) {
	if err.Detail == nil {
		return
	}
	env := &ErrorEnvelope{
		OK:       false,
		Identity: identity,
		Error:    err.Detail,
		Notice:   GetNotice(),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		return
	}
	// Encode appends a trailing newline; write directly.
	buf.WriteTo(w)
}

// --- Convenience constructors ---

// Errorf creates an ExitError with the given code, type, and formatted message.
func Errorf(code int, errType, format string, args ...any) *ExitError {
	var err error
	for _, arg := range args {
		if e, ok := arg.(error); ok {
			err = e
			break
		}
	}
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: fmt.Sprintf(format, args...)},
		Err:    err,
	}
}

// ErrValidation creates a validation ExitError (exit 2).
func ErrValidation(format string, args ...any) *ExitError {
	return Errorf(ExitValidation, "validation", format, args...)
}

// ErrAuth creates an auth ExitError (exit 3).
func ErrAuth(format string, args ...any) *ExitError {
	return Errorf(ExitAuth, "auth", format, args...)
}

// ErrNetwork creates a network ExitError (exit 4).
func ErrNetwork(format string, args ...any) *ExitError {
	return Errorf(ExitNetwork, "network", format, args...)
}

// ErrAPI creates an API ExitError using ClassifyLarkError.
// For permission errors, uses a concise message; the raw API response is preserved in Detail.
func ErrAPI(larkCode int, msg string, detail any) *ExitError {
	exitCode, errType, hint := ClassifyLarkError(larkCode, msg)
	if errType == "permission" {
		msg = fmt.Sprintf("Permission denied [%d]", larkCode)
	}
	return &ExitError{
		Code: exitCode,
		Detail: &ErrDetail{
			Type:    errType,
			Code:    larkCode,
			Message: msg,
			Hint:    hint,
			Detail:  detail,
		},
	}
}

// ErrWithHint creates an ExitError with a hint string.
func ErrWithHint(code int, errType, msg, hint string) *ExitError {
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: msg, Hint: hint},
	}
}

// ErrBare creates an ExitError with only an exit code and no envelope.
// Used for cases like `auth check` where the JSON output is already written to stdout.
func ErrBare(code int) *ExitError {
	return &ExitError{Code: code}
}

// MarkRaw sets Raw=true on an ExitError so that enrichment (e.g. enrichPermissionError)
// is skipped and the original API error is preserved. Returns the original error unchanged
// if it is not an ExitError.
func MarkRaw(err error) error {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		exitErr.Raw = true
	}
	return err
}
