// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"net/http"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
)

func TestClassifyAPIResponseWithIncludesRetryAfterSeconds(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "business rate limit", body: `{"code":99991400,"msg":"rate limited"}`},
		{name: "bare HTTP rate limit", body: `{"code":0,"msg":"rate limited"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			resp := &larkcore.ApiResp{
				StatusCode: http.StatusTooManyRequests,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Retry-After":  []string{"2"},
				},
				RawBody: []byte(test.body),
			}

			_, err := ClassifyAPIResponseWith(resp, errclass.ClassifyContext{Identity: "user"})
			var apiErr *errs.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("ClassifyAPIResponseWith() error = %T %v, want *errs.APIError", err, err)
			}
			if apiErr.Subtype != errs.SubtypeRateLimit || apiErr.RetryAfterSeconds != 2 {
				t.Fatalf("rate-limit error = %+v, want retry_after_seconds=2", apiErr)
			}
		})
	}
}
