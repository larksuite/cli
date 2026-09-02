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
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/ratelimit"
	"github.com/larksuite/cli/internal/recovery"
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

func (c *APIClient) resolveAccessToken(ctx context.Context, as core.Identity) (*credential.TokenResult, error) {
	result, err := c.Credential.ResolveToken(ctx, credential.NewTokenSpec(as, c.Config.AppID))
	if err != nil {
		var unavailableErr *credential.TokenUnavailableError
		if errors.As(err, &unavailableErr) {
			return nil, newTokenMissingError(as, unavailableErr)
		}
		// The credential chain already emits a typed *errs.AuthenticationError
		// for the missing-UAT case (e.g. UAT refresh returned
		// need_user_authorization), so it flows through unchanged: the
		// outer-typed gate in cmd/root.go and the idempotent WrapDoAPIError
		// both preserve its authentication category and exit 3.
		return nil, err
	}
	if result.Token == "" {
		return nil, newTokenMissingError(as, nil)
	}
	return result, nil
}

// newTokenMissingError builds the typed *errs.AuthenticationError that
// resolveAccessToken returns when no usable token is available for the
// requested identity. cause is the underlying credential-chain error (or nil
// for the defensive empty-token branch) and is preserved for errors.Is /
// errors.Unwrap traversal without being serialized on the wire.
func newTokenMissingError(as core.Identity, cause error) error {
	e := errs.NewAuthenticationError(errs.SubtypeTokenMissing,
		"no access token available for %s", as).
		WithCause(cause)
	if as == core.AsUser {
		return recovery.Attach(e, recovery.UserAuthorization())
	}
	return e.WithHint("configure valid app credentials for the bot identity")
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
// each one remembering to wrap. WrapDoAPIError classifies a raw transport
// failure into a typed *errs.NetworkError / *errs.InternalError per the
// contract in errs/ERROR_CONTRACT.md. Errors that arrive already-classified
// (a typed *errs.* from resolveAccessToken's missing-credential paths or
// elsewhere) flow through unchanged.
func (c *APIClient) DoSDKRequest(ctx context.Context, req *larkcore.ApiReq, as core.Identity, extraOpts ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error) {
	var opts []larkcore.RequestOptionFunc

	token, err := c.resolveAccessToken(ctx, as)
	if err != nil {
		// WrapDoAPIError is idempotent on already-classified errors:
		// the typed *errs.AuthenticationError that resolveAccessToken returns
		// for missing tokens passes through with its auth category and exit 3
		// intact, and any other typed *errs.* error from the credential chain
		// survives the same way. Only stray untyped errors (raw fmt.Errorf)
		// get the transport-or-internal fallback.
		return nil, WrapDoAPIError(err)
	}
	if as.IsBot() {
		req.SupportedAccessTokenTypes = []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant}
		opts = append(opts, larkcore.WithTenantAccessToken(token.Token))
	} else {
		req.SupportedAccessTokenTypes = []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser}
		opts = append(opts, larkcore.WithUserAccessToken(token.Token))
	}

	opts = append(opts, extraOpts...)
	requestCtx := core.WithCredentialSource(ctx, token.Source)
	resp, err := c.SDK.Do(requestCtx, req, opts...)
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
// closed, and returned as a typed error — callers only receive successful responses.
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
	requestCtx := core.WithCredentialSource(ctx, token.Source)
	if cfg.timeout > 0 {
		if _, hasDeadline := requestCtx.Deadline(); !hasDeadline {
			requestCtx, cancel = context.WithTimeout(requestCtx, cfg.timeout)
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
	httpReq.Header.Set("Authorization", "Bearer "+token.Token)

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, wrapTransportError(ctx, err, cfg.replaySafe, "stream request failed: %s", err)
	}
	resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}

	// Handle HTTP errors internally
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		msg := strings.TrimSpace(string(errBody))
		if cfg.replaySafe && resp.StatusCode == http.StatusTooManyRequests {
			rate := ratelimit.ParseHeaders(resp.Header, time.Now())
			retryAfterSeconds := rate.RetryAfterSeconds()
			if msg == "" {
				msg = "OpenAPI gateway rate limit exceeded"
			}
			apiErr := errs.NewAPIError(errs.SubtypeRateLimit, "HTTP %d: %s", resp.StatusCode, msg).
				WithCode(resp.StatusCode).
				WithRetryable().
				WithRetryAfterSeconds(retryAfterSeconds)
			hint := "retry with exponential backoff and jitter"
			if retryAfterSeconds > 0 {
				hint = fmt.Sprintf("wait at least %d seconds before retrying; use exponential backoff with jitter if throttling continues", retryAfterSeconds)
			}
			if rate.Limit > 0 {
				hint += fmt.Sprintf("; gateway request-window quota is %d", rate.Limit)
			}
			apiErr.WithHint("%s", hint)
			if logID := streamLogID(resp.Header); logID != "" {
				apiErr.WithLogID(logID)
			}
			return nil, apiErr
		}
		subtype := errs.SubtypeNetworkTransport
		if cfg.replaySafe && resp.StatusCode == http.StatusRequestTimeout {
			subtype = errs.SubtypeNetworkTimeout
		} else if resp.StatusCode >= 500 {
			subtype = errs.SubtypeNetworkServer
		}
		var netErr *errs.NetworkError
		if msg != "" {
			netErr = errs.NewNetworkError(subtype, "HTTP %d: %s", resp.StatusCode, msg)
		} else {
			netErr = errs.NewNetworkError(subtype, "HTTP %d", resp.StatusCode)
		}
		netErr = netErr.WithCode(resp.StatusCode)
		if cfg.replaySafe && (resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= http.StatusInternalServerError) {
			rate := ratelimit.ParseHeaders(resp.Header, time.Now())
			netErr = netErr.WithRetryable().WithRetryAfterSeconds(rate.RetryAfterSeconds())
		}
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
// see a typed *errs.InternalError (invalid_response) instead of a bare
// fmt.Errorf — otherwise an empty or malformed page body would surface to the
// root handler as a plain-text "Error: ..." line and bypass the JSON stderr
// envelope contract.
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

