// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
)

// RetryAfterSeconds returns the positive server-provided retry delay. The
// Open Gateway header takes precedence over the standard Retry-After header,
// matching the token endpoint behavior that established this contract.
func RetryAfterSeconds(header http.Header) int {
	for _, name := range []string{"X-Ogw-Ratelimit-Reset", "Retry-After"} {
		seconds, err := strconv.Atoi(strings.TrimSpace(header.Get(name)))
		if err == nil && seconds > 0 {
			return seconds
		}
	}
	return 0
}

// NewRateLimitError constructs a status-level rate-limit error using the same
// retry metadata enrichment as BuildAPIError. It is used when an HTTP 429 body
// has no usable Lark business code.
func NewRateLimitError(code int, message, logID string, cc ClassifyContext) *errs.APIError {
	return buildAPIError(errs.Problem{
		Category:  errs.CategoryAPI,
		Subtype:   errs.SubtypeRateLimit,
		Code:      code,
		Message:   message,
		LogID:     logID,
		Retryable: true,
	}, cc)
}

func buildAPIError(problem errs.Problem, cc ClassifyContext) *errs.APIError {
	err := &errs.APIError{Problem: problem}
	if problem.Subtype != errs.SubtypeRateLimit || cc.RetryAfterSeconds <= 0 {
		return err
	}

	err.RetryAfterSeconds = cc.RetryAfterSeconds
	retryHint := fmt.Sprintf(
		"the server requested waiting at least %d seconds before retrying; retryable does not mean this command is safe to repeat",
		cc.RetryAfterSeconds,
	)
	if err.Hint == "" {
		err.Hint = retryHint
	} else {
		err.Hint += "; " + retryHint
	}
	return err
}
