// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
)

// withStepContext annotates err with multi-step context (e.g. which steps
// already completed, or that a rollback ran) while preserving the underlying
// failure's classification. An already-typed error keeps its own
// category/subtype/code/log_id; we only append the formatted context to its
// Hint so the top-level envelope still tells the truth about what failed.
// Only an unclassified error falls back to a typed internal wrap.
func withStepContext(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	extra := fmt.Sprintf(format, args...)
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + extra
		} else {
			p.Hint = extra
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s", err.Error()).WithHint(extra).WithCause(err)
}

// withParam attaches the offending flag to a typed validation error, preserving
// the original error instead of re-wrapping it. Non-validation errors pass through.
func withParam(err error, flag string) error {
	var ve *errs.ValidationError
	if errors.As(err, &ve) {
		return ve.WithParam(flag)
	}
	return err
}

// unwrapCalendarAPIError returns a user-facing message extracted from a
// calendar business-domain *errs.APIError, or "" when the error is not an
// APIError or its Code is not specialized here. Callers should fall back to
// err.Error() on "".
//
// Today it handles:
//   - 190014 (invalid_parameters): returns Problem.Hint, which carries the
//     server-supplied field-level detail (e.g. "end_time should be later
//     than start_time") lifted by errclass.BuildAPIError.
//
// Add additional 19xxxx codes here as they become worth surfacing — keep this
// the single switch site so call sites stay readable.
func unwrapCalendarAPIError(err error) string {
	if err == nil {
		return ""
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		return ""
	}
	switch ae.Code {
	case 190014:
		return ae.Hint
	}
	return ""
}