// callPage is CallAPI plus the raw response. CallAPI drops the
// *larkcore.ApiResp, which is why the pagination loop could not tell a 4xx/5xx
// page from a successful one — nothing below it reads the HTTP status.
//
// A body that fails to parse is classified by status when the status is a
// failure, as HandleResponse does (response.go), or --page-all would exit 5
// where plain `api` exits 4; on a success status the parse error stands. What
// reaches that branch is a body the SDK did not decode itself: one with no
// Content-Type, or one typed text/json, which IsJSONContentType accepts but the
// SDK does not. A body typed application/json that does not parse fails inside
// DoAPI instead, with no response left to read a status from, and a body that
// declares a non-JSON type never gets as far as the parser.
func (c *APIClient) callPage(ctx context.Context, request RawApiRequest) (interface{}, *larkcore.ApiResp, error) {
	resp, err := c.DoAPI(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	// Whether the body is JSON is decided by the rule HandleResponse uses, so
	// both paths look at the same bytes the same way. A declared non-JSON body
	// is never parsed: failed, it is classified by status; succeeded, plain `api`
	// treats it as a download, which a paginated run has no way to be.
	if ct := resp.Header.Get("Content-Type"); !isJSONBody(ct) {
		if resp.StatusCode >= 400 {
			return nil, resp, httpStatusError(resp.StatusCode, resp.RawBody)
		}
		return nil, resp, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"response is not JSON (Content-Type %q); --page-all needs a JSON list response", ct)
	}
	result, parseErr := ParseJSONResponse(resp)
	if parseErr != nil {
		if resp.StatusCode >= 400 {
			return nil, resp, httpStatusError(resp.StatusCode, resp.RawBody)
		}
		return nil, resp, WrapJSONResponseParseError(parseErr, resp.RawBody)
	}
	return result, resp, nil
}

// pageDeclaresSuccess reports whether a page carries a business code that is
// exactly 0. It decides one thing — whether the loop may treat the page as a
// step in the pagination: follow its cursor, or accept it as the continuation
// an earlier page promised — and is deliberately narrower than CheckResponse's
// notion of "not an error", which is what decides whether the page is output:
// that one reads the code as a float, under which 0.5, 1e-324 and an absent
// field all become 0. Here a JSON number is zero only if every digit of its
// mantissa is 0 — 0, -0, 0.0, 0e10 — which is exact, allocation-free, and
// indifferent to how large an exponent a hostile body attaches.
func pageDeclaresSuccess(result interface{}) bool {
	m, ok := result.(map[string]interface{})
	if !ok {
		return false
	}
	switch n := m["code"].(type) {
	case int:
		return n == 0
	case int64:
		return n == 0
	case float64:
		// Reachable only from a caller that decoded without UseNumber;
		// ParseJSONResponse never produces this.
		return n == 0
	case json.Number:
		return isZeroLiteral(string(n))
	}
	return false
}

