// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/urlrewrite"
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

type upstreamError struct {
	error
	detail json.RawMessage
}

func (e *upstreamError) Unwrap() error { return e.error }

// WithUpstreamError attaches an optional upstream detail for stderr rendering.
func WithUpstreamError(err error, detail json.RawMessage) error {
	if err == nil || len(detail) == 0 {
		return err
	}
	return &upstreamError{error: err, detail: detail}
}

// UpstreamErrorOf returns the detail attached to an error chain, if present.
func UpstreamErrorOf(err error) json.RawMessage {
	var carrier *upstreamError
	if errors.As(err, &carrier) {
		return carrier.detail
	}
	return nil
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
		Error:    withEffectiveRequestURL(typed),
		Notice:   GetNotice(),
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if detail := UpstreamErrorOf(err); len(detail) != 0 {
		if err := enc.Encode(env.Error); err != nil {
			return false
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(buf.Bytes(), &fields); err != nil || fields == nil {
			return false
		}
		buf.Reset()
		fields["upstream_error"] = detail
		env.Error = fields
	}
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

// net/http records the original URL even when a transport rewrites it. Replace
// only that error fragment, including for raw API transport failures; server
// payloads and the producer's error chain remain untouched.
func withEffectiveRequestURL(err error) error {
	var requestErr *url.Error
	var effective urlrewrite.RequestError
	if !errors.As(err, &requestErr) || !errors.As(requestErr.Err, &effective) {
		return err
	}
	rendered, ok := recovery.CloneTyped(err)
	if !ok {
		return err
	}
	cloned := *requestErr
	cloned.URL = effective.EffectiveRequestURL()
	problem, _ := errs.ProblemOf(rendered)
	problem.Message = strings.ReplaceAll(problem.Message, requestErr.Error(), cloned.Error())
	return rendered
}

// typedEnvelope uses the concrete typed error's JSON shape, with optional
// upstream detail added alongside its fields. Notice mirrors the success envelope.
type typedEnvelope struct {
	OK       bool                   `json:"ok"`
	Identity string                 `json:"identity,omitempty"`
	Error    any                    `json:"error"`
	Notice   map[string]interface{} `json:"_notice,omitempty"`
}
