// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"io"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
)

// StopReason describes the neutral fact that stopped a pagination attempt.
// Business domains decide whether a given reason means success or failure.
type StopReason string

const (
	StopReasonExhausted        StopReason = "exhausted"
	StopReasonSinglePage       StopReason = "single_page"
	StopReasonPageLimit        StopReason = "page_limit"
	StopReasonStartPageToken   StopReason = "start_page_token"
	StopReasonTransportError   StopReason = "transport_error"
	StopReasonAPIError         StopReason = "api_error"
	StopReasonMissingToken     StopReason = "missing_token"
	StopReasonRepeatedToken    StopReason = "repeated_token"
	StopReasonServerTruncation StopReason = "server_truncation"
)

// PaginationStatus contains pagination facts without interpreting completeness.
// Cause is process-local diagnostic context and must never be serialized.
type PaginationStatus struct {
	PagesFetched  int        `json:"pages_fetched,omitempty"`
	HasMore       bool       `json:"has_more,omitempty"`
	NextPageToken string     `json:"next_page_token,omitempty"`
	StopReason    StopReason `json:"stop_reason,omitempty"`
	Cause         error      `json:"-"`
}

// InspectPaginationPage derives status from one already-fetched page.
// It is useful for callers that intentionally perform a single-page read.
func InspectPaginationPage(result interface{}, startPageToken string) (PaginationStatus, error) {
	status := PaginationStatus{PagesFetched: 1}
	hasMore, nextToken, truncated := paginationFacts(result)
	status.HasMore = hasMore
	status.NextPageToken = nextToken

	if truncated {
		status.StopReason = StopReasonServerTruncation
		return status, nil
	}
	if hasMore && nextToken == "" {
		err := missingPaginationTokenError()
		status.StopReason = StopReasonMissingToken
		status.Cause = err
		return status, err
	}
	if hasMore && startPageToken != "" && nextToken == startPageToken {
		err := repeatedPaginationTokenError()
		status.StopReason = StopReasonRepeatedToken
		status.Cause = err
		return status, err
	}
	if startPageToken != "" {
		status.StopReason = StopReasonStartPageToken
		return status, nil
	}
	if hasMore {
		status.StopReason = StopReasonSinglePage
		return status, nil
	}
	status.StopReason = StopReasonExhausted
	return status, nil
}

// PaginateAllWithStatus fetches pages until a neutral stop condition occurs.
// Unlike PaginateAll, later failures are returned together with already-fetched
// data so an opt-in caller can report an incomplete result without losing it.
func (c *APIClient) PaginateAllWithStatus(
	ctx context.Context,
	request *RawApiRequest,
	opts PaginationOptions,
) (map[string]interface{}, PaginationStatus, error) {
	results, status, err := c.paginateLoopWithStatus(ctx, request, opts, nil)
	return mergeStatusResults(io.Discard, results), status, err
}

// StreamPagesWithStatus emits each successful raw page and returns the neutral
// stop status. A later failure does not retract pages already emitted.
func (c *APIClient) StreamPagesWithStatus(
	ctx context.Context,
	request *RawApiRequest,
	opts PaginationOptions,
	emit func(page map[string]interface{}) error,
) (PaginationStatus, error) {
	_, status, err := c.paginateLoopWithStatus(ctx, request, opts, emit)
	return status, err
}

