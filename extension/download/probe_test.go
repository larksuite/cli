// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestProbeRangeRetriesAndValidatesOneByteResponse(t *testing.T) {
	attempts := 0
	result, err := ProbeRange(context.Background(), MutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkServer, "temporary probe failure").WithRetryable()
		}
		if req.Range == nil || req.Range.Start != 0 || req.Range.End != 0 {
			t.Fatalf("request = %#v, want bytes 0-0", req)
		}
		return testPartial([]byte("a"), 0, 0, 4, `"v1"`), nil
	}), `"v1"`)
	if err != nil {
		t.Fatalf("ProbeRange() error = %v", err)
	}
	if attempts != 2 || result.TotalSize != 4 || result.ETag != `"v1"` {
		t.Fatalf("result = %#v, attempts = %d", result, attempts)
	}
}

func TestProbeRangeRejectsMalformedRangeOrBody(t *testing.T) {
	tests := []struct {
		name string
		resp *http.Response
	}{
		{
			name: "wrong range",
			resp: testResponse(http.StatusPartialContent, []byte("a"), http.Header{
				"Content-Range": {"bytes 1-1/4"},
				"ETag":          {`"v1"`},
			}),
		},
		{
			name: "wrong body length",
			resp: testResponse(http.StatusPartialContent, []byte("ab"), http.Header{
				"Content-Range": {"bytes 0-0/4"},
				"ETag":          {`"v1"`},
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProbeRange(context.Background(), MutableSource(staticFetch(tt.resp)), `"v1"`)
			if err == nil {
				t.Fatal("ProbeRange() unexpectedly succeeded")
			}
		})
	}
}
