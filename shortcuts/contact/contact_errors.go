// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
)

const contactFanoutRetryHint = "retry the command; if it persists, narrow --queries to a single term to isolate the failing input"

func contactInvalidResponseError(format string, args ...any) *errs.InternalError {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...)
}

func contactFanoutErrorSummary(err error) string {
	if p, ok := errs.ProblemOf(err); ok {
		if p.Code >= 100 && p.Code < 600 {
			prefix := fmt.Sprintf("HTTP %d:", p.Code)
			body := strings.TrimSpace(strings.TrimPrefix(p.Message, prefix))
			msg := fmt.Sprintf("HTTP %d %s", p.Code, http.StatusText(p.Code))
			if body != "" {
				msg = fmt.Sprintf("%s: %s", msg, contactTruncateError(body, 200))
			}
			return msg
		}
		if p.Code != 0 {
			return fmt.Sprintf("API %d: %s", p.Code, p.Message)
		}
		return p.Message
	}
	return err.Error()
}

// contactFanoutAllFailedError builds the top-level error returned when every
// fanout query fails. It mirrors the representative (first) failure's
// classification — category, subtype, code, log_id, retryable, hint — so the
// exit-code classifier still sees the real signal, while carrying the aggregate
// message. The representative error is copied (never mutated) and kept as the
// cause, so a single-query problem object is not rewritten into an aggregate one.
func contactFanoutAllFailedError(err error, msg string) error {
	var (
		apiErr *errs.APIError
		netErr *errs.NetworkError
		intErr *errs.InternalError
	)
	switch {
	case errors.As(err, &apiErr):
		c := *apiErr
		c.Message = msg
		c.Cause = err
		return &c
	case errors.As(err, &netErr):
		c := *netErr
		c.Message = msg
		c.Cause = err
		return &c
	case errors.As(err, &intErr):
		c := *intErr
		c.Message = msg
		c.Cause = err
		return &c
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "%s", msg).WithHint(contactFanoutRetryHint).WithCause(err)
}

func contactTruncateError(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	return string(r[:maxRunes]) + "..."
}
