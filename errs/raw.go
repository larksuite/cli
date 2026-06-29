// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import "errors"

// rawPassthrough marks an error as raw passthrough: the dispatcher must not
// rewrite its message or hint with local enrichment. Raw is
// dispatcher-internal routing state, not a wire field. It is deliberately not
// a typed taxonomy error (no embedded Problem) — it only wraps one.
type rawPassthrough struct{ err error }

func (e *rawPassthrough) Error() string { return e.err.Error() }
func (e *rawPassthrough) Unwrap() error { return e.err }

// MarkRaw wraps err as raw passthrough. MarkRaw(nil) returns nil.
func MarkRaw(err error) error {
	if err == nil {
		return nil
	}
	return &rawPassthrough{err: err}
}

// IsRaw reports whether err or any error in its chain is marked raw.
func IsRaw(err error) bool {
	var raw *rawPassthrough
	return errors.As(err, &raw)
}