func isZeroLiteral(s string) bool {
	s = strings.TrimPrefix(s, "-")
	if e := strings.IndexAny(s, "eE"); e >= 0 {
		s = s[:e]
	}
	sawDigit := false
	for _, c := range s {
		switch c {
		case '0':
			sawDigit = true
		case '.':
		default:
			return false
		}
	}
	return sawDigit
}

// nextPageToken is the continuation token a page carries: its page_token or
// next_page_token when has_more is true, or "" when it carries none.
func nextPageToken(result interface{}) string {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return ""
	}
	data, ok := resultMap["data"].(map[string]interface{})
	if !ok {
		return ""
	}
	if hasMore, _ := data["has_more"].(bool); !hasMore {
		return ""
	}
	if pt, ok := data["page_token"].(string); ok && pt != "" {
		return pt
	}
	if pt, ok := data["next_page_token"].(string); ok && pt != "" {
		return pt
	}
	return ""
}

func firstPageRecoveryHint(method string) string {
	if strings.EqualFold(method, http.MethodGet) || strings.EqualFold(method, http.MethodHead) {
		return "remove `--page-all` and `--jq`; use `--output <path>` to save the raw response"
	}
	// The generic api command does not confirmation-gate every write. Do not
	// turn the automatic replay protection below into instructions for a human
	// or agent to replay a POST, PUT, PATCH, or DELETE manually.
	return "verify whether the first request changed remote state before retrying it"
}

