// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
)

// ExitError is a structured error that carries an exit code and optional detail.
// It is propagated up the call chain and handled by main.go to produce
// a JSON error envelope on stderr and the correct exit code.
//
// Deprecated: legacy error type. Return a typed *errs.XxxError instead
// (see errs/types.go).
type ExitError struct {
	Code   int
	Detail *ErrDetail
	Err    error
	Raw    bool // when true, the dispatcher skips enrichment (e.g. enrichPermissionError) and preserves the original error detail
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

// MarkRaw sets Raw=true on an ExitError so that the dispatcher skips
// enrichment (e.g. enrichPermissionError, enrichMissingScopeError) and
// preserves the upstream message verbatim. Returns the original error
// unchanged if it is not (or does not wrap) an ExitError.
//
// Used by `cmd/api` and other "passthrough" call sites where the caller
// wants the original Lark response wording rather than the enriched
// message/hint variant.
func MarkRaw(err error) error {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		exitErr.Raw = true
	}
	return err
}

// WriteErrorEnvelope writes a JSON error envelope for the given ExitError to w.
//
// Deprecated: legacy envelope writer. Typed errors are dispatched by
// cmd/root.go through WriteTypedErrorEnvelope.
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
//
// Deprecated: construct a typed *errs.XxxError directly
// (e.g. errs.NewValidationError, errs.NewInternalError).
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

// ErrValidation creates a validation ExitError (exit 2, wire type
// "validation"). The legacy envelope emits only `type`+`message`; for
// `subtype` / `param` extension fields, construct a typed
// *errs.ValidationError directly.
func ErrValidation(format string, args ...any) *ExitError {
	return Errorf(ExitValidation, "validation", format, args...)
}

// ErrAuth creates an authentication ExitError (exit 3, wire type "auth").
//
// New code should construct a typed *errs.AuthenticationError directly;
// the typed envelope emits the canonical `type: "authentication"`.
// Migrating an existing call site flips a user-visible wire field.
func ErrAuth(format string, args ...any) *ExitError {
	return Errorf(ExitAuth, "auth", format, args...)
}

// ErrNetwork creates a network ExitError (exit 4, wire type "network").
// The legacy envelope emits only `type`+`message`; for `subtype`
// ("transport" / "timeout" / "tls" / "dns") and retryable hint extension
// fields, construct a typed *errs.NetworkError directly.
func ErrNetwork(format string, args ...any) *ExitError {
	return Errorf(ExitNetwork, "network", format, args...)
}

// ErrAPI creates an API ExitError using ClassifyLarkError.
// For permission errors, uses a concise message; the raw API response is preserved in Detail.
//
// Deprecated: route through errclass.BuildAPIError, which emits typed
// *errs.PermissionError / *errs.AuthenticationError / etc. with
// MissingScopes, ConsoleURL, and Identity at the source.
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
//
// Deprecated: construct a typed *errs.XxxError directly and set its Hint
// field; the typed envelope promotes Problem.Hint to the wire.
func ErrWithHint(code int, errType, msg, hint string) *ExitError {
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: msg, Hint: hint},
	}
}

// ErrBare creates an ExitError with only an exit code and no envelope.
// The predicate-command silent-exit signal: stdout has already been
// written and the caller wants the matching exit code without a stderr
// envelope (e.g. `auth check` emitting its JSON result and then exiting
// non-zero on a no-token state). Outside the typed-envelope contract.
func ErrBare(code int) *ExitError {
	return &ExitError{Code: code}
}

// PartialFailureError is the exit signal for a batch / multi-status command that
// has already written an ok:false result envelope to stdout. The per-item
// outcomes are the primary, machine-readable output and live on stdout, so the
// dispatcher sets only the exit code and writes nothing to stderr.
//
// It is deliberately distinct from ErrBare (the predicate silent-exit signal)
// so the predicate contract stays narrow, and from a typed *errs.XxxError
// (which owns the stderr error envelope): a partial failure is a result, not an
// error envelope.
type PartialFailureError struct {
	Code int
}

func (e *PartialFailureError) Error() string {
	return fmt.Sprintf("partial failure (exit %d)", e.Code)
}

// PartialFailure builds the partial-failure exit signal with the given code.
func PartialFailure(code int) *PartialFailureError {
	return &PartialFailureError{Code: code}
}

// WriteTypedErrorEnvelope writes the JSON error envelope for a typed error.
// Each typed error owns its wire shape via its own struct tags: Problem fields
// are promoted to the top level through embedding, and extension fields
// (MissingScopes, ChallengeURL, etc.) sit alongside as siblings — not inside
// a `detail` sub-object.
//
// Two-stage write:
//
//  1. Serialize the envelope into an in-memory buffer. If serialization
//     fails, return false so the dispatcher falls back to the legacy
//     envelope path; nothing is written to w.
//  2. Best-effort write of the serialized bytes to w. A partial write is
//     accepted (return value still true): the typed exit code has already
//     been determined upstream by handleRootError calling ExitCodeOf(err)
//     before this writer runs, so a torn envelope on stderr must not
//     downgrade the caller's typed exit (3/4/6/10) to plain 1. Consumers
//     parse-or-skip on malformed JSON.
//
// Returns true when err was a typed error and serialization succeeded.
// Returns false only when err carries no Problem (caller should fall back
// to WriteErrorEnvelope) or when JSON encoding itself failed.
func WriteTypedErrorEnvelope(w io.Writer, err error, identity string) bool {
	typed, ok := errs.UnwrapTypedError(err)
	if !ok {
		return false
	}
	env := typedEnvelope{
		OK:       false,
		Identity: identity,
		Error:    typed,
		Notice:   GetNotice(),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(env); encErr != nil {
		// Encoding failed — emit nothing here and let the dispatcher fall
		// back to the legacy envelope writer so stderr is never blank.
		return false
	}
	// Best-effort write. Partial-write does not downgrade the success status:
	// the dispatcher has already captured ExitCodeOf(err) before calling us,
	// and a torn stderr is preferable to falling through to the plain
	// "Error:" path with exit 1.
	_, _ = w.Write(buf.Bytes())
	return true
}

// typedEnvelope wraps a typed error for wire emission. Error is `error` so the
// underlying typed error's own json tags determine the inner shape via
// encoding/json reflection; Notice mirrors the existing ErrorEnvelope (see
// GetNotice in envelope.go).
type typedEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    error                  `json:"error"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}
