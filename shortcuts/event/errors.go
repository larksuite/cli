// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "github.com/larksuite/cli/errs"

func eventValidationError(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

func eventValidationParamError(param, format string, args ...any) *errs.ValidationError {
	return eventValidationError(format, args...).WithParam(param)
}

// eventValidationParamErrorWithCause appends ": <err>" to the formatted
// message and preserves err as the unwrap cause.
func eventValidationParamErrorWithCause(err error, param, format string, args ...any) *errs.ValidationError {
	return eventValidationParamError(param, format+": %s", append(args, err)...).WithCause(err)
}

// eventFileIOError appends ": <err>" to the formatted message and preserves
// err as the unwrap cause.
func eventFileIOError(err error, format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeFileIO, format+": %s", append(args, err)...).WithCause(err)
}

// eventNetworkError appends ": <err>" to the formatted message and preserves
// err as the unwrap cause.
func eventNetworkError(err error, format string, args ...any) *errs.NetworkError {
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, format+": %s", append(args, err)...).WithCause(err)
}
