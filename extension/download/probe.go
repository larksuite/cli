// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"io"
	"net/http"
)

// RangeProbeResult contains the representation metadata learned from a
// one-byte range probe. TotalSize is -1 when a full-response fallback did not
// provide a Content-Length; callers must then treat the checkpoint as
// unverifiable.
type RangeProbeResult struct {
	TotalSize int64
	ETag      string

	_ struct{}
}

// ProbeRange validates a one-byte range response using the same retry and idle
// timeout policy as Open. A 200 response is returned as a valid probe result
// with its Content-Length, allowing callers to decide whether Range support is
// sufficient for their operation.
func ProbeRange(ctx context.Context, source Source, expectedETag string) (*RangeProbeResult, error) {
	opts := (Options{PartSize: 1, ExpectedETag: expectedETag}).withDefaults()
	if err := validateOptions(source, opts); err != nil {
		return nil, err
	}

	retryWait := newRetryWaitBudget(opts.RetryWaitBudget)
	requested := ByteRange{Start: 0, End: 0}
	resp, err := fetchWithRetry(ctx, source.transport, Request{Range: &requested, IfRange: expectedETag}, opts, retryWait)
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Body == nil {
		return nil, protocolError("range probe returned an empty response")
	}
	defer resp.Body.Close()

	if err := validateResponseEncoding(resp); err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		parsed, err := parseContentRange(resp.Header.Get("Content-Range"))
		if err != nil {
			return nil, protocolError("invalid Content-Range header on range probe: %s", err)
		}
		if parsed.start != 0 || parsed.end != 0 {
			return nil, protocolError("range probe returned %s, want bytes 0-0/%d", parsed, parsed.total)
		}
		if resp.ContentLength >= 0 && resp.ContentLength != 1 {
			return nil, protocolError("range probe declared %d body bytes, want 1", resp.ContentLength)
		}
		body, err := io.ReadAll(io.LimitReader(newClassifiedBody(ctx, resp.Body), 2))
		if err != nil {
			return nil, err
		}
		if len(body) != 1 {
			return nil, protocolError("range probe returned %d body bytes, want 1", len(body))
		}
		return &RangeProbeResult{
			TotalSize: parsed.total,
			ETag:      probeETag(resp),
		}, nil
	case http.StatusOK:
		return &RangeProbeResult{
			TotalSize: resp.ContentLength,
			ETag:      probeETag(resp),
		}, nil
	default:
		return nil, unexpectedStatus(resp.StatusCode)
	}
}

func probeETag(resp *http.Response) string {
	etag, ok := strongETag(resp.Header)
	if !ok {
		return ""
	}
	return etag
}
