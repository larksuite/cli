// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func TestRequestHeadersDescribeWireRepresentation(t *testing.T) {
	rangeRequest := Request{Range: &ByteRange{Start: 4, End: 9}, IfRange: `"v1"`}
	headers := rangeRequest.Headers()
	if got := headers.Get("Range"); got != "bytes=4-9" {
		t.Fatalf("Range = %q, want bytes=4-9", got)
	}
	if got := headers.Get("If-Range"); got != `"v1"` {
		t.Fatalf("If-Range = %q, want v1", got)
	}
	if got := headers.Get("Accept-Encoding"); got != "identity" {
		t.Fatalf("Accept-Encoding = %q, want identity", got)
	}

	fullHeaders := (Request{}).Headers()
	if fullHeaders.Get("Range") != "" || fullHeaders.Get("If-Range") != "" || fullHeaders.Get("Accept-Encoding") != "identity" {
		t.Fatalf("full response headers = %#v", fullHeaders)
	}
}

func TestOpenSmallObjectCompletesInRangeProbe(t *testing.T) {
	payload := []byte("tiny")
	requests := 0
	source := ImmutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		if req.Range == nil {
			t.Fatal("small object unexpectedly fell back to a full request")
		}
		return testPartial(payload, 0, int64(len(payload)-1), int64(len(payload)), `"v1"`), nil
	})
	stream, err := Open(context.Background(), source, Options{PartSize: 128 * 1024})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want one probe request", requests)
	}
}

func TestOpenFallsBackWhenRangeProbeIsRejected(t *testing.T) {
	payload := []byte("full response")
	var requests []Request
	source := ImmutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		if req.Range != nil {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "range unsupported").WithCode(http.StatusRequestedRangeNotSatisfiable)
		}
		return testResponse(http.StatusOK, payload, nil), nil
	})
	stream, err := Open(context.Background(), source, Options{PartSize: 4})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	if len(requests) != 2 || requests[0].Range == nil || requests[1].Range != nil {
		t.Fatalf("requests = %#v, want range probe then full request", requests)
	}
}

func TestOpenRetriesOnlyWhenTransportMarksFailureRetryable(t *testing.T) {
	payload := []byte("tiny")
	attempts := 0
	opts := Options{PartSize: 4, MaxPartRetries: 1}
	stream, err := Open(context.Background(), immutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTimeout, "part deadline").
				WithRetryable().
				WithCause(context.DeadlineExceeded)
		}
		return testPartial(payload, req.Range.Start, req.Range.End, int64(len(payload)), ""), nil
	}), opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want one retry", attempts)
	}
}

func TestOpenDoesNotInferRetryability(t *testing.T) {
	want := errs.NewNetworkError(errs.SubtypeNetworkServer, "permanent upstream failure").WithCode(http.StatusServiceUnavailable)
	attempts := 0
	_, err := Open(context.Background(), immutableSource(func(context.Context, Request) (*http.Response, error) {
		attempts++
		return nil, want
	}), Options{PartSize: 4, MaxPartRetries: 3})
	if err != want {
		t.Fatalf("Open() error = %T %v, want original transport error", err, err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want no inferred retry", attempts)
	}
}

func TestOpenPreservesRetryabilityAfterAttemptsAreExhausted(t *testing.T) {
	want := errs.NewNetworkError(errs.SubtypeNetworkServer, "temporary upstream failure").
		WithCode(http.StatusServiceUnavailable).
		WithRetryable()
	attempts := 0
	_, err := Open(context.Background(), immutableSource(func(context.Context, Request) (*http.Response, error) {
		attempts++
		return nil, want
	}), Options{PartSize: 4, MaxPartRetries: 1})
	if err != want || !errs.IsRetryable(err) {
		t.Fatalf("Open() error = %T %v, want original retryable transport error", err, err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want initial request and one retry", attempts)
	}
}

func TestOpenHonorsServerRetryResetWithinCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	attempts := 0
	_, err := Open(ctx, immutableSource(func(context.Context, Request) (*http.Response, error) {
		attempts++
		return nil, errs.NewAPIError(errs.SubtypeRateLimit, "slow down").
			WithCode(http.StatusTooManyRequests).
			WithRetryable().
			WithRetryAfterSeconds(1)
	}), Options{PartSize: 4, MaxPartRetries: 1})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Open() error = %v, want caller deadline while waiting for reset", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want no retry before server reset", attempts)
	}
}

