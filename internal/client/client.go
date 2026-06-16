// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/errcompat"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
)

// RawApiRequest describes a raw API request.
type RawApiRequest struct {
	Method    string
	URL       string
	Params    map[string]interface{}
	Data      interface{}
	As        core.Identity
	ExtraOpts []larkcore.RequestOptionFunc // additional SDK request options (e.g. security headers)
}

// APIClient wraps lark.Client for all Lark Open API calls.
type APIClient struct {
	Config     *core.CliConfig
	SDK        *lark.Client // All Lark API calls go through SDK
	HTTP       *http.Client // Only for non-Lark API (OAuth, MCP, etc.)
	ErrOut     io.Writer    // debug/progress output
	Credential *credential.CredentialProvider
}

func (c *APIClient) resolveAccessToken(ctx context.Context, as core.Identity) (string, error) {
	result, err := c.Credential.ResolveToken(ctx, credential.NewTokenSpec(as, c.Config.AppID))
	if err != nil {
		var unavailableErr *credential.TokenUnavailableError
		if errors.As(err, &unavailableErr) {
			return "", newTokenMissingError(as, unavailableErr)
		}
		// NeedAuthorizationError from the credential chain (e.g. UAT refresh
		// returned need_user_authorization) must surface as typed
		// AuthenticationError. Without this, WrapDoAPIError would wrap the
		// raw err as NetworkError, and cmd/root.go's outer-typed gate would
		// then skip PromoteAuthError — leaving the user with exit 4 and no
		// auth-login hint instead of exit 3 typed authentication.
		var needAuthErr *internalauth.NeedAuthorizationError
		if errors.As(err, &needAuthErr) {
			return "", errcompat.PromoteAuthError(needAuthErr)
		}
		return "", err
	}
	if result.Token == "" {
		return "", newTokenMissingError(as, nil)
	}
	return result.Token, nil
}

// newTokenMissingError builds the typed *errs.AuthenticationError that
// resolveAccessToken returns when no usable token is available for the
// requested identity. cause is the underlying credential-chain error (or nil
// for the defensive empty-token branch) and is preserved for errors.Is /
// errors.Unwrap traversal without being serialized on the wire.
func newTokenMissingError(as core.Identity, cause error) error {
	return errs.NewAuthenticationError(errs.SubtypeTokenMissing,
		"no access token available for %s", as).
		WithHint("run: lark-cli auth login to re-authorize").
		WithCause(cause)
}

// buildApiReq converts a RawApiRequest into SDK types and collects
// request-specific options (ExtraOpts, URL-based headers).
// Auth is handled separately by DoSDKRequest.
func (c *APIClient) buildApiReq(request RawApiRequest) (*larkcore.ApiReq, []larkcore.RequestOptionFunc) {
	queryParams := make(larkcore.QueryParams)
	for k, v := range request.Params {
		switch val := v.(type) {
		case []string:
			queryParams[k] = val
		case []interface{}:
			for _, item := range val {
				queryParams.Add(k, fmt.Sprintf("%v", item))
			}
		default:
			queryParams.Set(k, fmt.Sprintf("%v", v))
		}
	}

	apiReq := &larkcore.ApiReq{
		HttpMethod:  strings.ToUpper(request.Method),
		ApiPath:     request.URL,
		Body:        request.Data,
		QueryParams: queryParams,
	}

	var opts []larkcore.RequestOptionFunc
	opts = append(opts, request.ExtraOpts...)
	return apiReq, opts
}

