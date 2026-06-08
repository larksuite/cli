// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// wrapWbNetworkErr returns err unchanged when it is already a typed errs.* error
// (preserving subtype / code / log_id from the runtime boundary) and only wraps
// a raw, unclassified error as a transport-level network error.
func wrapWbNetworkErr(err error, format string, args ...any) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format, args...).WithCause(err)
}

// wbSaveError maps a FileIO.Save error to a typed error. Path validation
// failures are validation errors (exit code 2); mkdir / write failures are
// internal file-I/O errors (exit code 5).
func wbSaveError(err error) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).
			WithParam("--output").
			WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).WithCause(err)
	}
}

func wbInvalidResponse(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}
