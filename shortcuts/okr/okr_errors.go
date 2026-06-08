// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// okrInputStatError maps a FileIO.Stat/Open error for input file validation to
// a typed validation error: path validation failures read as "unsafe file
// path", other errors as "cannot read file".
func okrInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).
			WithParam("--file").
			WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read file: %s", err).
		WithParam("--file").
		WithCause(err)
}

// wrapOkrNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving subtype / code / log_id from the runtime boundary) and only
// wraps a raw, unclassified error as a transport-level network error.
func wrapOkrNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}