// DoSDKRequest resolves auth for the given identity and executes a pre-built SDK request.
// This is the shared auth+execute path used by both DoAPI (generic API calls via RawApiRequest)
// and shortcut RuntimeContext.DoAPI (direct larkcore.ApiReq calls).
//
// SDK Do() failures are normalised through WrapDoAPIError so every caller
// (cmd/api, RuntimeContext, shortcuts) gets the same wire shape without
// each one remembering to wrap. Today that wire shape is still the legacy
// *output.ExitError envelope (network / api_error); future framework-
// boundary migration flips WrapDoAPIError to typed *errs.NetworkError /
// *errs.InternalError per the contract in errs/ERROR_CONTRACT.md.
// Errors that arrive already-classified (legacy *output.ExitError from
// resolveAccessToken's missing-credential paths, or a typed *errs.*) flow
// through unchanged.
func (c *APIClient) DoSDKRequest(ctx context.Context, req *larkcore.ApiReq, as core.Identity, extraOpts ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error) {
	var opts []larkcore.RequestOptionFunc

	token, err := c.resolveAccessToken(ctx, as)
	if err != nil {
		// WrapDoAPIError is idempotent on already-classified errors:
		// the *output.ExitError that resolveAccessToken returns for missing
		// tokens (via output.ErrAuth) passes through with its auth category
		// and exit 3 intact, and any future typed *errs.* error from the
		// credential chain survives the same way. Only stray untyped errors
		// (raw fmt.Errorf) get the transport-or-internal fallback.
		return nil, WrapDoAPIError(err)
	}
	if as.IsBot() {
		req.SupportedAccessTokenTypes = []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}
		opts = append(opts, larkcore.WithTenantAccessToken(token))
	} else {
		req.SupportedAccessTokenTypes = []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}
		opts = append(opts, larkcore.WithUserAccessToken(token))
	}

	opts = append(opts, extraOpts...)
	resp, err := c.SDK.Do(ctx, req, opts...)
	if err != nil {
		return nil, WrapDoAPIError(err)
	}
	return resp, nil
}

// DoStream executes a streaming HTTP request against the Lark OpenAPI endpoint.
// Unlike DoSDKRequest (which buffers the full body via the SDK), DoStream returns
// a live *http.Response whose Body is an io.Reader for streaming consumption.
// Auth is resolved via Credential (same as DoSDKRequest). Security headers and
// any extra headers from opts are applied automatically.
// HTTP errors (status >= 400) are handled internally: the body is read (up to 4 KB),
// closed, and returned as an output.ErrNetwork — callers only receive successful responses.
func (c *APIClient) DoStream(ctx context.Context, req *larkcore.ApiReq, as core.Identity, opts ...Option) (*http.Response, error) {
	cfg := buildConfig(opts)

	// Resolve auth
	token, err := c.resolveAccessToken(ctx, as)
	if err != nil {
		// See DoSDKRequest comment on the same wrap pattern; the typed
		// auth-error pass-through plus untyped fallback applies equally to
		// streaming requests.
		return nil, WrapDoAPIError(err)
	}

	// Build URL
	requestURL, err := buildStreamURL(c.Config.Brand, req)
	if err != nil {
		return nil, err
	}

	// Build body
	bodyReader, contentType, err := buildStreamBody(req.Body)
	if err != nil {
		return nil, err
	}

	// Timeout — use context deadline only; httpClient.Timeout would cut off
	// healthy streaming responses because it includes body read time.
	httpClient := *c.HTTP
	httpClient.Timeout = 0
	cancel := func() {}
	requestCtx := ctx
	if cfg.timeout > 0 {
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			requestCtx, cancel = context.WithTimeout(ctx, cfg.timeout)
		}
	}

	// Build request
	httpReq, err := http.NewRequestWithContext(requestCtx, req.HttpMethod, requestURL, bodyReader)
	if err != nil {
		cancel()
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport, "stream request failed: %s", err).WithCause(err)
	}

	// Apply headers from opts
	for k, vs := range cfg.headers {
		for _, v := range vs {
			httpReq.Header.Add(k, v)
		}
	}

	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, errs.NewNetworkError(classifyNetworkSubtype(err), "stream request failed: %s", err).WithCause(err)
	}
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}

	// Handle HTTP errors internally
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(errBody))
		subtype := errs.SubtypeNetworkTransport
		if resp.StatusCode >= 500 {
			subtype = errs.SubtypeNetworkServer
		}
		var netErr *errs.NetworkError
		if msg != "" {
			netErr = errs.NewNetworkError(subtype, "HTTP %d: %s", resp.StatusCode, msg)
		} else {
			netErr = errs.NewNetworkError(subtype, "HTTP %d", resp.StatusCode)
		}
		netErr = netErr.WithCode(resp.StatusCode)
		if logID := streamLogID(resp.Header); logID != "" {
			netErr = netErr.WithLogID(logID)
		}
		return nil, netErr
	}

	return resp, nil
}

func streamLogID(header http.Header) string {
	logID := strings.TrimSpace(header.Get(larkcore.HttpHeaderKeyLogId))
	if logID == "" {
		logID = strings.TrimSpace(header.Get(larkcore.HttpHeaderKeyRequestId))
	}
	return logID
}

type cancelOnCloseBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseBody) Close() error {
	err := r.ReadCloser.Close()
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

func buildStreamURL(brand core.LarkBrand, req *larkcore.ApiReq) (string, error) {
	requestURL := req.ApiPath
	if !strings.HasPrefix(requestURL, "http://") && !strings.HasPrefix(requestURL, "https://") {
		var pathSegs []string
		for _, segment := range strings.Split(req.ApiPath, "/") {
			if !strings.HasPrefix(segment, ":") {
				pathSegs = append(pathSegs, segment)
				continue
			}
			pathKey := strings.TrimPrefix(segment, ":")
			pathValue, ok := req.PathParams[pathKey]
			if !ok {
				return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "missing path param %q for %s", pathKey, req.ApiPath).WithParam(pathKey)
			}
			if pathValue == "" {
				return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "empty path param %q for %s", pathKey, req.ApiPath).WithParam(pathKey)
			}
			pathSegs = append(pathSegs, url.PathEscape(pathValue))
		}
		endpoints := core.ResolveEndpoints(brand)
		requestURL = strings.TrimRight(endpoints.Open, "/") + strings.Join(pathSegs, "/")
	}
	if query := req.QueryParams.Encode(); query != "" {
		requestURL += "?" + query
	}
	return requestURL, nil
}

func buildStreamBody(body interface{}) (io.Reader, string, error) {
	switch typed := body.(type) {
	case nil:
		return nil, "", nil
	case io.Reader:
		return typed, "", nil
	case []byte:
		return bytes.NewReader(typed), "", nil
	case string:
		return strings.NewReader(typed), "text/plain; charset=utf-8", nil
	default:
		payload, err := json.Marshal(typed)
		if err != nil {
			return nil, "", errs.NewInternalError(errs.SubtypeSDKError, "failed to encode request body: %s", err).WithCause(err)
		}
		return bytes.NewReader(payload), "application/json", nil
	}
}

// DoAPI executes a raw Lark SDK request and returns the raw *larkcore.ApiResp.
// Unlike CallAPI which always JSON-decodes, DoAPI returns the raw response — suitable
// for file downloads (pass larkcore.WithFileDownload() via request.ExtraOpts) and
// any endpoint whose Content-Type may not be JSON.
func (c *APIClient) DoAPI(ctx context.Context, request RawApiRequest) (*larkcore.ApiResp, error) {
	apiReq, extraOpts := c.buildApiReq(request)
	return c.DoSDKRequest(ctx, apiReq, request.As, extraOpts...)
}

// CallAPI is a convenience wrapper: DoAPI + ParseJSONResponse. Use DoAPI
// directly when the response may not be JSON (e.g. file downloads).
//
// JSON parse failures are wrapped via WrapJSONResponseParseError so callers
// (notably the pagination loop and --page-all paths in cmd/api / cmd/service)
// see an *output.ExitError envelope (api_error for malformed JSON, network
// for everything else) instead of a bare fmt.Errorf — otherwise an empty
// or malformed page body would surface to the root handler as a plain-text
// "Error: ..." line and bypass the JSON stderr envelope contract.
func (c *APIClient) CallAPI(ctx context.Context, request RawApiRequest) (interface{}, error) {
	resp, err := c.DoAPI(ctx, request)
	if err != nil {
		return nil, err
	}
	result, parseErr := ParseJSONResponse(resp)
	if parseErr != nil {
		return nil, WrapJSONResponseParseError(parseErr, resp.RawBody)
	}
	return result, nil
}

