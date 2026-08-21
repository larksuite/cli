// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"bytes"
	"encoding/json"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
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
// --page-token sets the starting cursor; --page-all independently controls
// whether the walk continues from that cursor. When both are supplied, the
// walk starts at --page-token and continues until exhaustion or --page-limit.
// Multi-page runs wait --page-delay between successful page requests;
// the wait is context-aware and never occurs before page 1 or after the final
// page.
//
// The returned metadata and neutral status describe the fetch stage. Callers
// that apply global filters or enrichment should set Items to the final emitted
// record count. When err is non-nil and status.PagesFetched > 0, dst and status
// retain the successful prefix so a domain contract can emit an honest partial
// result instead of discarding it.
// Keeping the typed-page boundary here also keeps shortcut call sites stable
// when the transport supplies a response-native decode method.
func PaginateInto[T any](runtime *RuntimeContext, request PageRequest, dst PageAccumulator[T], policy PageAllPolicy) (*output.PaginationMeta, client.PaginationStatus, error) {
	return paginateInto(runtime, request, dst, policy, waitPageDelay)
}

func paginateInto[T any](runtime *RuntimeContext, request PageRequest, dst PageAccumulator[T], policy PageAllPolicy, wait pageDelayWaiter) (*output.PaginationMeta, client.PaginationStatus, error) {
	meta := &output.PaginationMeta{}
	walkOptions, err := resolvePaginationPolicy(runtime, policy)
	if err != nil {
		return meta, client.PaginationStatus{}, err
	}
	walkOptions.StartPageToken = pageTokenParam(request.Params)

	status, walkErr := walkPages(runtime, walkOptions, func(pageToken string) (map[string]any, error) {
		params := clonePageParams(request.Params)
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		return runtime.CallAPITyped(request.Method, request.Path, params, request.Body)
	}, func(data map[string]any, pageNumber int) error {
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
	}, wait)
	meta.Pages = status.PagesFetched
	meta.NextToken = status.NextPageToken
	meta.Complete = status.StopReason == client.StopReasonExhausted ||
		status.StopReason == client.StopReasonStartPageToken
	if walkErr != nil {
		return meta, status, walkErr
	}
	return meta, status, nil
}

// resolvePaginationPolicy resolves the framework's standard list semantics.
// Even a one-page call is a pagination run; --page-all only changes its page
// budget and progress presentation.
func resolvePaginationPolicy(runtime *RuntimeContext, policy PageAllPolicy) (PageWalkOptions, error) {
	config, err := pageAllValues(runtime, policy)
	if err != nil {
		return PageWalkOptions{}, err
	}
	return PageWalkOptions{
		AutoPaginate:   config.enabled,
		PageLimit:      config.maxPages,
		AllowUnlimited: policy.AllowUnlimited,
		PageDelay:      config.delay,
		ShowProgress:   config.enabled && paginationProgressEnabled(runtime),
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
