// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
)

func TestOAPIGetBuildsFreshRequestsFromInjectedStream(t *testing.T) {
	var requests []*larkcore.ApiReq
	doStream := APIStreamFunc(func(ctx context.Context, req *larkcore.ApiReq, options ...client.Option) (*http.Response, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("OAPI transport unexpectedly imposed a request deadline")
		}
		if req == nil || req.HttpMethod != http.MethodGet || req.ApiPath != "/download/:file" {
			t.Fatalf("request = %#v", req)
		}
		if req.PathParams.Get("file") != "file-1" || req.QueryParams.Get("type") != "file" {
			t.Fatalf("request params = path %#v, query %#v", req.PathParams, req.QueryParams)
		}
		if len(options) != 2 {
			t.Fatalf("options = %d, want headers and replay safety", len(options))
		}
		requests = append(requests, req)
		// Mutating one SDK request must not affect a later range request.
		req.PathParams.Set("file", "mutated")
		req.QueryParams.Set("version", "mutated")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("payload")),
		}, nil
	})

	transport := NewOAPI(doStream).Get(
		"/download/:file",
		PathParam("file", "file-1"),
		Query("type", "file"),
	)
	for i := 0; i < 2; i++ {
		resp, err := transport(context.Background(), Request{Range: &ByteRange{Start: int64(i * 4), End: int64(i*4 + 3)}})
		if err != nil {
			t.Fatalf("transport() call %d error = %v", i+1, err)
		}
		resp.Body.Close()
	}
	if len(requests) != 2 || requests[0] == requests[1] || requests[0].PathParams == nil || requests[1].PathParams == nil {
		t.Fatalf("requests = %#v, want two independent requests", requests)
	}
}