// paginateLoop runs the core pagination loop. For each successful page (code == 0),
// it calls onResult if non-nil. It always accumulates and returns all raw page results.
func (c *APIClient) paginateLoop(ctx context.Context, request RawApiRequest, opts PaginationOptions, onResult func(interface{}) error) ([]interface{}, error) {
	var allResults []interface{}
	var pageToken string
	page := 0
	pageDelay := opts.PageDelay
	if pageDelay == 0 {
		pageDelay = 200
	}

	for {
		page++
		params := make(map[string]interface{})
		for k, v := range request.Params {
			params[k] = v
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}

		fmt.Fprintf(c.ErrOut, "[page %d] fetching...\n", page)
		result, err := c.CallAPI(ctx, RawApiRequest{
			Method:    request.Method,
			URL:       request.URL,
			Params:    params,
			Data:      request.Data,
			As:        request.As,
			ExtraOpts: request.ExtraOpts,
		})
		if err != nil {
			if page == 1 {
				return nil, err
			}
			fmt.Fprintf(c.ErrOut, "[page %d] error, stopping pagination\n", page)
			break
		}

		if resultMap, ok := result.(map[string]interface{}); ok {
			code, _ := util.ToFloat64(resultMap["code"])
			if code != 0 {
				allResults = append(allResults, result)
				if page == 1 {
					return allResults, nil
				}
				fmt.Fprintf(c.ErrOut, "[page %d] API error (code=%.0f), stopping pagination\n", page, code)
				break
			}
		}

		if onResult != nil {
			if err := onResult(result); err != nil {
				return allResults, err
			}
		}
		allResults = append(allResults, result)

		pageToken = ""
		if resultMap, ok := result.(map[string]interface{}); ok {
			if data, ok := resultMap["data"].(map[string]interface{}); ok {
				hasMore, _ := data["has_more"].(bool)
				if hasMore {
					if pt, ok := data["page_token"].(string); ok && pt != "" {
						pageToken = pt
					} else if pt, ok := data["next_page_token"].(string); ok && pt != "" {
						pageToken = pt
					}
				}
			}
		}

		if pageToken == "" {
			break
		}

		if opts.PageLimit > 0 && page >= opts.PageLimit {
			fmt.Fprintf(c.ErrOut, "[pagination] reached page limit (%d), stopping. Use --page-all --page-limit 0 to fetch all pages.\n", opts.PageLimit)
			break
		}

		if pageDelay > 0 {
			time.Sleep(time.Duration(pageDelay) * time.Millisecond)
		}
	}
	return allResults, nil
}

// PaginateAll fetches all pages and returns a single merged result.
// Use this for formats that need the complete dataset (e.g. JSON).
func (c *APIClient) PaginateAll(ctx context.Context, request RawApiRequest, opts PaginationOptions) (interface{}, error) {
	results, err := c.paginateLoop(ctx, request, opts, nil)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return map[string]interface{}{}, nil
	}
	if len(results) == 1 {
		return results[0], nil
	}
	return mergePagedResults(c.ErrOut, results), nil
}

// StreamPages fetches all pages and streams each page's list items via onItems.
// Returns the last page result (for error checking), whether any list items were found,
// and any network error. Use this for streaming formats (ndjson, table, csv).
func (c *APIClient) StreamPages(ctx context.Context, request RawApiRequest, onItems func([]interface{}) error, opts PaginationOptions) (result interface{}, hasItems bool, err error) {
	totalItems := 0
	results, loopErr := c.paginateLoop(ctx, request, opts, func(r interface{}) error {
		resultMap, ok := r.(map[string]interface{})
		if !ok {
			return nil
		}
		data, ok := resultMap["data"].(map[string]interface{})
		if !ok {
			return nil
		}
		arrayField := output.FindArrayField(data)
		if arrayField == "" {
			return nil
		}
		items, ok := data[arrayField].([]interface{})
		if !ok {
			return nil
		}
		totalItems += len(items)
		if err := onItems(items); err != nil {
			return err
		}
		hasItems = true
		return nil
	})
	if loopErr != nil {
		return nil, false, loopErr
	}

	if hasItems {
		fmt.Fprintf(c.ErrOut, "[pagination] streamed %d pages, %d total items\n", len(results), totalItems)
	}

	if len(results) > 0 {
		return results[len(results)-1], hasItems, nil
	}
	return map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}}, false, nil
}

// CheckResponse inspects a Lark API response for business-level errors (non-zero code)
// and routes the result through errclass.BuildAPIError so the wire envelope carries
// the canonical Category/Subtype + identity-aware extension fields (MissingScopes,
// ConsoleURL, etc.) for known Lark codes; unknown codes still surface as
// *errs.APIError{Subtype: unknown}.
func (c *APIClient) CheckResponse(result interface{}, identity core.Identity) error {
	resultMap, ok := result.(map[string]interface{})
	if !ok || resultMap == nil {
		return nil
	}
	if code, _ := util.ToFloat64(resultMap["code"]); code == 0 {
		return nil
	}
	cc := errclass.ClassifyContext{Identity: string(identity)}
	if c != nil && c.Config != nil {
		cc.Brand = string(c.Config.Brand)
		cc.AppID = c.Config.AppID
	}
	return errclass.BuildAPIError(resultMap, cc)
}
