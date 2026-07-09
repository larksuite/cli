// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"errors"

	"github.com/larksuite/cli/errs"
)

func markdownValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func markdownValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return markdownValidationError(format, args...).WithParam(param)
}

func markdownInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: name, Reason: reason}
}

// withMarkdownFileParam tags a validation failure with the originating flag
// when it does not already name one. Shared input-file helpers such as
// common.WrapInputStatErrorTyped cannot know which flag supplied the path, so
// the caller attaches it here to keep the recoverable param on the wire.
func withMarkdownFileParam(err error, param string) error {
	if err == nil || param == "" {
		return err
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) && ve.Param == "" {
		ve.WithParam(param)
	}
	return err
}

// wrapMarkdownDownloadError classifies a download failure. An already-typed
// error keeps its carrier — type, subtype, code and extensions — so callers see
// the upstream classification: a validation problem passes through verbatim,
// any other problem gains a "download failed" prefix for operation context.
// An untyped error becomes a network transport error carrying the original as
// its cause.
func wrapMarkdownDownloadError(err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		if p.Category != errs.CategoryValidation {
			p.Message = "download failed: " + p.Message
		}
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, "download failed: %s", err).WithCause(err)
}