func (c *APIClient) paginateLoopWithStatus(
	ctx context.Context,
	request *RawApiRequest,
	opts PaginationOptions,
	emit func(page map[string]interface{}) error,
) ([]interface{}, PaginationStatus, error) {
	if request == nil {
		err := errs.NewInternalError(errs.SubtypeInvalidResponse, "pagination request is nil")
		return nil, PaginationStatus{Cause: err}, err
	}

	var results []interface{}
	status := PaginationStatus{}
	nextToken := stringParam(request.Params, "page_token")
	startPageToken := nextToken
	seenTokens := make(map[string]struct{})
	if nextToken != "" {
		seenTokens[nextToken] = struct{}{}
	}

	pageDelay := opts.PageDelay
	if pageDelay == 0 {
		pageDelay = 200
	}

	for {
		params := cloneParams(request.Params)
		if nextToken != "" {
			params["page_token"] = nextToken
		}

		resp, err := c.DoAPI(ctx, RawApiRequest{
			Method:    request.Method,
			URL:       request.URL,
			Params:    params,
			Data:      request.Data,
			As:        request.As,
			ExtraOpts: request.ExtraOpts,
		})
		if err != nil {
			status.StopReason = StopReasonTransportError
			status.Cause = err
			status.HasMore = nextToken != ""
			status.NextPageToken = nextToken
			return results, status, err
		}
		result, err := ParseJSONResponse(resp)
		if err != nil {
			err = WrapJSONResponseParseError(err, resp.RawBody)
			if opts.NormalizeHTTPError != nil && resp.StatusCode >= 400 {
				err = opts.NormalizeHTTPError(resp.StatusCode, streamLogID(resp.Header), err)
			}
			status.StopReason = StopReasonTransportError
			status.Cause = err
			status.HasMore = nextToken != ""
			status.NextPageToken = nextToken
			return results, status, err
		}
		identity := opts.Identity
		if identity == "" {
			identity = request.As
		}
		if identity == "" {
			identity = core.AsUser
		}
		apiErr := c.CheckResponse(result, identity)
		if opts.NormalizeHTTPError != nil && resp.StatusCode >= 400 {
			apiErr = opts.NormalizeHTTPError(resp.StatusCode, streamLogID(resp.Header), apiErr)
		}
		if apiErr != nil {
			status.StopReason = StopReasonAPIError
			status.Cause = apiErr
			status.HasMore = nextToken != ""
			status.NextPageToken = nextToken
			return results, status, apiErr
		}

		page, ok := result.(map[string]interface{})
		if !ok {
			err := errs.NewInternalError(errs.SubtypeInvalidResponse, "pagination response must be a JSON object")
			status.StopReason = StopReasonAPIError
			status.Cause = err
			return results, status, err
		}

		results = append(results, result)
		status.PagesFetched++
		if emit != nil {
			if err := emit(page); err != nil {
				status.Cause = err
				return results, status, err
			}
		}

		hasMore, returnedToken, truncated := paginationFacts(result)
		status.HasMore = hasMore
		status.NextPageToken = returnedToken
		if truncated {
			status.StopReason = StopReasonServerTruncation
			return results, status, nil
		}
		if !hasMore {
			if startPageToken != "" {
				status.StopReason = StopReasonStartPageToken
			} else {
				status.StopReason = StopReasonExhausted
			}
			status.NextPageToken = ""
			return results, status, nil
		}
		if returnedToken == "" {
			err := missingPaginationTokenError()
			status.StopReason = StopReasonMissingToken
			status.Cause = err
			return results, status, err
		}
		if _, exists := seenTokens[returnedToken]; exists {
			err := repeatedPaginationTokenError()
			status.StopReason = StopReasonRepeatedToken
			status.Cause = err
			return results, status, err
		}
		if opts.PageLimit > 0 && status.PagesFetched >= opts.PageLimit {
			status.StopReason = StopReasonPageLimit
			return results, status, nil
		}

		seenTokens[returnedToken] = struct{}{}
		nextToken = returnedToken
		if pageDelay > 0 {
			time.Sleep(time.Duration(pageDelay) * time.Millisecond)
		}
	}
}

func paginationFacts(result interface{}) (hasMore bool, nextToken string, truncated bool) {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return false, "", false
	}
	truncated = explicitTruncation(resultMap)
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		return false, "", truncated
	}
	hasMore, _ = data["has_more"].(bool)
	nextToken = stringParam(data, "page_token")
	if nextToken == "" {
		nextToken = stringParam(data, "next_page_token")
	}
	return hasMore, nextToken, truncated || explicitTruncation(data)
}

func explicitTruncation(object map[string]interface{}) bool {
	truncated, _ := object["truncated"].(bool)
	isTruncated, _ := object["is_truncated"].(bool)
	return truncated || isTruncated
}

func stringParam(params map[string]interface{}, name string) string {
	value, _ := params[name].(string)
	return value
}

func cloneParams(params map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(params)+1)
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func missingPaginationTokenError() error {
	return errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"paginated response has_more=true but next page token is missing",
	)
}

func repeatedPaginationTokenError() error {
	return errs.NewInternalError(
		errs.SubtypeInvalidResponse,
		"paginated response repeated the same next page token",
	)
}

func mergeStatusResults(w io.Writer, results []interface{}) map[string]interface{} {
	if len(results) == 0 {
		return map[string]interface{}{}
	}
	if len(results) == 1 {
		if result, ok := results[0].(map[string]interface{}); ok {
			return result
		}
		return map[string]interface{}{"pages": results}
	}
	if w == nil {
		w = io.Discard
	}
	merged := mergePagedResults(w, results)
	if result, ok := merged.(map[string]interface{}); ok {
		return result
	}
	return map[string]interface{}{"pages": results}
}