func TestOpenReturnsServerErrorWhenRetryDelayExceedsBudget(t *testing.T) {
	want := errs.NewAPIError(errs.SubtypeRateLimit, "slow down").
		WithCode(http.StatusTooManyRequests).
		WithRetryable().
		WithRetryAfterSeconds(1)
	attempts := 0
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := Open(ctx, immutableSource(func(context.Context, Request) (*http.Response, error) {
		attempts++
		return nil, want
	}), Options{
		PartSize:        4,
		MaxPartRetries:  1,
		RetryWaitBudget: 10 * time.Millisecond,
	})
	if err != want {
		t.Fatalf("Open() error = %T %v, want original rate-limit error", err, err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want no retry before server reset", attempts)
	}
}

func TestRetryWaitBudgetIsSharedWithBodyRetries(t *testing.T) {
	payload := []byte("abcdefgh")
	attempts := 0
	stream, err := Open(context.Background(), immutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		attempts++
		switch attempts {
		case 1:
			return nil, errs.NewNetworkError(errs.SubtypeNetworkServer, "temporary failure").WithRetryable()
		case 2:
			return &http.Response{
				StatusCode: http.StatusPartialContent,
				Header:     http.Header{"Content-Range": {"bytes 0-3/8"}, "Etag": {`"v1"`}},
				Body:       &scriptedBody{payload: []byte("ab"), readErr: io.ErrUnexpectedEOF},
			}, nil
		default:
			start := req.Range.Start
			end := min(req.Range.End, int64(len(payload))-1)
			return testPartial(payload[start:end+1], start, end, int64(len(payload)), `"v1"`), nil
		}
	}), Options{
		PartSize:        4,
		MaxPartRetries:  1,
		RetryDelay:      10 * time.Millisecond,
		RetryWaitBudget: 15 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkTransport, true, "range response body ended")
	if attempts != 2 {
		t.Fatalf("attempts = %d, want request retry only; body retry must exceed the shared budget", attempts)
	}
}

