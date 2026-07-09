// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// slidesInputStatError maps a FileIO.Stat error for an input image path to a
// typed validation error, prefixing the caller's context message and tagging
// the offending flag via param so callers route on the typed Param rather than
// parsing the message. Both path validation failures and other stat errors are
// user-actionable input problems (exit code 2). Already-typed errors are not
// expected here (Stat returns raw fs errors), so this always classifies as
// validation.
func slidesInputStatError(err error, param, msg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: unsafe file path: %s", msg, err).WithParam(param).WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: %s", msg, err).WithParam(param).WithCause(err)
}

// appendSlidesProgressHint preserves err's typed classification (per
// ERROR_CONTRACT.md "propagate typed errors unchanged") and appends an
// orchestration-progress hint — e.g. "presentation was created; N image(s)
// uploaded before failure" — so a failure mid-sequence still tells the caller
// what partial state exists. An unclassified error (e.g. surfaced from a shared
// helper boundary before it can be classified) falls back to a typed internal
// error carrying the hint.
func appendSlidesProgressHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		if p.Hint != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "%s", err.Error()).WithHint(hint).WithCause(err)
}
