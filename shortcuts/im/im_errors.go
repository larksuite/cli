// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"errors"
	"strings"

	"github.com/larksuite/cli/errs"
)

// wrapIMNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapIMNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

func imContextError(err error) error {
	if err == nil {
		return nil
	}
	subtype := errs.SubtypeNetworkTransport
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
	}
	return errs.NewNetworkError(subtype, "%s", err.Error()).WithCause(err)
}

func withIMValidationParam(err error, param string) error {
	if err == nil || param == "" {
		return err
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) && ve.Param == "" {
		ve.WithParam(param)
	}
	return err
}

// appendIMRecoveryHint attaches a recovery hint to err. A typed error keeps its
// classification (category/subtype/code/log_id); only the hint is appended to
// p.Hint (newline-joined when a hint already exists), and err is returned
// unchanged. An unclassified error falls back to a typed internal error.
func appendIMRecoveryHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "\n" + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s", err.Error()).WithHint(hint).WithCause(err)
}
