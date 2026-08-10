// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func TestOpenRetriesResponseHeaderIdleTimeout(t *testing.T) {
	attempts := 0
	opts := testOptions()
	opts.IdleTimeout = 10 * time.Millisecond
	opts.MaxPartRetries = 1
	stream, err := openTest(context.Background(), func(ctx context.Context, _ Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return testResponse(http.StatusOK, []byte("ok"), nil), nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || string(got) != "ok" || attempts != 2 {
		t.Fatalf("ReadAll() = %q, %v; attempts = %d, want ok after one retry", got, err, attempts)
	}
}

func TestOpenResumesAfterBodyIdleTimeout(t *testing.T) {
	payload := []byte("abcdefgh")
	var ranges []ByteRange
	opts := testOptions()
	opts.IdleTimeout = 10 * time.Millisecond
	opts.MaxPartRetries = 1
	stream, err := openTest(context.Background(), func(ctx context.Context, req Request) (*http.Response, error) {
		ranges = append(ranges, *req.Range)
		if len(ranges) == 1 {
			return &http.Response{
				StatusCode: http.StatusPartialContent,
				Header:     http.Header{"Content-Range": {"bytes 0-3/8"}, "Etag": {`"v1"`}},
				Body:       &stallAfterBody{ctx: ctx, payload: payload[:2]},
			}, nil
		}
		start := req.Range.Start
		end := min(req.Range.End, int64(len(payload))-1)
		return testPartial(payload[start:end+1], start, end, int64(len(payload)), `"v1"`), nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !slices.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
	want := []ByteRange{{Start: 0, End: 3}, {Start: 2, End: 5}, {Start: 6, End: 7}}
	if !slices.Equal(ranges, want) {
		t.Fatalf("ranges = %#v, want %#v", ranges, want)
	}
}

func TestOpenFullResponseBodyIdleTimeoutIsTyped(t *testing.T) {
	opts := testOptions()
	opts.DisableMultipart = true
	opts.IdleTimeout = 10 * time.Millisecond
	stream, err := openTest(context.Background(), func(ctx context.Context, _ Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          &stallAfterBody{ctx: ctx},
			ContentLength: 4,
		}, nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	_, err = io.ReadAll(stream.Body)
	requireProblem(t, err, errs.SubtypeNetworkTimeout, true, "no response data")
	if !errors.Is(err, errDownloadIdleTimeout) {
		t.Fatalf("ReadAll() error = %v, want idle timeout cause", err)
	}
}

func TestOpenIdleTimeoutResetsOnProgress(t *testing.T) {
	payload := []byte("progress-keeps-the-stream-alive")
	opts := testOptions()
	opts.DisableMultipart = true
	opts.IdleTimeout = 100 * time.Millisecond
	stream, err := openTest(context.Background(), func(ctx context.Context, _ Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          &pacedBody{ctx: ctx, payload: payload, delay: 10 * time.Millisecond},
			ContentLength: int64(len(payload)),
		}, nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()
	got, err := io.ReadAll(stream.Body)
	if err != nil || !slices.Equal(got, payload) {
		t.Fatalf("ReadAll() = %q, %v; want %q", got, err, payload)
	}
}

func TestOpenIdleTimeoutExcludesConsumerPauses(t *testing.T) {
	opts := testOptions()
	opts.DisableMultipart = true
	opts.IdleTimeout = 10 * time.Millisecond
	stream, err := openTest(context.Background(), staticFetch(testResponse(http.StatusOK, []byte("abcd"), nil)), opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer stream.Body.Close()

	first := make([]byte, 1)
	if n, err := stream.Body.Read(first); n != 1 || err != nil || string(first) != "a" {
		t.Fatalf("first Read() = %q, %d, %v", first, n, err)
	}
	time.Sleep(3 * opts.IdleTimeout)
	rest, err := io.ReadAll(stream.Body)
	if err != nil || string(rest) != "bcd" {
		t.Fatalf("ReadAll() after pause = %q, %v; want bcd", rest, err)
	}
}

func TestOpenCallerCancellationWinsOverIdleTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	opts := testOptions()
	opts.IdleTimeout = time.Minute
	go func() {
		<-started
		cancel()
	}()
	_, err := openTest(ctx, func(ctx context.Context, _ Request) (*http.Response, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}, opts)
	requireProblem(t, err, errs.SubtypeNetworkTransport, false, "canceled")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Open() error = %v, want caller cancellation cause", err)
	}
}

func TestOpenCallerDeadlineWinsOverIdleTimeout(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	opts := testOptions()
	opts.IdleTimeout = time.Minute
	_, err := openTest(ctx, func(ctx context.Context, _ Request) (*http.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}, opts)
	requireProblem(t, err, errs.SubtypeNetworkTimeout, false, "timed out")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Open() error = %v, want caller deadline cause", err)
	}
}

func TestOpenCloseInterruptsInFlightRead(t *testing.T) {
	started := make(chan struct{})
	opts := testOptions()
	opts.DisableMultipart = true
	opts.IdleTimeout = time.Minute
	stream, err := openTest(context.Background(), func(ctx context.Context, _ Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        make(http.Header),
			Body:          &stallAfterBody{ctx: ctx, started: started},
			ContentLength: -1,
		}, nil
	}, opts)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	readDone := make(chan error, 1)
	go func() {
		_, readErr := stream.Body.Read(make([]byte, 1))
		readDone <- readErr
	}()
	<-started
	if err := stream.Body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	select {
	case <-readDone:
	case <-time.After(time.Second):
		t.Fatal("Close() did not interrupt the in-flight read")
	}
}

type stallAfterBody struct {
	ctx       context.Context
	payload   []byte
	delivered bool
	started   chan struct{}
}

func (b *stallAfterBody) Read(p []byte) (int, error) {
	if !b.delivered && len(b.payload) > 0 {
		n := copy(p, b.payload)
		b.payload = b.payload[n:]
		return n, nil
	}
	if !b.delivered {
		b.delivered = true
		if b.started != nil {
			close(b.started)
		}
	}
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*stallAfterBody) Close() error { return nil }

type pacedBody struct {
	ctx     context.Context
	payload []byte
	delay   time.Duration
}

func (b *pacedBody) Read(p []byte) (int, error) {
	if len(b.payload) == 0 {
		return 0, io.EOF
	}
	timer := time.NewTimer(b.delay)
	defer timer.Stop()
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-timer.C:
	}
	p[0] = b.payload[0]
	b.payload = b.payload[1:]
	return 1, nil
}

func (*pacedBody) Close() error { return nil }
