// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
)

// PageRequest describes one paginated API walk. Pagination controls are not
// repeated here: PaginateInto derives the policy from the command's standard
// --page-all and --page-limit flags.
type PageRequest struct {
	Method string
	Path   string
	Params map[string]interface{}
	Body   interface{}
}

// PageAccumulator owns the business-specific meaning of combining pages.
// Framework pagination deliberately knows nothing about item field names or
// whether non-item fields come from the first, last, or every page.
type PageAccumulator[T any] interface {
	AddPage(T) error
}

// PaginateInto walks an endpoint and decodes each successful data object into
// T before handing it to dst. A normal invocation and --page-all use the same
// path: the former has a one-page policy, while the latter uses --page-limit.
// An explicit --page-token is only the starting cursor and never changes that
// policy. Multi-page runs wait --page-delay between successful page requests;
// the wait is context-aware and never occurs before page 1 or after the final
// page.
//
// The returned metadata describes the fetch stage. Callers that apply global
// filters or enrichment should set Items to the final emitted record count.
// Keeping the typed-page boundary here also keeps shortcut call sites stable
// when the transport supplies a response-native decode method.
func PaginateInto[T any](runtime *RuntimeContext, request PageRequest, dst PageAccumulator[T]) (*output.PaginationMeta, error) {
	return paginateInto(runtime, request, dst, waitPageDelay)
}

type pageDelayWaiter func(context.Context, time.Duration) error

func paginateInto[T any](runtime *RuntimeContext, request PageRequest, dst PageAccumulator[T], wait pageDelayWaiter) (*output.PaginationMeta, error) {
	meta := &output.PaginationMeta{}
	policy, err := resolvePaginationPolicy(runtime)
	if err != nil {
		return meta, err
	}

	pageToken := pageTokenParam(request.Params)
	seen := make(map[string]struct{})
	if pageToken != "" {
		seen[pageToken] = struct{}{}
	}

	// maxPages is always in [1, pageLimitMaximum]. Keeping the bound in the
	// loop statement makes finite execution a structural invariant, independent
	// of cursor quality and of any future exit-condition changes below.
	for pageNumber := 1; pageNumber <= policy.maxPages; pageNumber++ {
		params := clonePageParams(request.Params)
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		if policy.showProgress {
			fmt.Fprintf(runtime.IO().ErrOut, "[page %d] fetching...\n", pageNumber)
		}

		data, err := runtime.CallAPITyped(request.Method, request.Path, params, request.Body)
		if err != nil {
			meta.NextToken = pageToken
			return meta, err
		}
		page, err := decodePageData[T](data, pageNumber)
		if err != nil {
			meta.NextToken = pageToken
			return meta, err
		}
		if err := dst.AddPage(page); err != nil {
			meta.NextToken = pageToken
			if _, ok := errs.ProblemOf(err); ok {
				return meta, err
			}
			return meta, errs.NewInternalError(errs.SubtypeUnknown,
				"accumulate pagination page %d: %v", pageNumber, err).
				WithCause(err)
		}
		meta.Pages++

		hasMore, nextPageToken := PaginationMeta(data)
		if !hasMore {
			meta.Complete = true
			meta.NextToken = ""
			return meta, nil
		}
		if nextPageToken == "" {
			return meta, invalidPageCursor("response reports more pages but returned no page token")
		}
		if _, repeated := seen[nextPageToken]; repeated {
			return meta, invalidPageCursor("response repeated page token %q, which would paginate forever", nextPageToken)
		}

		meta.NextToken = nextPageToken
		if pageNumber == policy.maxPages {
			return meta, nil
		}

		seen[nextPageToken] = struct{}{}
		pageToken = nextPageToken
		if policy.pageDelay > 0 {
			ctx := runtime.Ctx()
			if ctx == nil {
				ctx = context.Background()
			}
			if err := wait(ctx, policy.pageDelay); err != nil {
				return meta, paginationWaitError(err)
			}
		}
	}

	return meta, errs.NewInternalError(errs.SubtypeUnknown,
		"pagination exhausted its page budget without producing a terminal result")
}

type paginationPolicy struct {
	maxPages     int
	pageDelay    time.Duration
	showProgress bool
}

// resolvePaginationPolicy resolves the framework's standard list semantics.
// Even a one-page call is a pagination run; --page-all only changes its page
// budget and progress presentation.
func resolvePaginationPolicy(runtime *RuntimeContext) (paginationPolicy, error) {
	config, err := pageAllValues(runtime)
	if err != nil {
		return paginationPolicy{}, err
	}
	if !config.enabled {
		return paginationPolicy{maxPages: 1}, nil
	}
	return paginationPolicy{
		maxPages:     config.maxPages,
		pageDelay:    config.delay,
		showProgress: true,
	}, nil
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

// decodePageData isolates the current map-returning RuntimeContext boundary.
// A response-native decoder can replace this adapter without changing either
// PaginateInto's public contract or any shortcut accumulator.
func decodePageData[T any](data map[string]interface{}, pageNumber int) (T, error) {
	var page T
	if data == nil {
		return page, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"pagination page %d response has no data object", pageNumber)
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return page, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"encode pagination page %d for typed decoding: %v", pageNumber, err).
			WithCause(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&page); err != nil {
		return page, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"decode pagination page %d: %v", pageNumber, err).
			WithCause(err)
	}
	return page, nil
}

func clonePageParams(params map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(params)+1)
	for name, value := range params {
		cloned[name] = value
	}
	return cloned
}

func pageTokenParam(params map[string]interface{}) string {
	switch value := params["page_token"].(type) {
	case string:
		return value
	case []string:
		if len(value) > 0 {
			return value[0]
		}
	case []interface{}:
		if len(value) > 0 {
			pageToken, _ := value[0].(string)
			return pageToken
		}
	}
	return ""
}

func invalidPageCursor(format string, args ...interface{}) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse, format, args...).
		WithHint("re-run without --page-all, or report the endpoint: its pagination cursor is inconsistent")
}
