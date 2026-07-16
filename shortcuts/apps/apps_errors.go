// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func appsValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func appsValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return appsValidationError(format, args...).WithParam(param)
}

func appsInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: name, Reason: reason}
}

func appsFailedPreconditionParamError(param, format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, format, args...).WithParam(param)
}

func appsFailedPreconditionError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, format, args...)
}

// appsStorageError classifies a local credential/state storage failure
// (read, write, delete, list) as internal/storage.
func appsStorageError(err error, format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeStorage, format, args...).WithCause(err)
}

// appsExternalToolError classifies a runtime failure of an external tool the
// CLI shells out to (git, npx) as internal/external_tool. The tool output is
// carried in the message; recovery guidance goes in the hint.
func appsExternalToolError(err error, format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeExternalTool, format, args...).WithCause(err)
}

// appsSubprocessEnvelopeError classifies a malformed or unexpected response
// structure as internal/invalid_response. Used for subprocess envelopes
// (+git-credential-init / +env-pull) and server responses (e.g. pre_release).
func appsSubprocessEnvelopeError(format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}

func appsInputPathError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return appsValidationParamError("--path", "unsafe --path: %s", err).WithCause(err)
	}
	return appsValidationParamError("--path", "cannot read --path: %s", err).WithCause(err)
}

func appsInputPathEntryError(path string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return appsValidationParamError("--path", "unsafe --path entry %s: %s", path, err).WithCause(err)
	}
	return appsValidationParamError("--path", "cannot read --path entry %s: %s", path, err).WithCause(err)
}

func appsFileIOError(err error, format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeFileIO, format, args...).WithCause(err)
}
