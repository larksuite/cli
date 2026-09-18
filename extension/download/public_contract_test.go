// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/larksuite/cli/extension/download"
)

// This external-package test pins that extensions can implement Transport and
// opt into immutable multipart transfer using only the public package.
func TestPublicImmutableMultipartContract(t *testing.T) {
	payload := []byte("abcdefgh")
	var ranges []string
	transport := download.Transport(func(_ context.Context, request download.Request) (*http.Response, error) {
		if request.Range == nil {
			t.Fatal("immutable multipart unexpectedly used a full request")
		}
		ranges = append(ranges, request.Range.HeaderValue())
		start, end := request.Range.Start, min(request.Range.End, int64(len(payload)-1))
		body := payload[start : end+1]
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header: http.Header{
				"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
			},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
		}, nil
	})

	stream, err := download.Open(context.Background(), download.ImmutableSource(transport), download.Options{PartSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	content, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(content, payload) {
		t.Fatalf("download = %q, %v", content, err)
	}
	if len(ranges) != 2 || ranges[0] != "bytes=0-3" || ranges[1] != "bytes=4-7" {
		t.Fatalf("ranges = %#v", ranges)
	}
}
