// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

func mailValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func mailValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return mailValidationError(format, args...).WithParam(param)
}

func mailInvalidParam(name, reason string) errs.InvalidParam {
	return errs.InvalidParam{Name: name, Reason: reason}
}

func mailFailedPreconditionError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, format, args...)
}

func mailInvalidResponseError(format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}

func mailFileIOError(format string, err error, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeFileIO, format, args...).WithCause(err)
}

func mailInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return mailValidationError("unsafe file path: %s", err).WithCause(err)
	}
	return mailValidationError("cannot read file: %s", err).WithCause(err)
}

func mailDecorateProblemMessage(err error, format string, args ...any) error {
	if err == nil {
		return nil
	}
	prefix := fmt.Sprintf(format, args...)
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(prefix) != "" {
			p.Message = prefix + ": " + p.Message
		}
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "%s: %s", prefix, err.Error()).WithCause(err)
}

func mailAppendProblemHint(err error, hint string) error {
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		if strings.TrimSpace(p.Hint) != "" {
			p.Hint = p.Hint + "; " + hint
		} else {
			p.Hint = hint
		}
		return err
	}
	return errs.NewAPIError(errs.SubtypeUnknown, "%s", err.Error()).WithHint("%s", hint).WithCause(err)
}
