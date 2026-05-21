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
// Deprecated: *output.ExitError is the legacy error type that predates the
// typed error contract introduced by errs/. New code MUST NOT instantiate it
// — return a typed *errs.XxxError (see errs/ for the available categories:
// *AuthenticationError / *PermissionError / *ValidationError / *NetworkError /
// *APIError / *InternalError / etc.). This type is retained only while
// existing call sites are migrated; it will be removed once they have moved
// to the typed surface.
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
// preserves the original API error detail. Returns the original error
// unchanged if it is not (or does not wrap) an ExitError.
//
// Used by `cmd/api` and other "passthrough" call sites where the caller
// explicitly wants the raw Lark API detail (log_id, troubleshooter, etc.)
// on the wire rather than the enriched message/hint variant.
func MarkRaw(err error) error {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		exitErr.Raw = true
	}
	return err
}

// WriteErrorEnvelope writes a JSON error envelope for the given ExitError to w.
//
// Deprecated: WriteErrorEnvelope is the legacy envelope writer paired with
// *output.ExitError, which predates the typed error contract introduced by
// errs/. New code MUST NOT call this directly — return a typed *errs.XxxError
// from the command, and cmd/root.go handleRootError will dispatch through
// WriteTypedErrorEnvelope. This writer is retained only while existing
// *ExitError producers are migrated; it will be removed once they have moved
// to the typed surface.
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
// Deprecated: Errorf belongs to the legacy *output.ExitError surface that
// predates the typed error contract introduced by errs/. New code MUST NOT
// use it — construct a typed *errs.XxxError directly (e.g.
// *errs.ValidationError, *errs.InternalError). This helper is retained only
// while existing call sites are migrated; it will be removed once they have
// moved to the typed surface.
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
// "validation"). The legacy *output.ExitError envelope emits only
// `type`+`message` — no `subtype`/`param` extension fields.
//
// Stage-1 status: still acceptable to use in new code that only needs the
// (type, message) pair. To carry extension fields (Subtype, Param, etc.)
// on the wire, construct `&errs.ValidationError{...}` directly so
// cmd/root.go routes it through the typed envelope writer. Per-domain
// typed migration in stage 2+ will migrate existing call sites and
// remove this helper.
func ErrValidation(format string, args ...any) *ExitError {
	return Errorf(ExitValidation, "validation", format, args...)
}

// ErrAuth creates an authentication ExitError (exit 3, wire type "auth").
//
// Stage-1 status: kept as the canonical helper for token-missing /
// login-required errors, so the 19 existing call sites in cmd/auth,
// cmd/config, cmd/event, internal/client, and shortcuts/common keep
// emitting `type: "auth"`. To migrate a single call site to the typed
// taxonomy (`type: "authentication"` on the wire), construct
// `&errs.AuthenticationError{...}` directly — but note that flips a
// user-visible wire field and belongs in the per-domain stage-2 PR for
// that area, not in unrelated new code.
func ErrAuth(format string, args ...any) *ExitError {
	return Errorf(ExitAuth, "auth", format, args...)
}

// ErrNetwork creates a network ExitError (exit 4, wire type "network").
// The legacy *output.ExitError envelope emits only `type`+`message` — no
// `subtype`/`cause` extension fields.
//
// Stage-1 status: still acceptable to use in new code that only needs the
// (type, message) pair. To carry extension fields (Subtype "transport" /
// "timeout" / "tls" / "dns", retryable hint, etc.) on the wire, construct
// `&errs.NetworkError{...}` directly. Per-domain typed migration in
// stage 2+ will migrate existing call sites and remove this helper.
func ErrNetwork(format string, args ...any) *ExitError {
	return Errorf(ExitNetwork, "network", format, args...)
}

// ErrAPI creates an API ExitError using ClassifyLarkError.
// For permission errors, uses a concise message; the raw API response is preserved in Detail.
//
// Deprecated: ErrAPI belongs to the legacy *output.ExitError surface that
// predates the typed error contract introduced by errs/. New code SHOULD
// construct a typed *errs.XxxError directly. The stage-2+ migration will
// route classification through internal/errclass.BuildAPIError (shipped
// but not yet invoked from production paths) so the typed envelope carries
// Category, Subtype, MissingScopes, ConsoleURL, and Identity from the
// source. This helper is retained only while existing call sites are
// migrated; it will be removed once they have moved to the typed surface.
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
// Deprecated: ErrWithHint belongs to the legacy *output.ExitError surface
// that predates the typed error contract introduced by errs/. New code MUST
// NOT use it — construct a typed *errs.XxxError directly and set its Hint
// field (the typed envelope promotes Problem.Hint to the wire). This helper
// is retained only while existing call sites are migrated; it will be
// removed once they have moved to the typed surface.
func ErrWithHint(code int, errType, msg, hint string) *ExitError {
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: msg, Hint: hint},
	}
}

// ErrBare creates an ExitError with only an exit code and no envelope.
// Used for cases like `auth check` where the JSON output is already written to stdout.
//
// Deprecated: ErrBare belongs to the legacy *output.ExitError surface that
// predates the typed error contract introduced by errs/. New code MUST NOT
// use it — express the "exit with code, emit no envelope" semantics
// explicitly at the call site (e.g. return a typed *errs.XxxError or call
// os.Exit directly from RunE). This helper is retained only while existing
// call sites are migrated; it will be removed once they have moved to the
// typed surface.
func ErrBare(code int) *ExitError {
	return &ExitError{Code: code}
}

// WriteTypedErrorEnvelope writes the JSON error envelope for a typed error.
// Each typed error owns its wire shape via its own struct tags: Problem fields
// are promoted to the top level through embedding, and extension fields
// (MissingScopes, ChallengeURL, etc.) sit alongside as siblings — not inside
// a `detail` sub-object.
//
// Returns true when err was a typed error (envelope written) and false when
// err had no Problem (caller should fall back to WriteErrorEnvelope).
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
	if _, writeErr := buf.WriteTo(w); writeErr != nil {
		// Write failed mid-envelope. Return false so the dispatcher does
		// not silently treat a half-written stderr as a successful emit
		// and skip every other fallback.
		return false
	}
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
