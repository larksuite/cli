// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
)

// PageFetcher fetches one data page for the supplied opaque cursor.
type PageFetcher func(pageToken string) (map[string]any, error)

// PageConsumer folds one successful page into a caller-owned result.
type PageConsumer func(page map[string]any, pageNumber int) error

// PageWalkOptions contains transport-independent pagination policy. A zero
// PageLimit is accepted only when AllowUnlimited is true.
type PageWalkOptions struct {
	AutoPaginate   bool
	PageLimit      int
	AllowUnlimited bool
	PageDelay      time.Duration
	StartPageToken string
	ShowProgress   bool
	IsTruncated    func(page map[string]any) bool
}

type pageDelayWaiter func(context.Context, time.Duration) error

// WalkPages owns the single shortcut pagination loop. It reports neutral
// facts and preserves every page already handed to consume when a later fetch
// fails; business domains decide whether that status is complete or usable.
func WalkPages(
	runtime *RuntimeContext,
	options PageWalkOptions,
	fetch PageFetcher,
	consume PageConsumer,
) (client.PaginationStatus, error) {
	return walkPages(runtime, options, fetch, consume, waitPageDelay)
}

func walkPages(
	runtime *RuntimeContext,
	options PageWalkOptions,
	fetch PageFetcher,
	consume PageConsumer,
	wait pageDelayWaiter,
) (client.PaginationStatus, error) {
	status := client.PaginationStatus{}
	if runtime == nil || fetch == nil || consume == nil {
		err := errs.NewInternalError(errs.SubtypeUnknown, "pagination walker requires runtime, fetch, and consume")
		status.Cause = err
		return status, err
	}

	maxPages, err := pageWalkBudget(options)
	if err != nil {
		status.Cause = err
		return status, err
	}

	requestToken := options.StartPageToken
	status.HasMore = requestToken != ""
	status.NextPageToken = requestToken
	seenTokens := make(map[string]struct{})
	if requestToken != "" {
		seenTokens[requestToken] = struct{}{}
	}

	// maxPages is always positive. Explicit unlimited mode maps to the largest
	// platform int, while cursor guards keep non-advancing servers finite.
	for pageNumber := 1; pageNumber <= maxPages; pageNumber++ {
		if options.ShowProgress {
			fmt.Fprintf(runtime.IO().ErrOut, "[page %d] fetching...\n", pageNumber)
		}

		page, fetchErr := fetch(requestToken)
		if fetchErr != nil {
			status.Cause = fetchErr
			status.StopReason = paginationErrorStopReason(fetchErr)
			return status, fetchErr
		}
		if page == nil {
			fetchErr = errs.NewInternalError(errs.SubtypeInvalidResponse,
				"pagination page %d response has no data object", pageNumber)
			status.Cause = fetchErr
			status.StopReason = client.StopReasonAPIError
			return status, fetchErr
		}
		if consumeErr := consume(page, pageNumber); consumeErr != nil {
			if _, ok := errs.ProblemOf(consumeErr); !ok {
				consumeErr = errs.NewInternalError(errs.SubtypeUnknown,
					"accumulate pagination page %d: %v", pageNumber, consumeErr).
					WithCause(consumeErr)
			}
			status.Cause = consumeErr
			status.StopReason = paginationErrorStopReason(consumeErr)
			return status, consumeErr
		}

		status.PagesFetched++
		status.HasMore, status.NextPageToken = PaginationMeta(page)
		if options.IsTruncated != nil && options.IsTruncated(page) {
			status.StopReason = client.StopReasonServerTruncation
			return status, nil
		}
		if !status.HasMore {
			if options.StartPageToken != "" {
				status.StopReason = client.StopReasonStartPageToken
			} else {
				status.StopReason = client.StopReasonExhausted
			}
			status.NextPageToken = ""
			return status, nil
		}
		if status.NextPageToken == "" {
			fetchErr = invalidPageCursor("paginated response has_more=true but next page token is missing")
			status.Cause = fetchErr
			status.StopReason = client.StopReasonMissingToken
			return status, fetchErr
		}
		if _, repeated := seenTokens[status.NextPageToken]; repeated {
			fetchErr = invalidPageCursor("paginated response repeated page token, which would paginate forever")
			status.Cause = fetchErr
			status.StopReason = client.StopReasonRepeatedToken
			return status, fetchErr
		}
		if !options.AutoPaginate {
			status.StopReason = client.StopReasonSinglePage
			return status, nil
		}
		if pageNumber == maxPages {
			status.StopReason = client.StopReasonPageLimit
			return status, nil
		}

		requestToken = status.NextPageToken
		seenTokens[requestToken] = struct{}{}
		if options.PageDelay > 0 {
			ctx := runtime.Ctx()
			if ctx == nil {
				ctx = context.Background()
			}
			if waitErr := wait(ctx, options.PageDelay); waitErr != nil {
				waitErr = paginationWaitError(waitErr)
				status.Cause = waitErr
				status.StopReason = paginationErrorStopReason(waitErr)
				return status, waitErr
			}
		}
	}

	err = errs.NewInternalError(errs.SubtypeUnknown,
		"pagination exhausted its page budget without producing a terminal result")
	status.Cause = err
	status.StopReason = client.StopReasonAPIError
	return status, err
}

func pageWalkBudget(options PageWalkOptions) (int, error) {
	if !options.AutoPaginate {
		return 1, nil
	}
	if options.PageLimit > 0 {
		return options.PageLimit, nil
	}
	if options.PageLimit == 0 && options.AllowUnlimited {
		return int(^uint(0) >> 1), nil
	}
	return 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unlimited pagination requires an explicit caller policy").WithParam("--page-limit")
}

func paginationErrorStopReason(err error) client.StopReason {
	problem, ok := errs.ProblemOf(err)
	if ok && problem.Category == errs.CategoryNetwork {
		return client.StopReasonTransportError
	}
	return client.StopReasonAPIError
}

func waitPageDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func paginationWaitError(err error) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	subtype := errs.SubtypeNetworkTransport
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
	}
	return errs.NewNetworkError(subtype,
		"pagination interrupted while waiting between pages: %v", err).
		WithCause(err)
}

func invalidPageCursor(format string, args ...interface{}) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...).
		WithHint("re-run without --page-all, or report the endpoint: its pagination cursor is inconsistent")
}