func TestRetryDelayUsesServerMinimumAndLocalBackoff(t *testing.T) {
	rateLimited := errs.NewAPIError(errs.SubtypeRateLimit, "slow down").WithRetryAfterSeconds(8)
	tests := []struct {
		name    string
		base    time.Duration
		attempt int
		err     error
		want    time.Duration
	}{
		{name: "server minimum wins", base: 300 * time.Millisecond, attempt: 2, err: rateLimited, want: 8 * time.Second},
		{name: "local backoff wins", base: 3 * time.Second, attempt: 2, err: rateLimited, want: 12 * time.Second},
		{name: "local only", base: 300 * time.Millisecond, attempt: 1, want: 600 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryDelay(tt.base, tt.attempt, tt.err); got != tt.want {
				t.Fatalf("retryDelay() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestOptionsSelectRetryDelayDefault(t *testing.T) {
	if got := (Options{}).withDefaults().RetryDelay; got != DefaultRetryDelay {
		t.Fatalf("RetryDelay = %s, want %s", got, DefaultRetryDelay)
	}
}

func TestRetryJitterNeverShortensDelay(t *testing.T) {
	const delay = 8 * time.Second
	for range 100 {
		got := addRetryJitter(delay)
		if got < delay || got > delay+250*time.Millisecond {
			t.Fatalf("addRetryJitter() = %s, want [%s, %s]", got, delay, delay+250*time.Millisecond)
		}
	}
}

func TestOpenSingleStreamWhenRangeIsIgnored(t *testing.T) {
	payload := []byte("all")
	var requests []Request
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		return testResponse(http.StatusOK, payload, nil), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) || stream.ContentLength != int64(len(payload)) {
		t.Fatalf("stream length:%d payload:%q", stream.ContentLength, got)
	}
	if len(requests) != 1 || requests[0].Range == nil || requests[0].Range.HeaderValue() != "bytes=0-3" {
		t.Fatalf("requests = %#v, want one range probe", requests)
	}
}

func TestOpenUsesLargeIgnoredRangeAsFullStream(t *testing.T) {
	payload := []byte("abcdefgh")
	var requests []Request
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		return testResponse(http.StatusOK, payload, nil), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	if len(requests) != 1 || requests[0].Range == nil {
		t.Fatalf("requests = %#v, want one range probe response reused as the full stream", requests)
	}
}

func TestOpenResumesFromResponseRanges(t *testing.T) {
	payload := []byte("abcdefghijkl")
	var starts []int64
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		if req.Range == nil {
			t.Fatal("unexpected full-response fallback")
		}
		start := req.Range.Start
		starts = append(starts, start)
		// The peer deliberately returns only two bytes even though the engine asks
		// for four. The next request must use this response end, not request.End.
		end := min(start+1, int64(len(payload))-1)
		header := http.Header{
			"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
			"Etag":          {`"v1"`},
		}
		if start > 0 && req.IfRange != `"v1"` {
			t.Fatalf("IfRange = %q, want strong probe ETag", req.IfRange)
		}
		return testResponse(http.StatusPartialContent, payload[start:end+1], header), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	wantStarts := []int64{0, 2, 4, 6, 8, 10}
	if fmt.Sprint(starts) != fmt.Sprint(wantStarts) {
		t.Fatalf("request starts = %v, want %v", starts, wantStarts)
	}
}

func TestOpenRejectsResponseAtWrongOffset(t *testing.T) {
	payload := []byte("abcdefgh")
	stream, err := openTest(context.Background(), func(_ context.Context, _ Request) (*http.Response, error) {
		return testPartial(payload[:4], 0, 3, int64(len(payload)), `"v1"`), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "want it to start at byte 4")
}

func TestOpenImmutableWithoutValidatorUsesExactRanges(t *testing.T) {
	full := []byte("abcdefgh")
	var requests []Request
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		if req.Range == nil {
			t.Fatal("unexpected full-response fallback")
		}
		start := req.Range.Start
		end := min(req.Range.End, int64(len(full))-1)
		return testPartial(full[start:end+1], start, end, int64(len(full)), ""), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, full) {
		t.Fatalf("stream payload=%q, want %q", got, full)
	}
	if len(requests) != 2 || requests[0].Range == nil || requests[1].Range == nil {
		t.Fatalf("requests = %#v, want two range requests", requests)
	}
	if got := []ByteRange{*requests[0].Range, *requests[1].Range}; fmt.Sprint(got) != fmt.Sprint([]ByteRange{{Start: 0, End: 3}, {Start: 4, End: 7}}) {
		t.Fatalf("ranges = %v, want exact non-overlapping ranges", got)
	}
}

func TestOpenMutableWithoutValidatorFallsBackBeforeDelivery(t *testing.T) {
	payload := []byte("abcdefgh")
	var requests []Request
	stream, err := Open(context.Background(), MutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		if req.Range != nil {
			return testPartial(payload[:4], 0, 3, int64(len(payload)), ""), nil
		}
		return testResponse(http.StatusOK, payload, nil), nil
	}), testOptions().Options)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	if len(requests) != 2 || requests[0].Range == nil || requests[1].Range != nil {
		t.Fatalf("requests = %#v, want range probe then full request", requests)
	}
}

func TestOpenMutableWithStrongETagUsesMultipart(t *testing.T) {
	payload := []byte("abcdefgh")
	var requests []Request
	stream, err := Open(context.Background(), MutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		requests = append(requests, req)
		if req.Range == nil {
			t.Fatal("validated mutable source unexpectedly used a full request")
		}
		start := req.Range.Start
		end := min(req.Range.End, int64(len(payload))-1)
		return testPartial(payload[start:end+1], start, end, int64(len(payload)), `"v1"`), nil
	}), testOptions().Options)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	if len(requests) != 2 || requests[1].IfRange != `"v1"` {
		t.Fatalf("requests = %#v, want validated continuation", requests)
	}
}

