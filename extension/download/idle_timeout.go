// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package download

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/larksuite/cli/errs"
)

var errDownloadIdleTimeout = &downloadIdleTimeoutCause{}

type downloadIdleTimeoutCause struct{}

func (*downloadIdleTimeoutCause) Error() string   { return "download made no progress" }
func (*downloadIdleTimeoutCause) Timeout() bool   { return true }
func (*downloadIdleTimeoutCause) Temporary() bool { return true }

func fetchWithIdleTimeout(ctx context.Context, fetch Transport, request Request, timeout time.Duration) (*http.Response, error) {
	requestCtx, cancel := context.WithCancelCause(ctx)
	timer := startIdleTimer(timeout, cancel)
	resp, err := fetch(requestCtx, request)
	timer.stop()

	if errors.Is(context.Cause(requestCtx), errDownloadIdleTimeout) && ctx.Err() == nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		cancel(nil)
		return nil, newIdleTimeoutError(timeout)
	}
	if err != nil {
		cancel(nil)
		return resp, err
	}
	if ctx.Err() != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		cancel(nil)
		return nil, downloadContextError(ctx.Err())
	}
	if resp == nil || resp.Body == nil {
		cancel(nil)
		return resp, nil
	}
	resp.Body = &idleTimeoutBody{
		ctx:        ctx,
		requestCtx: requestCtx,
		cancel:     cancel,
		source:     resp.Body,
		timeout:    timeout,
	}
	return resp, nil
}

type idleTimeoutBody struct {
	ctx        context.Context
	requestCtx context.Context
	cancel     context.CancelCauseFunc
	source     io.ReadCloser
	timeout    time.Duration
	closeOnce  sync.Once
	closeErr   error
}

func (b *idleTimeoutBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if errors.Is(context.Cause(b.requestCtx), errDownloadIdleTimeout) && b.ctx.Err() == nil {
		return 0, newIdleTimeoutError(b.timeout)
	}

	timer := startIdleTimer(b.timeout, b.cancel)
	n, err := b.source.Read(p)
	timer.stop()
	if errors.Is(context.Cause(b.requestCtx), errDownloadIdleTimeout) && b.ctx.Err() == nil {
		return n, newIdleTimeoutError(b.timeout)
	}
	if n == 0 && err == nil {
		return 0, io.ErrNoProgress
	}
	if err == io.EOF {
		b.cancel(nil)
	}
	return n, err
}

func (b *idleTimeoutBody) Close() error {
	b.closeOnce.Do(func() {
		b.cancel(nil)
		b.closeErr = b.source.Close()
	})
	return b.closeErr
}

type idleTimer struct {
	timer *time.Timer
	fired chan struct{}
}

func startIdleTimer(timeout time.Duration, cancel context.CancelCauseFunc) *idleTimer {
	t := &idleTimer{fired: make(chan struct{})}
	t.timer = time.AfterFunc(timeout, func() {
		cancel(errDownloadIdleTimeout)
		close(t.fired)
	})
	return t
}

func (t *idleTimer) stop() {
	if t.timer.Stop() {
		return
	}
	<-t.fired
}

func newIdleTimeoutError(timeout time.Duration) *errs.NetworkError {
	return errs.NewNetworkError(errs.SubtypeNetworkTimeout,
		"download received no response data for %s", timeout).
		WithRetryable().
		WithHint("retry the download; the server or network stopped making progress").
		WithCause(errDownloadIdleTimeout)
}
