// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
)

// PartialFailureError is the exit signal for a batch / multi-status command that
// has already written an ok:false result envelope to stdout. The per-item
// outcomes are the primary, machine-readable output and live on stdout, so the
// dispatcher sets only the exit code and writes nothing to stderr.
//
// It is deliberately distinct from ErrBare (the stdout-carries-the-answer
// silent-exit signal) so that contract stays narrow, and from a typed *errs.XxxError
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
//     fails, return false so the dispatcher handles it via its signal /
//     usage-error branches; nothing is written to w.
//  2. Best-effort write of the serialized bytes to w. A partial write is
//     accepted (return value still true): the typed exit code has already
//     been determined upstream by handleRootError calling ExitCodeOf(err)
//     before this writer runs, so a torn envelope on stderr must not
//     downgrade the caller's typed exit (3/4/6/10) to plain 1. Consumers
//     parse-or-skip on malformed JSON.
//
// Returns true when err was a typed error and serialization succeeded.
// Returns false only when err carries no Problem (the dispatcher then handles
// it via its signal / usage-error branches) or when JSON encoding itself failed.
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
		// Encoding failed — emit nothing here; the dispatcher's fall-through
		// branches still surface the error, so stderr is never blank.
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
// encoding/json reflection; Notice mirrors the success Envelope's notice (see
// GetNotice in envelope.go).
type typedEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    error                  `json:"error"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}