func TestOpenMutableSmallObjectNeedsNoValidator(t *testing.T) {
	payload := []byte("tiny")
	requests := 0
	stream, err := Open(context.Background(), MutableSource(func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		if req.Range == nil {
			t.Fatal("single-response object unexpectedly used a full request")
		}
		return testPartial(payload, 0, int64(len(payload)-1), int64(len(payload)), ""), nil
	}), Options{PartSize: 8})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !bytes.Equal(got, payload) || requests != 1 {
		t.Fatalf("ReadAll() = %q, %v; requests = %d", got, err, requests)
	}
}

func TestOpenMutableWithoutValidatorDoesNotResumeInterruptedBody(t *testing.T) {
	for _, tt := range []struct {
		name    string
		readErr error
	}{
		{name: "same read", readErr: io.ErrUnexpectedEOF},
		{name: "subsequent EOF"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			stream, err := Open(context.Background(), MutableSource(func(_ context.Context, _ Request) (*http.Response, error) {
				requests++
				return &http.Response{
					StatusCode: http.StatusPartialContent,
					Header:     http.Header{"Content-Range": {"bytes 0-3/4"}},
					Body:       &scriptedBody{payload: []byte("ab"), readErr: tt.readErr},
				}, nil
			}), Options{PartSize: 4, MaxPartRetries: 1})
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer stream.Body.Close()
			_, err = io.ReadAll(stream.Body)
			requireProblem(t, err, errs.SubtypeNetworkTransport, true, "range response body ended")
			if requests != 1 {
				t.Fatalf("requests = %d, mutable source without a validator must restart as a new transfer", requests)
			}
		})
	}
}

func TestOpenContinuesWithoutValidator(t *testing.T) {
	payload := []byte("abcdefgh")
	var ifRanges []string
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		ifRanges = append(ifRanges, req.IfRange)
		start := req.Range.Start
		end := min(req.Range.End, int64(len(payload))-1)
		return testPartial(payload[start:end+1], start, end, int64(len(payload)), ""), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	for i, value := range ifRanges {
		if value != "" {
			t.Fatalf("request %d If-Range = %q, want empty", i, value)
		}
	}
}

func TestOpenRetriesInterruptedBodyFromDeliveredOffset(t *testing.T) {
	for _, tt := range []struct {
		name    string
		readErr error
	}{
		{name: "same read", readErr: io.ErrUnexpectedEOF},
		{name: "subsequent EOF"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte("abcdefgh")
			requests := 0
			var ranges []ByteRange
			opts := testOptions()
			opts.MaxPartRetries = 1
			stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
				requests++
				ranges = append(ranges, *req.Range)
				switch requests {
				case 1:
					return &http.Response{
						StatusCode: http.StatusPartialContent,
						Header:     http.Header{"Content-Range": {"bytes 0-3/8"}, "Etag": {`"v1"`}},
						Body:       &scriptedBody{payload: []byte("ab"), readErr: tt.readErr},
					}, nil
				default:
					if req.IfRange != `"v1"` {
						t.Fatalf("If-Range = %q, want v1", req.IfRange)
					}
					start := req.Range.Start
					end := min(req.Range.End, int64(len(payload))-1)
					return testPartial(payload[start:end+1], start, end, int64(len(payload)), `"v1"`), nil
				}
			}, opts)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer stream.Body.Close()
			got, err := io.ReadAll(stream.Body)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload = %q, want %q", got, payload)
			}
			want := []ByteRange{{Start: 0, End: 3}, {Start: 2, End: 5}, {Start: 6, End: 7}}
			if fmt.Sprint(ranges) != fmt.Sprint(want) {
				t.Fatalf("ranges = %v, want exact resume ranges %v", ranges, want)
			}
		})
	}
}

