// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

func TestRetryAfterSeconds(t *testing.T) {
	for _, test := range []struct {
		name   string
		header http.Header
		want   int
	}{
		{
			name: "gateway header takes precedence",
			header: http.Header{
				"X-Ogw-Ratelimit-Reset": []string{"8"},
				"Retry-After":           []string{"4"},
			},
			want: 8,
		},
		{name: "standard header", header: http.Header{"Retry-After": []string{"3600"}}, want: 3600},
		{name: "invalid gateway falls back", header: http.Header{
			"X-Ogw-Ratelimit-Reset": []string{"invalid"},
			"Retry-After":           []string{"2"},
		}, want: 2},
		{name: "zero omitted", header: http.Header{"Retry-After": []string{"0"}}},
		{name: "missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := errclass.RetryAfterSeconds(test.header); got != test.want {
				t.Fatalf("RetryAfterSeconds() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestBuildAPIErrorAttachesRetryAfterOnlyToRateLimits(t *testing.T) {
	for _, test := range []struct {
		name       string
		code       int
		retryAfter int
		want       int
	}{
		{name: "rate limit preserves server value", code: 99991400, retryAfter: 3600, want: 3600},
		{name: "rate limit without header omits value", code: 99991400},
		{name: "other retryable error ignores header", code: 1061045, retryAfter: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := errclass.BuildAPIError(
				map[string]any{"code": test.code, "msg": "upstream failure"},
				errclass.ClassifyContext{RetryAfterSeconds: test.retryAfter},
			)
			var apiErr *errs.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("BuildAPIError() = %T %v, want *errs.APIError", err, err)
			}
			if apiErr.RetryAfterSeconds != test.want {
				t.Fatalf("retry_after_seconds = %d, want %d", apiErr.RetryAfterSeconds, test.want)
			}
			if test.want > 0 && (!strings.Contains(apiErr.Hint, "server requested") || !strings.Contains(apiErr.Hint, "safe to repeat")) {
				t.Fatalf("hint = %q, want server delay and repeat-safety guidance", apiErr.Hint)
			}
		})
	}
}
