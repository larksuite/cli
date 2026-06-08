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
