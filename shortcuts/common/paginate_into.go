// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	internalpagination "github.com/larksuite/cli/internal/pagination"
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
// --page-token sets the starting cursor; --page-all independently controls
// whether the walk continues from that cursor. When both are supplied, the
// walk starts at --page-token and continues until exhaustion or --page-limit.
// Multi-page runs wait --page-delay between successful page requests;
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
	ctx := runtime.Ctx()
	if ctx == nil {
		ctx = context.Background()
	}
	walk := pageWalk{
		policy:  policy,
		request: request,
		wait:    wait,
		// The runtime carries its own context, so this fetch ignores the
		// walker's -- unlike the externally declared commands, whose context
		// arrives with the call.
		fetch: func(_ context.Context, page PageRequest) (map[string]interface{}, error) {
			return runtime.CallAPITyped(page.Method, page.Path, page.Params, page.Body)
		},
		accumulate: func(data map[string]interface{}, pageNumber int) error {
			return addDecodedPage(data, pageNumber, dst)
		},
	}
	if policy.showProgress {
		walk.progress = runtime.IO().ErrOut
	}
	state, walkErr := walk.run(ctx)
	meta.Complete = state.Complete
	meta.Pages = state.Pages
	meta.NextToken = state.NextToken
	return meta, walkErr
}

// pageWalk is one pagination run. Built-in shortcuts and externally declared
// commands differ only in where the policy comes from, how a page is fetched
// and what accumulates it; the cursor walk, the inter-page delay and the error
// mapping are the same and live here.
type pageWalk struct {
	policy     paginationPolicy
	request    PageRequest
	fetch      func(context.Context, PageRequest) (map[string]interface{}, error)
	accumulate func(data map[string]interface{}, pageNumber int) error
	wait       pageDelayWaiter
	progress   io.Writer // nil when the run reports no per-page progress
}

func (w pageWalk) run(ctx context.Context) (internalpagination.State, error) {
	state, walkErr := internalpagination.Walk(ctx, internalpagination.Options{
		InitialToken: pageTokenParam(w.request.Params),
		MaxPages:     w.policy.maxPages,
		Delay:        w.policy.pageDelay,
		Wait:         w.wait,
		Fetch: func(ctx context.Context, pageNumber int, pageToken string) (bool, string, error) {
			page := w.request
			page.Params = clonePageParams(w.request.Params)
			if pageToken != "" {
				page.Params["page_token"] = pageToken
			}
			if w.progress != nil {
				fmt.Fprintf(w.progress, "[page %d] fetching...\n", pageNumber)
			}

			data, err := w.fetch(ctx, page)
			if err != nil {
				return false, "", err
			}
			if err := w.accumulate(data, pageNumber); err != nil {
				return false, "", err
			}
			hasMore, nextPageToken := PaginationMeta(data)
			return hasMore, nextPageToken, nil
		},
	})
	if walkErr != nil {
		return state, paginationWalkError(walkErr)
	}
	return state, nil
}

// addDecodedPage keeps the typed-accumulator half of the walk out of pageWalk,
// which stays generic-free so both entry points can share one struct.
func addDecodedPage[T any](data map[string]interface{}, pageNumber int, dst PageAccumulator[T]) error {
	page, err := decodePageData[T](data, pageNumber)
	if err != nil {
		return err
	}
	if err := dst.AddPage(page); err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return err
		}
		return errs.NewInternalError(errs.SubtypeUnknown,
			"accumulate pagination page %d: %v", pageNumber, err).
			WithCause(err)
	}
	return nil
}

func paginationWalkError(walkErr error) error {
	var cursorErr *internalpagination.CursorError
	if errors.As(walkErr, &cursorErr) {
		if cursorErr.Kind == internalpagination.CursorMissing {
			return invalidPageCursor("response reports more pages but returned no page token")
		}
		return invalidPageCursor("response repeated page token %q, which would paginate forever", cursorErr.Token)
	}
	var waitErr *internalpagination.WaitError
	if errors.As(walkErr, &waitErr) {
		return paginationWaitError(waitErr.Err)
	}
	if _, ok := errs.ProblemOf(walkErr); ok {
		return walkErr
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "paginate: %v", walkErr).WithCause(walkErr)
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
		showProgress: paginationProgressEnabled(runtime),
	}, nil
}

// paginationProgressEnabled keeps stderr suitable for its actual consumer.
// Human progress is useful only on an interactive diagnostics stream. CSV and
// NDJSON reserve stderr for the emitter's one-object-per-line structured
// pagination diagnostic, even when stderr happens to be a terminal. JQ owns
// the effective output contract when present, just as it does in Emitter.
func paginationProgressEnabled(runtime *RuntimeContext) bool {
	if runtime == nil || !runtime.IO().StderrIsTerminal {
		return false
	}
	if runtime.JqExpr != "" {
		return true
	}
	format, known := output.ParseFormat(runtime.Format)
	return !known || (format != output.FormatCSV && format != output.FormatNDJSON)
}

func waitPageDelay(ctx context.Context, delay time.Duration) error {
	return internalpagination.WaitContext(ctx, delay)
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