func TestOpenUsesValidatedMidstreamFullResponse(t *testing.T) {
	payload := []byte("abcdefghijkl")
	requests := 0
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return testPartial(payload[:4], 0, 3, int64(len(payload)), ""), nil
		}
		if req.Range == nil || req.Range.Start != 4 {
			t.Fatalf("follow-up request = %#v, want exact range from byte 4", req)
		}
		return testResponse(http.StatusOK, payload, nil), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want the existing full response to be reused", requests)
	}
}

func TestOpenTreats200WithIfRangeAsRepresentationChange(t *testing.T) {
	payload := []byte("abcdefgh")
	requests := 0
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return testPartial(payload[:4], 0, 3, int64(len(payload)), `"v1"`), nil
		}
		if req.Range != nil && req.IfRange != `"v1"` {
			t.Fatalf("IfRange = %q, want v1", req.IfRange)
		}
		return testResponse(http.StatusOK, payload, http.Header{"Etag": {`"v2"`}}), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkRepresentationChanged, true, "validator")
}

func TestOpenTreatsUnvalidatedTotalChangeAsRepresentationChange(t *testing.T) {
	requests := 0
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			return testPartial([]byte("abcd"), 0, 3, 8, ""), nil
		}
		if req.Range == nil || req.Range.Start != 4 {
			t.Fatalf("follow-up request = %#v, want byte 4", req)
		}
		return testPartial([]byte("efgh"), 4, 7, 9, ""), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkRepresentationChanged, true, "resource size changed")
}

func TestOpenRejectsBodyLengthContradiction(t *testing.T) {
	stream, err := openTest(context.Background(), func(_ context.Context, _ Request) (*http.Response, error) {
		return testPartial([]byte("abcde"), 0, 3, 8, `"v1"`), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "range response delivered")
}

func TestOpenCapsResponseAmplification(t *testing.T) {
	const total = int64(128)
	payload := bytes.Repeat([]byte("x"), int(total))
	requests := 0
	opts := testOptions()
	opts.MaxResponses = 5
	stream, err := openTest(context.Background(), func(_ context.Context, req Request) (*http.Response, error) {
		requests++
		start := req.Range.Start
		return testPartial(payload[start:start+1], start, start, total, `"v1"`), nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "range responses; giving up")
	if requests != opts.MaxResponses {
		t.Fatalf("requests = %d, want ceiling %d", requests, opts.MaxResponses)
	}
}

func TestStreamCannotReopenAfterClose(t *testing.T) {
	requests := 0
	stream, err := openTest(context.Background(), func(_ context.Context, _ Request) (*http.Response, error) {
		requests++
		return testPartial([]byte("abcd"), 0, 3, 8, `"v1"`), nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := stream.Body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := stream.Body.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read() error = %v, want io.ErrClosedPipe", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want no request after Close", requests)
	}
}

func TestParseContentRange(t *testing.T) {
	tests := []struct {
		value   string
		want    contentRange
		wantErr string
	}{
		{value: "bytes 0-15/16", want: contentRange{start: 0, end: 15, total: 16}},
		{value: "BYTES 4-7/16", want: contentRange{start: 4, end: 7, total: 16}},
		{value: "", wantErr: "content-range is empty"},
		{value: "items 0-1/2", wantErr: "unsupported content-range"},
		{value: "bytes 0-15/", wantErr: "unsupported content-range"},
		{value: "bytes 0-15", wantErr: "unsupported content-range"},
		{value: "bytes 0-/16", wantErr: "unsupported content-range"},
		{value: "bytes */16", wantErr: "unsupported content-range"},
		{value: "bytes 0-1/*", wantErr: "unknown total size"},
		{value: "bytes nope-1/2", wantErr: "parse range start"},
		{value: "bytes 0-nope/2", wantErr: "parse range end"},
		{value: "bytes 0-1/nope", wantErr: "parse total size"},
		{value: "bytes 0-0/0", wantErr: "invalid total size"},
		{value: "bytes -1-0/2", wantErr: "unsupported content-range"},
		{value: "bytes 0--1/2", wantErr: "invalid negative content range"},
		{value: "bytes 2-1/3", wantErr: "start 2 is after end 1"},
		{value: "bytes 0-3/3", wantErr: "end 3 is outside total 3"},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseContentRange(tt.value)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want || got.length() != tt.want.end-tt.want.start+1 {
				t.Fatalf("parseContentRange() = %+v, %v; want %+v", got, err, tt.want)
			}
		})
	}
}

