// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/larksuite/cli/internal/output"
)

const errorSource = "local_ratelimit"

func newRateLimitError(rule *Rule, retryAfter time.Duration) error {
	retryAfterMs := RetryAfterMs(retryAfter)
	windowSeconds := int(math.Ceil(rule.Window.Seconds()))
	msg := fmt.Sprintf("local rate limit: %s %s exceeded %d requests per %ds",
		rule.Method, rule.CanonicalPath, rule.Limit, windowSeconds)
	return output.ErrAPI(output.LarkErrRateLimit, msg, map[string]any{
		"source":         errorSource,
		"retry_after_ms": retryAfterMs,
		"method":         rule.Method,
		"api_path":       rule.CanonicalPath,
	})
}

func IsLocalRateLimit(err error) bool {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return false
	}
	if exitErr.Detail.Type != "rate_limit" || exitErr.Detail.Code != output.LarkErrRateLimit {
		return false
	}
	detail, ok := exitErr.Detail.Detail.(map[string]any)
	return ok && detail["source"] == errorSource
}

func RetryAfterMs(d time.Duration) int64 {
	if d <= 0 {
		return 1
	}
	ms := int64(math.Ceil(float64(d) / float64(time.Millisecond)))
	if ms < 1 {
		return 1
	}
	return ms
}
