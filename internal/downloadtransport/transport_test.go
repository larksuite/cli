// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package downloadtransport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	extdownload "github.com/larksuite/cli/extension/download"
	"github.com/larksuite/cli/internal/client"
)

func TestURLAppliesDownloadHeaders(t *testing.T) {
	var captured *http.Request
	httpClient := &http.Client{Timeout: time.Nanosecond, Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured = req
		if _, hasDeadline := req.Context().Deadline(); hasDeadline {
			t.Fatal("URL transport retained the client's absolute timeout")
		}
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("payload")),
		}, nil
	})}

	resp, err := URL(httpClient, "https://example.com/object")(context.Background(), extdownload.Request{
		Range:   &extdownload.ByteRange{Start: 4, End: 9},
		IfRange: `"v1"`,
	})
	if err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	defer resp.Body.Close()
	if captured == nil || captured.Method != http.MethodGet || captured.URL.String() != "https://example.com/object" {
		t.Fatalf("request = %#v", captured)
	}
	if captured.Header.Get("Range") != "bytes=4-9" || captured.Header.Get("If-Range") != `"v1"` || captured.Header.Get("Accept-Encoding") != "identity" {
		t.Fatalf("headers = %#v", captured.Header)
	}
}

func TestURLOpensImmutableMultipartSource(t *testing.T) {
	payload := []byte("abcdefgh")
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		value := req.Header.Get("Range")
		var start, end int64
		if _, err := fmt.Sscanf(value, "bytes=%d-%d", &start, &end); err != nil {
			t.Fatalf("Range = %q: %v", value, err)
		}
		end = min(end, int64(len(payload))-1)
		return transportTestPartial(payload[start:end+1], start, end, int64(len(payload))), nil
	})}
	stream, err := extdownload.Open(context.Background(), extdownload.ImmutableSource(URL(httpClient, "https://example.com/object")), extdownload.Options{PartSize: 4})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || string(got) != string(payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
}

func TestURLClassifiesHTTPStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		header     http.Header
		subtype    errs.Subtype
		retryable  bool
		retryAfter time.Duration
	}{
		{name: "request timeout", status: http.StatusRequestTimeout, subtype: errs.SubtypeNetworkTimeout, retryable: true},
		{name: "rate limit", status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"4"}}, subtype: errs.SubtypeNetworkTransport, retryable: true, retryAfter: 4 * time.Second},
		{name: "server", status: http.StatusServiceUnavailable, header: http.Header{"X-Ogw-Ratelimit-Reset": {"8"}, "Retry-After": {"3"}}, subtype: errs.SubtypeNetworkServer, retryable: true, retryAfter: 3 * time.Second},
		{name: "bad request", status: http.StatusBadRequest, subtype: errs.SubtypeNetworkTransport},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Header:     tt.header,
					Body:       io.NopCloser(strings.NewReader("upstream response")),
				}, nil
			})}
			_, err := URL(httpClient, "https://example.com/object")(context.Background(), extdownload.Request{})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != tt.subtype || problem.Code != tt.status || problem.Retryable != tt.retryable {
				t.Fatalf("problem = %#v, %v", problem, ok)
			}
			delay, hasDelay := errs.RetryAfter(err)
			if delay != tt.retryAfter || hasDelay != (tt.retryAfter > 0) {
				t.Fatalf("RetryAfter() = %s, %v; want %s", delay, hasDelay, tt.retryAfter)
			}
		})
	}
}

func TestURLTransportFailureOwnsRetryability(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		subtype   errs.Subtype
		retryable bool
	}{
		{name: "dns", err: &net.DNSError{Err: "temporary failure", Name: "example.com", IsTemporary: true}, subtype: errs.SubtypeNetworkDNS, retryable: true},
		{name: "transport", err: errors.New("connection reset by peer"), subtype: errs.SubtypeNetworkTransport, retryable: true},
		{name: "tls", err: errors.New("tls: certificate verification failed"), subtype: errs.SubtypeNetworkTLS},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, tt.err
			})}
			_, err := URL(httpClient, "https://example.com/object")(context.Background(), extdownload.Request{})
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != tt.subtype || problem.Retryable != tt.retryable {
				t.Fatalf("problem = %#v, %v", problem, ok)
			}
		})
	}
}

func TestURLCallerDeadlineIsNotRetryable(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := URL(httpClient, "https://example.com/object")(ctx, extdownload.Request{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeNetworkTimeout || problem.Retryable || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("problem = %#v, %v; error = %v", problem, ok, err)
	}
}

func TestURLRedirectPolicyFailureIsNotRetryable(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": {"https://blocked.example/object"}},
				Body:       io.NopCloser(strings.NewReader("redirect")),
				Request:    req,
			}, nil
		}),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("blocked redirect target")
		},
	}
	_, err := URL(httpClient, "https://example.com/object")(context.Background(), extdownload.Request{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeNetworkTransport || problem.Retryable {
		t.Fatalf("problem = %#v, %v; error = %v", problem, ok, err)
	}
}

func TestURLFailuresDoNotExposeSignedURLOrResponseBody(t *testing.T) {
	const signedURL = "https://example.com/object?signature=top-secret"
	transportFailure := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection failed")
	})}
	_, err := URL(transportFailure, signedURL)(context.Background(), extdownload.Request{})
	if err == nil || strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), signedURL) {
		t.Fatalf("transport error leaked signed URL: %v", err)
	}

	httpFailure := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("rejected signature=top-secret")),
		}, nil
	})}
	_, err = URL(httpFailure, signedURL)(context.Background(), extdownload.Request{})
	if err == nil || strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), "rejected signature") {
		t.Fatalf("HTTP error leaked signed response detail: %v", err)
	}
}

func TestOAPIGetBuildsFreshRequestsFromInjectedStream(t *testing.T) {
	var requests []*larkcore.ApiReq
	doStream := APIStreamFunc(func(ctx context.Context, req *larkcore.ApiReq, options ...client.Option) (*http.Response, error) {
		if _, ok := ctx.Deadline(); ok {
			t.Fatal("OAPI transport unexpectedly imposed a request deadline")
		}
		if req == nil || req.HttpMethod != http.MethodGet || req.ApiPath != "/download/:file" {
			t.Fatalf("request = %#v", req)
		}
		if req.PathParams.Get("file") != "file-1" || req.QueryParams.Get("type") != "file" || req.QueryParams.Get("version") != "7" || req.QueryParams.Get("empty") != "" {
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
		QueryIf("version", "7"),
		QueryIf("empty", ""),
	)
	for i := 0; i < 2; i++ {
		resp, err := transport(context.Background(), extdownload.Request{Range: &extdownload.ByteRange{Start: int64(i * 4), End: int64(i*4 + 3)}})
		if err != nil {
			t.Fatalf("transport() call %d error = %v", i+1, err)
		}
		resp.Body.Close()
	}
	if len(requests) != 2 || requests[0] == requests[1] || requests[0].PathParams == nil || requests[1].PathParams == nil {
		t.Fatalf("requests = %#v, want two independent requests", requests)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func transportTestPartial(body []byte, start, end, total int64) *http.Response {
	return &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, total)},
		},
		Body:          io.NopCloser(strings.NewReader(string(body))),
		ContentLength: int64(len(body)),
	}
}