func TestStrongETag(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   string
		ok     bool
	}{
		{name: "strong", header: http.Header{"Etag": {`"abc"`}, "Last-Modified": {"Wed, 21 Oct 2015 07:28:00 GMT"}}, want: `"abc"`, ok: true},
		{name: "surrounding whitespace", header: http.Header{"Etag": {`  "abc"  `}}, want: `"abc"`, ok: true},
		{name: "empty strong", header: http.Header{"Etag": {`""`}}, want: `""`, ok: true},
		{name: "weak", header: http.Header{"Etag": {`W/"abc"`}}},
		{name: "malformed", header: http.Header{"Etag": {"abc"}}},
		{name: "unterminated", header: http.Header{"Etag": {`"abc`}}},
		{name: "embedded quote", header: http.Header{"Etag": {`"ab"c"`}}},
		{name: "control character", header: http.Header{"Etag": {"\"a\tb\""}}},
		{name: "obs text", header: http.Header{"Etag": {"\"a\xc3\xa9\""}}, want: "\"a\xc3\xa9\"", ok: true},
		{name: "multiple", header: http.Header{"Etag": {`"a"`, `"b"`}}},
		{name: "date only", header: http.Header{"Last-Modified": {"Wed, 21 Oct 2015 07:28:00 GMT"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := strongETag(tt.header)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("strongETag() = %q, %v; want %q, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestResponseLimit(t *testing.T) {
	if got := responseLimit(1, 8); got != 64 {
		t.Fatalf("small response limit = %d, want 64", got)
	}
	if got := responseLimit(400*8, 8); got != 1600 {
		t.Fatalf("large response limit = %d, want 1600", got)
	}
}

func TestOpenRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		source Source
		opts   Options
	}{
		{name: "unconfigured source"},
		{name: "unspecified representation", source: Source{transport: unusedFetch}},
		{name: "negative part size", source: immutableSource(unusedFetch), opts: Options{PartSize: -1}},
		{name: "negative max responses", source: immutableSource(unusedFetch), opts: Options{MaxResponses: -1}},
		{name: "negative part retries", source: immutableSource(unusedFetch), opts: Options{MaxPartRetries: -1}},
		{name: "negative retry delay", source: immutableSource(unusedFetch), opts: Options{RetryDelay: -time.Second}},
		{name: "negative retry wait budget", source: immutableSource(unusedFetch), opts: Options{RetryWaitBudget: -time.Second}},
		{name: "negative idle timeout", source: immutableSource(unusedFetch), opts: Options{IdleTimeout: -time.Second}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Open(context.Background(), tt.source, tt.opts)
			p, ok := errs.ProblemOf(err)
			if !ok || p.Category != errs.CategoryInternal {
				t.Fatalf("problem = %#v, %v, want internal error", p, ok)
			}
		})
	}
}

func TestOpenRejectsInvalidInitialResponses(t *testing.T) {
	tests := []struct {
		name      string
		fetch     Transport
		subtype   errs.Subtype
		retryable bool
		message   string
	}{
		{
			name: "fetch error",
			fetch: func(context.Context, Request) (*http.Response, error) {
				return nil, errs.NewNetworkError(errs.SubtypeNetworkTimeout, "probe timeout")
			},
			subtype: errs.SubtypeNetworkTimeout,
			message: "probe timeout",
		},
		{name: "nil response", fetch: func(context.Context, Request) (*http.Response, error) { return nil, nil }, subtype: errs.SubtypeNetworkProtocol, message: "empty response"},
		{name: "redirect", fetch: staticFetch(testResponse(http.StatusFound, nil, nil)), subtype: errs.SubtypeNetworkProtocol, message: "status 302"},
		{name: "server error", fetch: staticFetch(testResponse(http.StatusBadGateway, nil, nil)), subtype: errs.SubtypeNetworkProtocol, message: "status 502"},
		{name: "missing content range", fetch: staticFetch(testResponse(http.StatusPartialContent, []byte("x"), nil)), subtype: errs.SubtypeNetworkProtocol, message: "content-range is empty"},
		{name: "wrong initial start", fetch: staticFetch(testPartial([]byte("b"), 1, 1, 2, `"v1"`)), subtype: errs.SubtypeNetworkProtocol, message: "start at byte 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := openTest(context.Background(), tt.fetch, testOptions())
			requireProblem(t, err, tt.subtype, tt.retryable, tt.message)
		})
	}
}

func TestOpenRejectsInvalidFullResponseFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		second   *http.Response
		fetchErr error
		message  string
	}{
		{name: "fetch error", fetchErr: errs.NewNetworkError(errs.SubtypeNetworkTimeout, "fallback timeout"), message: "fallback timeout"},
		{name: "nil response", message: "empty response"},
		{name: "not full response", second: testPartial([]byte("abcd"), 0, 3, 8, ""), message: "returned HTTP 206, want 200"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			_, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return testResponse(http.StatusRequestedRangeNotSatisfiable, nil, nil), nil
				}
				return tt.second, tt.fetchErr
			}, testOptions())
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("Open() error = %v, want %q", err, tt.message)
			}
		})
	}
}

func TestOpenRejectsInvalidFollowupResponses(t *testing.T) {
	tests := []struct {
		name      string
		firstETag string
		second    *http.Response
		subtype   errs.Subtype
		retryable bool
		message   string
	}{
		{name: "nil", firstETag: `"v1"`, subtype: errs.SubtypeNetworkProtocol, message: "empty response"},
		{name: "unexpected status", firstETag: `"v1"`, second: testResponse(http.StatusTeapot, nil, nil), subtype: errs.SubtypeNetworkProtocol, message: "status 418"},
		{name: "invalid content range", firstETag: `"v1"`, second: testResponse(http.StatusPartialContent, []byte("efgh"), nil), subtype: errs.SubtypeNetworkProtocol, message: "content-range is empty"},
		{name: "validator disappeared", firstETag: `"v1"`, second: testPartial([]byte("efgh"), 4, 7, 8, ""), subtype: errs.SubtypeNetworkProtocol, message: "no usable validator"},
		{name: "validator changed", firstETag: `"v1"`, second: testPartial([]byte("efgh"), 4, 7, 8, `"v2"`), subtype: errs.SubtypeNetworkRepresentationChanged, retryable: true, message: "carries validator"},
		{name: "if-range ignored", firstETag: `"v1"`, second: testPartial([]byte("efgh"), 4, 7, 9, `"v1"`), subtype: errs.SubtypeNetworkProtocol, message: "ignored If-Range"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return testPartial([]byte("abcd"), 0, 3, 8, tt.firstETag), nil
				}
				return tt.second, nil
			}, testOptions())
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer stream.Body.Close()
			_, err = io.ReadAll(stream.Body)
			requireProblem(t, err, tt.subtype, tt.retryable, tt.message)
		})
	}
}