// paginateLoop runs the core pagination loop. Each page accepted for output
// calls onResult if non-nil and is accumulated after that callback succeeds.
// Acceptance requires a business code of exactly 0, except that a first page
// which passes classification and carries no continuation token is still output
// for plain-api compatibility.
// A first-page business error is handed back whole with a nil error for the
// command layer to classify; any later failure returns the pages accumulated so
// far together with the error.
func (c *APIClient) paginateLoop(ctx context.Context, request RawApiRequest, opts PaginationOptions, onResult func(interface{}) error) ([]interface{}, error) {
	var allResults []interface{}
	var pageToken string
	page := 0
	pageDelay := opts.PageDelay
	if pageDelay == 0 {
		pageDelay = 200
	}

	// This loop owns the CheckResponse call that classifies a later page's
	// failure, so it resolves the identity that classification uses rather than
	// trusting every caller to fill opts in. An empty identity is not neutral:
	// errclass rewrites it to "user" (internal/errclass/classify.go) and hands a bot the
	// user-facing recovery text. The request carries the identity it was sent
	// with, so prefer it over guessing; AsAuto means "not resolved yet" and is
	// treated as unset at both steps.
	identity := opts.Identity
	if identity == "" || identity == core.AsAuto {
		identity = request.As
	}
	if identity == "" || identity == core.AsAuto {
		identity = core.AsUser
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
		result, resp, err := c.callPage(ctx, RawApiRequest{
			Method:    request.Method,
			URL:       request.URL,
			Params:    params,
			Data:      request.Data,
			As:        request.As,
			ExtraOpts: request.ExtraOpts,
		})
		if err != nil {
			// Page 1 has nothing accumulated yet, so both paths return the same
			// (nil, err); only the progress line differs. A later page must not
			// fall through to the loop's `return allResults, nil` — that is what
			// turned a mid-pagination failure into a successful partial result.
			if page > 1 {
				fmt.Fprintf(c.ErrOut, "[page %d] error, stopping pagination\n", page)
			}
			return allResults, err
		}

		// A failed page is classified exactly as HandleResponse classifies a plain
		// `api` response, in the same order — the business code through
		// CheckResponse, then the HTTP status — so one response cannot exit two
		// ways depending on a flag. A business error on page 1 is handed back
		// whole with a nil error so the command layer can classify it and dump
		// the raw response, the long-standing output contract; on a later page it
		// fails the run. An HTTP failure fails the run on any page, as plain `api`
		// fails it too. Whether a page that passes both checks is a step in the
		// pagination at all is decided below.
		if apiErr := c.CheckResponse(result, identity); apiErr != nil {
			if page == 1 {
				return append(allResults, result), nil
			}
			fmt.Fprintf(c.ErrOut, "[page %d] API error, stopping pagination\n", page)
			return allResults, apiErr
		}
		if resp.StatusCode >= 400 {
			if page > 1 {
				fmt.Fprintf(c.ErrOut, "[page %d] HTTP %d, stopping pagination\n", page, resp.StatusCode)
			}
			return allResults, httpStatusError(resp.StatusCode, resp.RawBody)
		}

		// A page that passed both checks is a response the classifier accepts. It
		// is output on the existing contract — unless the run must not treat it as
		// a step in the pagination, which is decided by whether it declared
		// success: a business code that is exactly 0. CheckResponse cannot make
		// that call, it reads the code as a float, under which 0.5, 1e-324 and an
		// absent field all become 0. The rule is asymmetric on purpose:
		//   - a later page that did not declare success fails the run before it is
		//     emitted or accumulated. The page before it promised more data, and
		//     accepting this one — with or without a cursor of its own — would end
		//     the run as complete over that promise, which is #2477.
		//   - a first page that did not declare success but carries a continuation
		//     token is refused before output. `api --page-all` is not restricted to
		//     GET, and re-issuing the request for its cursor would replay a POST or a
		//     DELETE against a page the loop could not read. Refusing with an
		//     error, rather than outputting the page and stopping, is what gives
		//     the streaming formats — which have no envelope to carry has_more — a
		//     machine-readable signal that the run did not complete.
		//   - a first page that did not declare success and carries no continuation
		//     token is output on the existing contract: there is nothing to
		//     paginate, and plain `api` would have shown the same response.
		token := nextPageToken(result)
		if !pageDeclaresSuccess(result) && (page > 1 || token != "") {
			if page > 1 {
				fmt.Fprintf(c.ErrOut, "[page %d] response did not declare success (code is missing, unreadable, or not exactly zero), stopping pagination\n", page)
				return allResults, errs.NewInternalError(errs.SubtypeInvalidResponse,
					"page %d of a --page-all run did not declare success (code is missing, unreadable, or not exactly zero)", page)
			}
			return allResults, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"the first page carries a continuation token but did not declare success (code is missing, unreadable, or not exactly zero)").
				WithHint("%s", firstPageRecoveryHint(request.Method))
		}

		if onResult != nil {
			if err := onResult(result); err != nil {
				return allResults, err
			}
		}
		allResults = append(allResults, result)

		pageToken = token
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
// and the error that ended the run — transport, HTTP status, business code, or a page
// that did not declare success. Use this for streaming formats (ndjson, table, csv).
func (c *APIClient) StreamPages(ctx context.Context, request RawApiRequest, onItems func([]interface{}) error, opts PaginationOptions) (result interface{}, hasItems bool, err error) {
	totalItems := 0
	emittedPages := 0
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
		// Counted only after onItems accepts them. onItems is what actually
		// writes the page out, and it can refuse — counting first would let the
		// failure summary claim items that never reached stdout, in precisely
		// the run where the caller is relying on that number.
		if err := onItems(items); err != nil {
			return err
		}
		totalItems += len(items)
		emittedPages++
		hasItems = true
		return nil
	})
	if loopErr != nil {
		// Streaming formats have already written the pages that succeeded, so the
		// exit code says the run is incomplete and this line says how far it got.
		// Worded apart from the success summary so the two are never confused.
		//
		// emittedPages, not len(results): a page whose data carries no array
		// field is fetched and accumulated but never offered to onItems, so it
		// put nothing on stdout and must not be counted here.
		if hasItems {
			fmt.Fprintf(c.ErrOut, "[pagination] streamed %d pages, %d total items before the run failed\n", emittedPages, totalItems)
		}
		return nil, false, loopErr
	}

	// Deliberately still len(results): this line predates this change and reads
	// as "the run traversed N pages", which is true. Switching it to
	// emittedPages would alter established output outside the scope of the
	// failure semantics being fixed here. Unifying the two is a separate call.
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
