// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// wrapDriveNetworkErr returns err unchanged when it is already a typed errs.*
// error (preserving its subtype / code / log_id from the runtime boundary),
// and only wraps a raw, unclassified error as a transport-level network error.
func wrapDriveNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// driveInputStatError maps a FileIO.Stat/Open error for input file validation
// to a typed validation error:
//   - Path validation failures → "unsafe file path: ..."
//   - Other errors → "cannot read file: ..."
func driveInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read file: %s", err).WithCause(err)
}

// driveSaveError maps a FileIO.Save error to a typed error. Path validation
// failures are validation errors (exit code 2); mkdir / write failures are
// internal file-I/O errors (exit code 5).
func driveSaveError(err error) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).WithCause(err)
	}
}

// appendDriveExportRecoveryHint attaches a recovery hint to err while preserving
// its original classification (typed subtype/code), only falling back to a typed
// internal error when err is unclassified.
func appendDriveExportRecoveryHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	// An already-typed error keeps its own category/subtype/code/log_id
	// (per ERROR_CONTRACT.md "propagate typed errors unchanged"); we only
	// append the recovery hint. p points at the embedded Problem, so the
	// mutation is reflected in the returned err.
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
