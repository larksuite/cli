// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestOpenClassifiesFullResponseReadFailures(t *testing.T) {
	tests := []struct {
		name    string
		readErr error
		subtype errs.Subtype
		message string
	}{
		{name: "deadline", readErr: context.DeadlineExceeded, subtype: errs.SubtypeNetworkTimeout, message: "timed out"},
		{name: "socket timeout", readErr: timeoutReadError{}, subtype: errs.SubtypeNetworkTimeout, message: "timed out"},
		{name: "truncated", readErr: io.ErrUnexpectedEOF, subtype: errs.SubtypeNetworkTransport, message: "ended unexpectedly"},
		{name: "transport", readErr: errors.New("connection reset"), subtype: errs.SubtypeNetworkTransport, message: "connection reset"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Header:        make(http.Header),
					Body:          errorBody{err: tt.readErr},
					ContentLength: -1,
				}, nil
			}, testOptions())
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer stream.Body.Close()
			_, err = io.ReadAll(stream.Body)
			requireProblem(t, err, tt.subtype, true, tt.message)
		})
	}
}

func TestOpenClassifiesUnknownLengthNoProgress(t *testing.T) {
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          io.NopCloser(noProgressReader{}),
			ContentLength: -1,
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkTransport, true, "no progress")
	if !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("ReadAll() error = %v, want io.ErrNoProgress cause", err)
	}
}

func TestOpenPreservesTypedFullResponseReadError(t *testing.T) {
	want := errs.NewNetworkError(errs.SubtypeNetworkServer, "upstream failed").WithRetryable()
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          errorBody{err: want},
			ContentLength: -1,
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	if !errors.Is(err, want) {
		t.Fatalf("ReadAll() error = %v, want preserved typed error", err)
	}
}

func TestOpenClassifiesCallerCancellationAsTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := openTest(ctx, func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          errorBody{err: errors.New("connection closed")},
			ContentLength: -1,
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	cancel()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkTransport, false, "canceled")
}

func TestBodyRequestCancellationIsTerminal(t *testing.T) {
	err := classifyBodyReadError(context.Background(), context.Canceled, context.Canceled)
	requireProblem(t, err, errs.SubtypeNetworkTransport, false, "canceled")
}

func TestOpenClassifiesTruncatedFixedLengthResponse(t *testing.T) {
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          &scriptedBody{payload: []byte("ab"), readErr: io.ErrUnexpectedEOF},
			ContentLength: 4,
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if string(got) != "ab" {
		t.Fatalf("ReadAll() payload = %q, want ab", got)
	}
	requireProblem(t, err, errs.SubtypeNetworkTransport, true, "ended unexpectedly")
}

func TestOpenRejectsEncodedResponses(t *testing.T) {
	tests := []struct {
		name  string
		fetch Transport
		read  bool
	}{
		{
			name: "full response",
			fetch: func(context.Context, Request) (*http.Response, error) {
				return testResponse(http.StatusOK, []byte("compressed"), http.Header{"Content-Encoding": {"gzip"}}), nil
			},
		},
		{
			name: "stacked encoding",
			fetch: func(context.Context, Request) (*http.Response, error) {
				return testResponse(http.StatusOK, []byte("compressed"), http.Header{"Content-Encoding": {"identity", "gzip"}}), nil
			},
		},
		{
			name: "empty encoding",
			fetch: func(context.Context, Request) (*http.Response, error) {
				return testResponse(http.StatusOK, []byte("unknown"), http.Header{"Content-Encoding": {""}}), nil
			},
		},
		{
			name: "transparent decoding",
			fetch: func(context.Context, Request) (*http.Response, error) {
				resp := testResponse(http.StatusOK, []byte("decoded"), nil)
				resp.Uncompressed = true
				return resp, nil
			},
		},
		{
			name: "initial partial response",
			fetch: func(context.Context, Request) (*http.Response, error) {
				resp := testPartial([]byte("abcd"), 0, 3, 8, `"v1"`)
				resp.Header.Set("Content-Encoding", "gzip")
				return resp, nil
			},
		},
		{
			name: "followup partial response",
			fetch: func() Transport {
				calls := 0
				return func(context.Context, Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return testPartial([]byte("abcd"), 0, 3, 8, `"v1"`), nil
					}
					resp := testPartial([]byte("efgh"), 4, 7, 8, `"v1"`)
					resp.Header.Set("Content-Encoding", "br")
					return resp, nil
				}
			}(),
			read: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream, err := openTest(context.Background(), tt.fetch, testOptions())
			if tt.read && err == nil {
				defer stream.Body.Close()
				_, err = io.ReadAll(stream.Body)
			}
			requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "Content-Encoding")
		})
	}
}

func TestOpenRejectsEncodedFullResponseContinuation(t *testing.T) {
	payload := []byte("abcdefgh")
	calls := 0
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return testPartial(payload[:4], 0, 3, int64(len(payload)), ""), nil
		default:
			return testResponse(http.StatusOK, payload, http.Header{"Content-Encoding": {"gzip"}}), nil
		}
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "Content-Encoding")
}

func TestOpenClassifiesFullContinuationPrefixFailure(t *testing.T) {
	payload := []byte("abcdefgh")
	calls := 0
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return testPartial(payload[:4], 0, 3, int64(len(payload)), ""), nil
		default:
			return &http.Response{
				StatusCode:    http.StatusOK,
				Header:        make(http.Header),
				Body:          &scriptedBody{payload: payload[:2], readErr: io.ErrUnexpectedEOF},
				ContentLength: int64(len(payload)),
			}, nil
		}
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkTransport, true, "ended unexpectedly")
}

func TestOpenAcceptsExplicitIdentityEncoding(t *testing.T) {
	resp := testResponse(http.StatusOK, []byte("plain"), http.Header{"Content-Encoding": {" identity "}})
	stream, err := openTest(context.Background(), staticFetch(resp), testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	if got, err := io.ReadAll(stream.Body); err != nil || string(got) != "plain" {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
}

func TestOpenClosesRejectedEncodedBody(t *testing.T) {
	body := &closeTrackingBody{}
	_, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Encoding": {"gzip"}},
			Body:       body,
		}, nil
	}, testOptions())
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "Content-Encoding")
	if !body.closed {
		t.Fatal("rejected response body was not closed")
	}
}

type timeoutReadError struct{}

func (timeoutReadError) Error() string   { return "read timeout" }
func (timeoutReadError) Timeout() bool   { return true }
func (timeoutReadError) Temporary() bool { return true }

type closeTrackingBody struct{ closed bool }

func (*closeTrackingBody) Read([]byte) (int, error) { return 0, io.EOF }
func (b *closeTrackingBody) Close() error {
	b.closed = true
	return nil
}
