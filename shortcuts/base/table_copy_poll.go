// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"time"

	"github.com/larksuite/cli/errs"
)

const (
	tableCopyPollInitial = 3 * time.Second
	tableCopyPollMax     = 30 * time.Second
)

type tableCopyTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type tableCopyClock interface {
	Now() time.Time
	NewTimer(time.Duration) tableCopyTimer
}

type tableCopyStatusFetcher func(context.Context) (tableCopyStatus, error)

type realTableCopyClock struct{}

func (realTableCopyClock) Now() time.Time { return time.Now() }

func (realTableCopyClock) NewTimer(duration time.Duration) tableCopyTimer {
	return realTableCopyTimer{Timer: time.NewTimer(duration)}
}

type realTableCopyTimer struct {
	*time.Timer
}

func (t realTableCopyTimer) C() <-chan time.Time { return t.Timer.C }

func pollTableCopy(
	ctx context.Context,
	timeout time.Duration,
	clock tableCopyClock,
	fetch tableCopyStatusFetcher,
) (tableCopyStatus, bool, error) {
	deadline := clock.Now().Add(timeout)
	delay := tableCopyPollInitial
	var lastStatus tableCopyStatus
	var lastErr error
	hasStatus := false

	for {
		remaining := deadline.Sub(clock.Now())
		if remaining <= 0 {
			if !hasStatus && lastErr != nil {
				return tableCopyStatus{}, false, lastErr
			}
			return lastStatus, true, nil
		}
		if delay > remaining {
			delay = remaining
		}

		timer := clock.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return lastStatus, false, ctx.Err()
		case <-timer.C():
		}

		if !clock.Now().Before(deadline) {
			if !hasStatus && lastErr != nil {
				return tableCopyStatus{}, false, lastErr
			}
			return lastStatus, true, nil
		}
		requestBudget := deadline.Sub(clock.Now())
		if requestBudget <= 0 {
			if !hasStatus && lastErr != nil {
				return tableCopyStatus{}, false, lastErr
			}
			return lastStatus, true, nil
		}
		fetchCtx, cancelFetch := context.WithTimeout(ctx, requestBudget)
		status, err := fetch(fetchCtx)
		cancelFetch()
		if ctx.Err() != nil {
			return lastStatus, false, ctx.Err()
		}
		if !clock.Now().Before(deadline) {
			if !hasStatus && err != nil {
				return tableCopyStatus{}, false, err
			}
			return lastStatus, true, nil
		}
		if err != nil {
			if !tableCopyPollErrorRetryable(err) {
				return lastStatus, false, err
			}
			lastErr = err
		} else {
			lastStatus = status
			hasStatus = true
			switch status.State {
			case tableCopyStateSuccess:
				return status, false, nil
			case tableCopyStateInit, tableCopyStateProcess:
			default:
				return lastStatus, false, errs.NewInternalError(errs.SubtypeInvalidResponse, "table copy status has invalid state %q", status.State)
			}
		}

		delay *= 2
		if delay > tableCopyPollMax {
			delay = tableCopyPollMax
		}
	}
}

func tableCopyPollErrorRetryable(err error) bool {
	problem, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	if problem.Category == errs.CategoryNetwork {
		switch problem.Subtype {
		case errs.SubtypeNetworkTimeout, errs.SubtypeNetworkTransport, errs.SubtypeNetworkServer:
			return true
		default:
			return false
		}
	}
	return problem.Category == errs.CategoryAPI && problem.Retryable
}
