// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
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

// appsSubprocessEnvelopeError classifies a malformed or failed envelope from a
// lark-cli subprocess (+git-credential-init / +env-pull) as internal/invalid_response.
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

// enrichHTMLPublishAPIError adapts a typed failure from the HTML publish
// endpoint: refines endpoint-scoped business codes, prefixes the message with
// command context, and attaches endpoint-specific recovery hints. A
// still-untyped error is lifted at the SDK boundary instead.
func enrichHTMLPublishAPIError(err error) error {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return client.WrapDoAPIError(err)
	}
	// The HTML publish business codes (90001/90002) are scoped to this
	// endpoint, not service-global, so their subtype classification lives
	// here instead of the global errclass code table. Only an
	// otherwise-unclassified API error is refined; a stronger upstream
	// classification is never overridden.
	if p.Category == errs.CategoryAPI && p.Subtype == errs.SubtypeUnknown && p.Code == errCodeAppNotFound {
		p.Subtype = errs.SubtypeNotFound
	}
	if p.Message != "" {
		p.Message = "html-publish failed: " + p.Message
	}
	if hint := buildHTMLPublishFailureHint(p.Code); hint != "" {
		p.Hint = hint
	}
	return err
}