func TestOpenFullResponseContinuationFailures(t *testing.T) {
	tests := []struct {
		name      string
		full      *http.Response
		subtype   errs.Subtype
		retryable bool
		message   string
	}{
		{name: "size changed", full: testResponse(http.StatusOK, []byte("123456789"), nil), subtype: errs.SubtypeNetworkRepresentationChanged, retryable: true, message: "full response has 9 bytes"},
		{name: "short prefix", full: testUnknownLengthResponse(http.StatusOK, []byte("ab"), nil), subtype: errs.SubtypeNetworkProtocol, message: "ended before byte 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return testPartial([]byte("abcd"), 0, 3, 8, ""), nil
				}
				return tt.full, nil
			}, testOptions())
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer stream.Body.Close()
			_, err = io.ReadAll(stream.Body)
			requireProblem(t, err, tt.subtype, tt.retryable, tt.message)
		})
	}
}

func TestOpenIgnoresCloseErrorAfterExactPart(t *testing.T) {
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Range": {"bytes 0-3/4"}},
			Body:       &scriptedBody{payload: []byte("abcd"), readErr: io.EOF, closeErr: errors.New("close failed")},
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || string(got) != "abcd" {
		t.Fatalf("ReadAll() = %q, %v", got, err)
	}
}

func TestOpenRejectsPartOverflow(t *testing.T) {
	stream, err := openTest(context.Background(), func(context.Context, Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Range": {"bytes 0-3/4"}},
			Body:       io.NopCloser(strings.NewReader("abcde")),
		}, nil
	}, testOptions())
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkProtocol, false, "range response delivered")
}

type testConfig struct {
	Options
}

func testOptions() testConfig {
	return testConfig{Options: Options{PartSize: 4}}
}

func openTest(ctx context.Context, fetch Transport, cfg testConfig) (*Stream, error) {
	return Open(ctx, immutableSource(fetch), cfg.Options)
}

func immutableSource(fetch Transport) Source {
	return ImmutableSource(fetch)
}

func testPartial(body []byte, start, end, total int64, etag string) *http.Response {
	header := http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/%d", start, end, total)}}
	if etag != "" {
		header.Set("ETag", etag)
	}
	return testResponse(http.StatusPartialContent, body, header)
}

func testResponse(status int, body []byte, header http.Header) *http.Response {
	if header == nil {
		header = make(http.Header)
	}
	return &http.Response{
		StatusCode:    status,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func testUnknownLengthResponse(status int, body []byte, header http.Header) *http.Response {
	resp := testResponse(status, body, header)
	resp.ContentLength = -1
	return resp
}

func staticFetch(resp *http.Response) Transport {
	return func(context.Context, Request) (*http.Response, error) { return resp, nil }
}

func unusedFetch(context.Context, Request) (*http.Response, error) {
	panic("fetch must not be called for invalid options")
}

func requireProblem(t *testing.T, err error, subtype errs.Subtype, retryable bool, message string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), message) {
		t.Fatalf("error = %v, want message containing %q", err, message)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryNetwork || p.Subtype != subtype || p.Retryable != retryable {
		t.Fatalf("problem = %#v, %v; want network/%s retryable=%v", p, ok, subtype, retryable)
	}
}

type errorBody struct {
	err error
}

type scriptedBody struct {
	payload   []byte
	readErr   error
	closeErr  error
	delivered bool
}

func (b *scriptedBody) Read(p []byte) (int, error) {
	if b.delivered {
		return 0, io.EOF
	}
	b.delivered = true
	return copy(p, b.payload), b.readErr
}

func (b *scriptedBody) Close() error { return b.closeErr }

func (b errorBody) Read([]byte) (int, error) { return 0, b.err }
func (b errorBody) Close() error             { return nil }

func TestOpenPreservesTypedReadError(t *testing.T) {
	want := errs.NewNetworkError(errs.SubtypeNetworkTimeout, "timed out").WithRetryable()
	stream, err := openTest(context.Background(), func(_ context.Context, _ Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Header:     http.Header{"Content-Range": {"bytes 0-3/8"}, "Etag": {`"v1"`}},
			Body:       errorBody{err: want},
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
