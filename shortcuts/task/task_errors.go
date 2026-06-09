// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// wrapTaskNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapTaskNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// taskInputStatError maps a FileIO.Stat/Open error for input file validation
// to a typed validation error:
//   - Path validation failures → "unsafe file path: ..."
//   - Other errors → readMsg prefix (default "cannot read file")
//
// param names the input flag/path field that failed (for example "--file").
// Pass an optional readMsg to override the non-path-validation message prefix,
// mirroring the shared input-stat helper so call-site context is preserved.
func taskInputStatError(err error, param string, readMsg ...string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		validationErr := errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).WithCause(err)
		if param != "" {
			validationErr = validationErr.WithParam(param)
		}
		return validationErr
	}
	msg := "cannot read file"
	if len(readMsg) > 0 && readMsg[0] != "" {
		msg = readMsg[0]
	}
	validationErr := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: %s", msg, err).WithCause(err)
	if param != "" {
		validationErr = validationErr.WithParam(param)
	}
	return validationErr
}
