// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/larksuite/cli/errs"
)

// ValidationErrorf creates a typed invalid-argument error.
func ValidationErrorf(format string, args ...any) *errs.ValidationError {
	return errs.NewValidationError(errs.SubtypeInvalidArgument, format, args...)
}

// InvalidResponseErrorf creates a typed malformed-response error.
func InvalidResponseErrorf(format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}

// InternalErrorf creates a typed internal error for an invariant failure.
func InternalErrorf(format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeUnknown, format, args...)
}

// PaginationLimitError reports an incomplete all-pages read with a resume token.
func PaginationLimitError(pages int, nextToken string) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeQuotaExceeded,
		"pagination reached the hard limit after %d page(s)", pages).
		WithHint("resume from page_token %q after narrowing the request or increasing the host bound", nextToken)
}

// PaginationInterruptedError converts context cancellation into a typed network error.
func PaginationInterruptedError(cause error) *errs.NetworkError {
	subtype := errs.SubtypeNetworkTransport
	if errors.Is(cause, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
	}
	return errs.NewNetworkError(subtype, "pagination interrupted: %v", cause).WithCause(cause)
}

// Failure is a stable snapshot suitable for embedding in partial result data.
type Failure struct {
	Type      string `json:"type" schema:"required" doc:"error category"`
	Subtype   string `json:"subtype,omitempty" schema:"optional" doc:"error subtype"`
	Code      int    `json:"code,omitempty" schema:"optional" doc:"remote error code"`
	Message   string `json:"message" schema:"required" doc:"safe error message"`
	Hint      string `json:"hint,omitempty" schema:"optional" doc:"recovery hint"`
	LogID     string `json:"log_id,omitempty" schema:"optional" doc:"remote request log identifier"`
	Retryable bool   `json:"retryable,omitempty" schema:"optional" doc:"whether retry may succeed"`
}

// SnapshotFailure copies safe typed error fields into result data.
func SnapshotFailure(err error) Failure {
	if problem, ok := errs.ProblemOf(err); ok {
		return Failure{
			Type:      string(problem.Category),
			Subtype:   string(problem.Subtype),
			Code:      problem.Code,
			Message:   problem.Message,
			Hint:      problem.Hint,
			LogID:     problem.LogID,
			Retryable: problem.Retryable,
		}
	}
	return Failure{Type: string(errs.CategoryInternal), Subtype: string(errs.SubtypeUnknown), Message: fmt.Sprint(err)}
}
