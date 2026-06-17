// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/google/uuid"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/i18n"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// RuntimeContext provides helpers for shortcut execution.
type RuntimeContext struct {
	ctx           context.Context // from cmd.Context(), propagated through the call chain
	Config        *core.CliConfig
	Cmd           *cobra.Command
	Format        string
	JqExpr        string                            // --jq expression; empty = no filter
	outputErrOnce sync.Once                         // guards first-error capture in Out()/OutFormat()
	outputErr     error                             // deferred error from jq filtering; written at most once
	botOnly       bool                              // set by framework for bot-only shortcuts
	resolvedAs    core.Identity                     // effective identity resolved by framework
	Factory       *cmdutil.Factory                  // injected by framework
	apiClientFunc func() (*client.APIClient, error) // sync.OnceValues; initialized in newRuntimeContext
	botInfoFunc   func() (*BotInfo, error)          // sync.OnceValues; lazy bot identity from /bot/v3/info
	larkSDK       *lark.Client                      // eagerly initialized in mountDeclarative
}

// ── Identity ──

// As returns the current identity.
// For bot-only shortcuts, always returns AsBot.
// For dual-auth shortcuts, uses the resolved identity (respects default-as config).
func (ctx *RuntimeContext) As() core.Identity {
	if ctx.botOnly {
		return core.AsBot
	}
	if ctx.resolvedAs.IsBot() {
		return core.AsBot
	}
	if ctx.resolvedAs != "" {
		return ctx.resolvedAs
	}
	return core.AsUser
}

// IsBot returns true if current identity is bot.
func (ctx *RuntimeContext) IsBot() bool {
	return ctx.As().IsBot()
}

// Command returns the shortcut command name as cobra knows it (e.g.
// "+pivot-create"). Used by per-service helpers (e.g. sheets schema
// validation) that key off the shortcut identity.
func (ctx *RuntimeContext) Command() string {
	if ctx.Cmd == nil {
		return ""
	}
	return ctx.Cmd.Name()
}

// UserOpenId returns the current user's open_id from config.
func (ctx *RuntimeContext) UserOpenId() string { return ctx.Config.UserOpenId }

// Lang returns the user's preference as a canonical locale, or "" if unset or
// unrecognized; callers choose their own fallback.
func (ctx *RuntimeContext) Lang() i18n.Lang {
	lang, _ := i18n.Parse(string(ctx.Config.Lang))
	return lang
}

// BotInfo holds bot identity metadata fetched lazily from /bot/v3/info.
type BotInfo struct {
	OpenID  string
	AppName string
}

// BotInfo returns the bot's open_id and display name, fetched lazily from /bot/v3/info.
// Unlike UserOpenId() (which reads from config), this requires a network call and may fail.
// Thread-safe via sync.OnceValues; the API is called at most once per RuntimeContext.
func (ctx *RuntimeContext) BotInfo() (*BotInfo, error) {
	if ctx.botInfoFunc == nil {
		return nil, fmt.Errorf("BotInfo not available (runtime context not fully initialized)")
	}
	return ctx.botInfoFunc()
}

// fetchBotInfo calls /bot/v3/info using bot identity and parses the response.
func (ctx *RuntimeContext) fetchBotInfo() (*BotInfo, error) {
	if !ctx.Config.CanBot() {
		return nil, fmt.Errorf("fetch bot info: bot identity is not available in current credential context")
	}
	resp, err := ctx.DoAPIAsBot(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    "/open-apis/bot/v3/info",
	})
	if err != nil {
		return nil, fmt.Errorf("fetch bot info: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch bot info: HTTP %d", resp.StatusCode)
	}
	// /open-apis/bot/v3/info returns `{code, msg, bot: {...}}` — the bot
	// payload is under "bot", not "data" as the newer Lark API convention.
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"bot"`
	}
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
		return nil, fmt.Errorf("fetch bot info: unmarshal: %w", err)
	}
	if envelope.Code != 0 {
		return nil, fmt.Errorf("fetch bot info: [%d] %s", envelope.Code, envelope.Msg)
	}
	if envelope.Data.OpenID == "" {
		return nil, fmt.Errorf("fetch bot info: open_id is empty")
	}
	return &BotInfo{OpenID: envelope.Data.OpenID, AppName: envelope.Data.AppName}, nil
}

// Ctx returns the context.Context propagated from cmd.Context().
func (ctx *RuntimeContext) Ctx() context.Context { return ctx.ctx }

// getAPIClient returns the cached APIClient, creating it on first use.
// Thread-safe via sync.OnceValues (initialized in newRuntimeContext).
// Falls back to direct construction for test contexts that bypass newRuntimeContext.
func (ctx *RuntimeContext) getAPIClient() (*client.APIClient, error) {
	if ctx.apiClientFunc != nil {
		return ctx.apiClientFunc()
	}
	return ctx.Factory.NewAPIClientWithConfig(ctx.Config)
}

// AccessToken returns a valid access token for the current identity.
// For user: returns user access token (with auto-refresh).
// For bot: returns tenant access token.
func (ctx *RuntimeContext) AccessToken() (string, error) {
	result, err := ctx.Factory.Credential.ResolveToken(ctx.ctx, credential.NewTokenSpec(ctx.As(), ctx.Config.AppID))
	if err != nil {
		return "", output.ErrAuth("failed to get access token: %s", err)
	}
	if result == nil || result.Token == "" {
		return "", output.ErrAuth("no access token available for %s", ctx.As())
	}
	return result.Token, nil
}

// LarkSDK returns the eagerly-initialized Lark SDK client.
func (ctx *RuntimeContext) LarkSDK() *lark.Client {
	return ctx.larkSDK
}

// EnsureScopes runs the same pre-flight scope check used by the framework
// before Validate, but on a caller-supplied set of scopes. Use it from a
// shortcut's Validate to enforce conditional scope requirements that depend
// on flag values (e.g. --delete-remote needing space:document:delete) so a
// destructive operation never starts on a token that can't finish it.
//
// Behavior matches checkShortcutScopes: when no token is available or the
// resolver doesn't expose scope metadata, this is a silent no-op — the
// downstream API call still surfaces missing_scope at runtime.
func (ctx *RuntimeContext) EnsureScopes(scopes []string) error {
	return checkShortcutScopes(ctx.Factory, ctx.ctx, ctx.As(), ctx.Config, scopes)
}

// ── Flag accessors ──

// Str returns a string flag value.
func (ctx *RuntimeContext) Str(name string) string {
	v, _ := ctx.Cmd.Flags().GetString(name)
	return v
}

// Bool returns a bool flag value.
func (ctx *RuntimeContext) Bool(name string) bool {
	v, _ := ctx.Cmd.Flags().GetBool(name)
	return v
}

// Int returns an int flag value.
func (ctx *RuntimeContext) Int(name string) int {
	v, _ := ctx.Cmd.Flags().GetInt(name)
	return v
}

// Float64 returns a float64 flag value (non-integer numbers).
func (ctx *RuntimeContext) Float64(name string) float64 {
	v, _ := ctx.Cmd.Flags().GetFloat64(name)
	return v
}

// StrArray returns a string-array flag value (repeated flag, no CSV splitting).
func (ctx *RuntimeContext) StrArray(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringArray(name)
	return v
}

// StrSlice returns a string-slice flag value (supports CSV splitting and repeated flags).
func (ctx *RuntimeContext) StrSlice(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringSlice(name)
	return v
}

// Changed reports whether the user explicitly set the named flag on the
// command line, as opposed to the flag carrying its default value.
func (ctx *RuntimeContext) Changed(name string) bool {
	f := ctx.Cmd.Flags().Lookup(name)
	if f == nil {
		return false
	}
	return f.Changed
}

// ── API helpers ──

//	CallAPI uses an internal HTTP wrapper with limited control over request/response.
//
// Prefer DoAPI for new code — it calls the Lark SDK directly and supports file upload/download options.
//
// CallAPI calls the Lark API using the current identity (ctx.As()) and auto-handles errors.
func (ctx *RuntimeContext) CallAPI(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	result, err := ctx.callRaw(method, url, params, data)
	return HandleApiResult(result, err, "API call failed")
}

// CallAPITyped is the typed-only replacement for CallAPI: it performs the same
// SDK request (buildRequest → APIClient.DoAPI → DoSDKRequest, identical
// transport and query model to CallAPI) and returns the "data" object, but
// classifies failures into typed errs.* errors via errclass.BuildAPIError.
//
// A transport / auth error from the client boundary is already typed and passes
// through unchanged; a non-zero API response code is classified into a typed
// error carrying subtype / code / log_id. Unlike CallAPI it never emits a legacy
// output.ExitError envelope, and never downgrades a typed network/auth error.
//
// It lifts x-tt-logid from the response header (which the body-only parse drops)
// so log_id surfaces on the typed error even when the server returns it only in
// the header.
func (ctx *RuntimeContext) CallAPITyped(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, typedOrInternal(err)
	}
	resp, err := ac.DoAPI(ctx.ctx, ctx.buildRequest(method, url, params, data))
	if err != nil {
		return nil, typedOrInternal(err)
	}
	return ctx.ClassifyAPIResponse(resp)
}

// ClassifyAPIResponse turns a raw *larkcore.ApiResp into the "data" object or a
// typed errs.* error. It is the shared response classifier for typed API paths
// — used by CallAPITyped and by callers that drive the request themselves
// (e.g. file upload via DoAPI). It:
//
//  1. parses the JSON body; an unparseable body on an HTTP error status (a
//     gateway 5xx text/html page, an empty body, a missing Content-Type) is
//     classified by status — 5xx → retryable network/server_error, 404 →
//     not_found, other 4xx → api error — not a misleading invalid-response
//     internal error;
//  2. rejects a top-level non-object JSON ([], null, scalar) as an
//     invalid-response internal error — never a silent success ack;
//  3. lifts x-tt-logid from the response header onto the typed error so log_id
//     surfaces even when the body omits it;
//  4. classifies a non-zero API code via errclass.BuildAPIError, and treats any
//     HTTP error status that parsed to code==0 as a status error.
//
// The success "data" object is returned untouched. On a non-zero API code the
// data is returned alongside the typed error, since the response can still
// carry fields a caller needs on failure (e.g. the file_token an overwrite
// returned, for token-stability handling).
func (ctx *RuntimeContext) ClassifyAPIResponse(resp *larkcore.ApiResp) (map[string]interface{}, error) {
	return ClassifyAPIResponseWith(resp, ctx.APIClassifyContext())
}

// ClassifyAPIResponseWith is the RuntimeContext-free form of
// ClassifyAPIResponse for callers that drive the request outside a running
// shortcut (e.g. a cobra command holding only a factory) and supply their own
// classification context.
func ClassifyAPIResponseWith(resp *larkcore.ApiResp, cc errclass.ClassifyContext) (map[string]interface{}, error) {
	logID, _ := logIDFromHeader(resp)["log_id"].(string)

	result, parseErr := client.ParseJSONResponse(resp)
	if parseErr != nil {
		if resp.StatusCode >= 400 {
			return nil, httpStatusError(resp.StatusCode, resp.RawBody, logID)
		}
		return nil, client.WrapJSONResponseParseError(parseErr, resp.RawBody)
	}
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		e := errs.NewInternalError(errs.SubtypeInvalidResponse, "API returned a non-object JSON response")
		if logID != "" {
			e = e.WithLogID(logID)
		}
		return nil, e
	}
	if logID != "" {
		if _, present := resultMap["log_id"]; !present {
			resultMap["log_id"] = logID
		}
	}
	out, _ := resultMap["data"].(map[string]interface{})
	if apiErr := errclass.BuildAPIError(resultMap, cc); apiErr != nil {
		return out, apiErr
	}
	if resp.StatusCode >= 400 {
		return out, httpStatusError(resp.StatusCode, resp.RawBody, logID)
	}
	return out, nil
}

// httpStatusError classifies an HTTP error status whose body is not a usable
// API envelope: 5xx → retryable network/server_error, 404 → not_found, other
// 4xx → api error. The x-tt-logid (when present) is attached for diagnosis.
func httpStatusError(status int, rawBody []byte, logID string) error {
	body := TruncateStr(strings.TrimSpace(string(rawBody)), 500)
	if status >= 500 {
		e := errs.NewNetworkError(errs.SubtypeNetworkServer, "HTTP %d: %s", status, body).WithCode(status).WithRetryable()
		if logID != "" {
			e = e.WithLogID(logID)
		}
		return e
	}
	subtype := errs.SubtypeUnknown
	if status == http.StatusNotFound {
		subtype = errs.SubtypeNotFound
	}
	e := errs.NewAPIError(subtype, "HTTP %d: %s", status, body).WithCode(status)
	if logID != "" {
		e = e.WithLogID(logID)
	}
	return e
}

// typedOrInternal passes an already-typed errs.* error through unchanged and
// lifts a still-untyped one to a typed internal error, so CallAPITyped never
// returns a bare/legacy error.
func typedOrInternal(err error) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.WrapInternal(err)
}

// APIClassifyContext builds the errclass.ClassifyContext for the running command
// from the runtime config and resolved identity.
func (ctx *RuntimeContext) APIClassifyContext() errclass.ClassifyContext {
	larkCmd := ""
	if ctx.Cmd != nil {
		larkCmd = strings.TrimPrefix(ctx.Cmd.CommandPath(), "lark ")
	}
	return errclass.ClassifyContext{
		Brand:    string(ctx.Config.Brand),
		AppID:    ctx.Config.AppID,
		Identity: string(ctx.As()),
		LarkCmd:  larkCmd,
	}
}

// Deprecated: RawAPI uses an internal HTTP wrapper with limited control over request/response.
// Prefer DoAPI for new code — it calls the Lark SDK directly and supports file upload/download options.
//
// RawAPI calls the Lark API using the current identity (ctx.As()) and returns raw result for manual error handling.
func (ctx *RuntimeContext) RawAPI(method, url string, params map[string]interface{}, data interface{}) (interface{}, error) {
	return ctx.callRaw(method, url, params, data)
}

// PaginateAll fetches all pages and returns a single merged result.
func (ctx *RuntimeContext) PaginateAll(method, url string, params map[string]interface{}, data interface{}, opts client.PaginationOptions) (interface{}, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	req := ctx.buildRequest(method, url, params, data)
	return ac.PaginateAll(ctx.ctx, req, opts)
}

// StreamPages fetches all pages and streams each page's items via onItems.
// Returns the last result (for error checking) and whether any list items were found.
func (ctx *RuntimeContext) StreamPages(method, url string, params map[string]interface{}, data interface{}, onItems func([]interface{}), opts client.PaginationOptions) (interface{}, bool, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, false, err
	}
	req := ctx.buildRequest(method, url, params, data)
	return ac.StreamPages(ctx.ctx, req, func(items []interface{}) error {
		onItems(items)
		return nil
	}, opts)
}

func (ctx *RuntimeContext) buildRequest(method, url string, params map[string]interface{}, data interface{}) client.RawApiRequest {
	req := client.RawApiRequest{
		Method: method,
		URL:    url,
		Params: params,
		Data:   data,
		As:     ctx.As(),
	}
	if optFn := cmdutil.ShortcutHeaderOpts(ctx.ctx); optFn != nil {
		req.ExtraOpts = append(req.ExtraOpts, optFn)
	}
	return req
}

func (ctx *RuntimeContext) callRaw(method, url string, params map[string]interface{}, data interface{}) (interface{}, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	return ac.CallAPI(ctx.ctx, ctx.buildRequest(method, url, params, data))
}

// DoAPI executes a raw Lark SDK request with automatic auth handling.
// Unlike CallAPI which parses JSON and extracts the "data" field, DoAPI returns
// the raw *larkcore.ApiResp — suitable for file downloads (WithFileDownload)
// and uploads (WithFileUpload).
//
// Auth resolution is delegated to APIClient.DoSDKRequest to avoid duplicating
// the identity → token logic across the generic and shortcut API paths.
func (ctx *RuntimeContext) DoAPI(req *larkcore.ApiReq, opts ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	if optFn := cmdutil.ShortcutHeaderOpts(ctx.ctx); optFn != nil {
		opts = append(opts, optFn)
	}
	return ac.DoSDKRequest(ctx.ctx, req, ctx.As(), opts...)
}

// DoAPIAsBot executes a raw Lark SDK request using bot identity (tenant access token),
// regardless of the current --as flag. Use this for APIs that must always be called
// with TAT even when the surrounding shortcut runs as user.
func (ctx *RuntimeContext) DoAPIAsBot(req *larkcore.ApiReq, opts ...larkcore.RequestOptionFunc) (*larkcore.ApiResp, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	if optFn := cmdutil.ShortcutHeaderOpts(ctx.ctx); optFn != nil {
		opts = append(opts, optFn)
	}
	return ac.DoSDKRequest(ctx.ctx, req, core.AsBot, opts...)
}

// DoAPIStream executes a streaming HTTP request via APIClient.DoStream.
// Unlike DoAPI (which buffers the full body via the SDK), DoAPIStream returns
// a live *http.Response whose Body is an io.Reader for streaming consumption.
// HTTP errors (status >= 400) are handled internally by DoStream.
func (ctx *RuntimeContext) DoAPIStream(callCtx context.Context, req *larkcore.ApiReq, opts ...client.Option) (*http.Response, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	base := []client.Option{
		client.WithHeaders(cmdutil.BaseSecurityHeaders()),
	}
	if h := cmdutil.ShortcutHeaders(ctx.ctx); h != nil {
		base = append(base, client.WithHeaders(h))
	}
	return ac.DoStream(callCtx, req, ctx.As(), append(base, opts...)...)
}

// DoAPIJSON calls the Lark API via DoAPI, parses the JSON response envelope,
// and returns the "data" field. Suitable for standard JSON APIs (non-file).
func (ctx *RuntimeContext) DoAPIJSON(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	return ctx.doAPIJSON(method, apiPath, query, body, false)
}

// DoAPIJSONWithLogID is like DoAPIJSON but merges x-tt-logid from the response
// header into the returned data and into error details as "log_id". Intended
// for endpoints where surfacing the log id aids troubleshooting (e.g. doc v2).
func (ctx *RuntimeContext) DoAPIJSONWithLogID(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	return ctx.doAPIJSON(method, apiPath, query, body, true)
}

// DoAPIJSONTyped is the typed-only replacement for DoAPIJSON: it issues the same
// larkcore.ApiReq request (identical method / path / query / body model) but
// classifies failures into typed errs.* errors via ClassifyAPIResponse instead
// of emitting a legacy output.ExitError "api_error" envelope. A transport / auth
// error from the client boundary is already typed and passes through unchanged;
// a non-zero API code is classified with subtype / code / log_id.
func (ctx *RuntimeContext) DoAPIJSONTyped(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     apiPath,
		QueryParams: query,
	}
	if body != nil {
		req.Body = body
	}
	resp, err := ctx.DoAPI(req)
	if err != nil {
		return nil, typedOrInternal(err)
	}
	return ctx.ClassifyAPIResponse(resp)
}

func (ctx *RuntimeContext) doAPIJSON(method, apiPath string, query larkcore.QueryParams, body any, includeLogID bool) (map[string]any, error) {
	req := &larkcore.ApiReq{
		HttpMethod:  method,
		ApiPath:     apiPath,
		QueryParams: query,
	}
	if body != nil {
		req.Body = body
	}
	resp, err := ctx.DoAPI(req)
	if err != nil {
		return nil, err
	}
	var detail map[string]any
	if includeLogID {
		detail = logIDFromHeader(resp)
	}
	if resp.StatusCode >= 400 {
		if len(resp.RawBody) > 0 {
			var errEnv struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if json.Unmarshal(resp.RawBody, &errEnv) == nil && errEnv.Msg != "" {
				return nil, output.ErrAPI(errEnv.Code, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errEnv.Msg), detail)
			}
		}
		return nil, output.ErrAPI(resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode), detail)
	}
	if len(resp.RawBody) == 0 {
		return nil, fmt.Errorf("empty response body")
	}
	var envelope struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(resp.RawBody, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	if envelope.Code != 0 {
		return nil, output.ErrAPI(envelope.Code, envelope.Msg, detail)
	}
	if detail != nil {
		if envelope.Data == nil {
			envelope.Data = make(map[string]any)
		}
		for k, v := range detail {
			envelope.Data[k] = v
		}
	}
	return envelope.Data, nil
}

// logIDFromHeader extracts x-tt-logid from response headers and returns it as a detail map.
// Returns nil if the header is absent.
func logIDFromHeader(resp *larkcore.ApiResp) map[string]any {
	if resp == nil {
		return nil
	}
	logID := resp.Header.Get("x-tt-logid")
	if logID == "" {
		return nil
	}
	return map[string]any{"log_id": logID}
}

// ── IO access ──

// IO returns the IOStreams from the Factory.
func (ctx *RuntimeContext) IO() *cmdutil.IOStreams {
	return ctx.Factory.IOStreams
}

// FileIO resolves the FileIO using the current execution context.
// Falls back to the globally registered provider when Factory or its
// FileIOProvider is nil (e.g. in lightweight test helpers).
func (ctx *RuntimeContext) FileIO() fileio.FileIO {
	if ctx != nil && ctx.Factory != nil {
		if fio := ctx.Factory.ResolveFileIO(ctx.ctx); fio != nil {
			return fio
		}
	}
	if p := fileio.GetProvider(); p != nil {
		c := context.Background()
		if ctx != nil {
			c = ctx.ctx
		}
		return p.ResolveFileIO(c)
	}
	return nil
}

// ResolveSavePath resolves a relative path to a validated absolute path via
// FileIO.ResolvePath. It returns an error if no FileIO provider is registered
// or if the path fails validation (e.g. traversal, symlink escape).
func (ctx *RuntimeContext) ResolveSavePath(path string) (string, error) {
	fio := ctx.FileIO()
	if fio == nil {
		return "", fmt.Errorf("no file I/O provider registered")
	}
	resolved, err := fio.ResolvePath(path)
	if err != nil {
		return "", fmt.Errorf("resolve save path: %w", err)
	}
	if resolved == "" {
		return "", fmt.Errorf("resolve save path: empty result for %q", path)
	}
	return resolved, nil
}

// WrapOpenError matches a FileIO.Open/Stat error and wraps it with the
// caller-provided message prefix.
func WrapOpenError(err error, pathMsg, readMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return fmt.Errorf("%s: %w", pathMsg, err)
	}
	return fmt.Errorf("%s: %w", readMsg, err)
}

// WrapInputStatError wraps a FileIO.Stat/Open error for input file validation,
// returning output.ErrValidation with the appropriate message:
//   - Path validation failures → "unsafe file path: ..."
//   - Other errors → readMsg prefix (default "cannot read file")
//
// Pass an optional readMsg to override the non-path-validation message prefix.
//
// Deprecated: use WrapInputStatErrorTyped for typed error envelopes.
func WrapInputStatError(err error, readMsg ...string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return output.ErrValidation("unsafe file path: %s", err)
	}
	msg := "cannot read file"
	if len(readMsg) > 0 && readMsg[0] != "" {
		msg = readMsg[0]
	}
	return output.ErrValidation("%s: %s", msg, err)
}

// WrapInputStatErrorTyped wraps a FileIO.Stat/Open error for input file validation.
func WrapInputStatErrorTyped(err error, readMsg ...string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).
			WithCause(err)
	}
	msg := "cannot read file"
	if len(readMsg) > 0 && readMsg[0] != "" {
		msg = readMsg[0]
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s: %s", msg, err).
		WithCause(err)
}

// WrapSaveErrorTyped maps a FileIO.Save error to typed validation/internal errors.
// Non-path failures always emit the canonical "internal" wire type; call sites
// migrating from a custom category (e.g. "io", "api_error") change their
// envelope's type field.
func WrapSaveErrorTyped(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).
			WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).
			WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).
			WithCause(err)
	}
}

// ValidatePath checks that path is a valid relative input path within the
// working directory by delegating to FileIO.Stat. Returns nil if the path is
// valid or does not exist yet; returns an error only for illegal paths
// (absolute, traversal, symlink escape, control chars).
//
// NOTE: This validates input (read) paths via SafeInputPath semantics inside
// the FileIO implementation. For output (write) path validation, use
// ResolveSavePath instead.
func (ctx *RuntimeContext) ValidatePath(path string) error {
	fio := ctx.FileIO()
	if fio == nil {
		return fmt.Errorf("no file I/O provider registered")
	}
	if _, err := fio.Stat(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ── Output helpers ──

// Out prints a success JSON envelope to stdout.
func (ctx *RuntimeContext) Out(data interface{}, meta *output.Meta) {
	ctx.emit(data, meta, false, true)
}

// OutRaw prints a success JSON envelope to stdout with HTML escaping disabled.
// Use this instead of Out when the data contains XML/HTML content (e.g. document bodies)
// that should be preserved as-is in JSON output.
func (ctx *RuntimeContext) OutRaw(data interface{}, meta *output.Meta) {
	ctx.emit(data, meta, true, true)
}

// OutPartialFailure writes an ok:false multi-status result envelope to stdout
// and returns the partial-failure exit signal. Use it for batch operations
// where some items failed but the per-item outcomes are the primary output:
// the full result (summary + per-item statuses) stays machine-readable on
// stdout, the process exits non-zero, and nothing is written to stderr.
//
// It is the typed alternative to `Out(...)` + `output.ErrBare(...)` — the
// envelope's ok field honestly reports failure instead of a misleading
// ok:true, and the exit signal is distinct from the predicate-only ErrBare.
func (ctx *RuntimeContext) OutPartialFailure(data interface{}, meta *output.Meta) error {
	ctx.emit(data, meta, false, false)
	if ctx.outputErr != nil {
		return ctx.outputErr
	}
	return output.PartialFailure(output.ExitAPI)
}

// emit is the shared stdout envelope emitter; ok sets the envelope's ok field
// (true for success, false for a partial-failure result). raw=true disables JSON
// HTML escaping so XML/HTML payloads (e.g. DocxXML bodies) are preserved
// verbatim; otherwise behavior
// is identical — content-safety scanning and race-safe first-error capture via
// outputErrOnce apply in both modes.
func (ctx *RuntimeContext) emit(data interface{}, meta *output.Meta, raw, ok bool) {
	scanResult := output.ScanForSafety(ctx.Cmd.CommandPath(), data, ctx.IO().ErrOut)
	if scanResult.Blocked {
		ctx.outputErrOnce.Do(func() { ctx.outputErr = scanResult.BlockErr })
		return
	}

	env := output.Envelope{OK: ok, Identity: string(ctx.As()), Data: data, Meta: meta, Notice: output.GetNotice()}
	if scanResult.Alert != nil {
		env.ContentSafetyAlert = scanResult.Alert
	}

	if ctx.JqExpr != "" {
		filter := output.JqFilter
		if raw {
			filter = output.JqFilterRaw
		}
		if err := filter(ctx.IO().Out, env, ctx.JqExpr); err != nil {
			fmt.Fprintf(ctx.IO().ErrOut, "error: %v\n", err)
			ctx.outputErrOnce.Do(func() { ctx.outputErr = err })
		}
		return
	}

	if raw {
		enc := json.NewEncoder(ctx.IO().Out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(env)
		return
	}
	b, _ := json.MarshalIndent(env, "", "  ")
	fmt.Fprintln(ctx.IO().Out, string(b))
}

// OutFormat prints output based on --format flag.
// "json" (default) outputs JSON envelope; "pretty" calls prettyFn; others delegate to FormatValue.
// When JqExpr is set, routes through Out() regardless of format.
// For json/"" and jq paths, Out() handles content safety scanning.
// For pretty/table/csv/ndjson, scanning is done here and the alert is written to stderr.
func (ctx *RuntimeContext) OutFormat(data interface{}, meta *output.Meta, prettyFn func(w io.Writer)) {
	ctx.outFormat(data, meta, prettyFn, false)
}

// OutFormatRaw is like OutFormat but with HTML escaping disabled in JSON output.
// Use this when the data contains XML/HTML content that should be preserved as-is.
func (ctx *RuntimeContext) OutFormatRaw(data interface{}, meta *output.Meta, prettyFn func(w io.Writer)) {
	ctx.outFormat(data, meta, prettyFn, true)
}

func (ctx *RuntimeContext) outFormat(data interface{}, meta *output.Meta, prettyFn func(w io.Writer), raw bool) {
	outFn := ctx.Out
	if raw {
		outFn = ctx.OutRaw
	}
	if ctx.JqExpr != "" {
		outFn(data, meta)
		return
	}
	switch ctx.Format {
	case "pretty":
		scanResult := output.ScanForSafety(ctx.Cmd.CommandPath(), data, ctx.IO().ErrOut)
		if scanResult.Blocked {
			ctx.outputErrOnce.Do(func() { ctx.outputErr = scanResult.BlockErr })
			return
		}
		if scanResult.Alert != nil {
			output.WriteAlertWarning(ctx.IO().ErrOut, scanResult.Alert)
		}
		if prettyFn != nil {
			prettyFn(ctx.IO().Out)
		} else {
			outFn(data, meta)
		}
	case "json", "":
		outFn(data, meta)
	default:
		// table, csv, ndjson — pass data directly; FormatValue handles both
		// plain arrays and maps with array fields (e.g. {"members":[…]})
		scanResult := output.ScanForSafety(ctx.Cmd.CommandPath(), data, ctx.IO().ErrOut)
		if scanResult.Blocked {
			ctx.outputErrOnce.Do(func() { ctx.outputErr = scanResult.BlockErr })
			return
		}
		if scanResult.Alert != nil {
			output.WriteAlertWarning(ctx.IO().ErrOut, scanResult.Alert)
		}
		format, formatOK := output.ParseFormat(ctx.Format)
		if !formatOK {
			fmt.Fprintf(ctx.IO().ErrOut, "warning: unknown format %q, falling back to json\n", ctx.Format)
		}
		output.FormatValue(ctx.IO().Out, data, format)
	}
}

// ── Scope pre-check ──

// checkScopePrereqs performs a fast local check: does the token
// contain all scopes declared by the shortcut? Returns the missing ones.
// If scope data is unavailable, returns nil (let the API call handle it).
func checkScopePrereqs(f *cmdutil.Factory, ctx context.Context, appID string, identity core.Identity, required []string) ([]string, error) {
	result, err := f.Credential.ResolveToken(ctx, credential.NewTokenSpec(identity, appID))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, nil
	}
	if result == nil || result.Scopes == "" {
		return nil, nil
	}
	return auth.MissingScopes(result.Scopes, required), nil
}

// enhancePermissionError enriches a permission / auth error with the
// shortcut's declared required scopes so the user knows exactly what to do.
//
// Detection is typed: an error qualifies when it (or any error in its
// Unwrap chain) is *errs.PermissionError, or — for legacy bridge paths —
// when it is an *output.ExitError carrying Detail.Type "permission" or
// "missing_scope". The previous implementation scanned the upstream
// message text for keywords like "permission" / "scope" / "unauthorized",
// which was brittle to canonical-message rewrites; routing on the typed
// shape decouples this helper from the wording.
func enhancePermissionError(err error, requiredScopes []string) error {
	var permErr *errs.PermissionError
	if errors.As(err, &permErr) {
		scopeDisplay := strings.Join(requiredScopes, ", ")
		scopeArg := strings.Join(requiredScopes, " ")
		hint := fmt.Sprintf(
			"this command requires scope(s): %s\nrun `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
			scopeDisplay, scopeArg)
		permErr.Hint = hint
		return err
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}
	if exitErr.Detail.Type != "permission" && exitErr.Detail.Type != "missing_scope" {
		return err
	}

	scopeDisplay := strings.Join(requiredScopes, ", ")
	scopeArg := strings.Join(requiredScopes, " ")
	hint := fmt.Sprintf(
		"this command requires scope(s): %s\nrun `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.",
		scopeDisplay, scopeArg)
	// Return a new error instead of mutating the original's Detail in place.
	return output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
}

// ── Mounting ──

// Mount registers the shortcut on a parent command.
func (s Shortcut) Mount(parent *cobra.Command, f *cmdutil.Factory) {
	s.MountWithContext(context.Background(), parent, f)
}

func (s Shortcut) MountWithContext(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory) {
	if s.Execute != nil {
		s.mountDeclarative(ctx, parent, f)
	}
}

func (s Shortcut) mountDeclarative(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory) {
	shortcut := s
	if len(shortcut.AuthTypes) == 0 {
		shortcut.AuthTypes = []string{"user"}
	}
	botOnly := len(shortcut.AuthTypes) == 1 && shortcut.AuthTypes[0] == "bot"

	cmd := &cobra.Command{
		Use:    shortcut.Command,
		Short:  shortcut.Description,
		Hidden: shortcut.Hidden,
		Args:   rejectPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShortcut(cmd, f, &shortcut, botOnly)
		},
	}
	if shortcut.PrintFlagSchema != nil || shortcut.OnInvoke != nil {
		onInvoke := shortcut.OnInvoke
		relaxRequiredForSchema := shortcut.PrintFlagSchema != nil
		// PreRunE runs before cobra's ValidateRequiredFlags. Two opt-in uses:
		//   - OnInvoke: fire a side effect (e.g. a deprecation notice) that must
		//     surface even when the call later fails on a missing required flag.
		//   - --print-schema: pure local introspection; relax the required-flag
		//     gate so callers don't fill in unrelated flags just to ask for a
		//     schema (clearing the annotation here is the supported opt-out).
		cmd.PreRunE = func(c *cobra.Command, _ []string) error {
			if onInvoke != nil {
				onInvoke()
			}
			if relaxRequiredForSchema {
				if want, _ := c.Flags().GetBool("print-schema"); want {
					c.Flags().VisitAll(func(fl *pflag.Flag) {
						delete(fl.Annotations, cobra.BashCompOneRequiredFlag)
					})
				}
			}
			return nil
		}
	}
	cmdmeta.SetSource(cmd, cmdmeta.SourceShortcut, false)
	cmdutil.SetSupportedIdentities(cmd, shortcut.AuthTypes)
	registerShortcutFlagsWithContext(ctx, cmd, f, &shortcut)
	cmdutil.SetTips(cmd, shortcut.Tips)
	cmdutil.SetRisk(cmd, shortcut.Risk)
	parent.AddCommand(cmd)
	if shortcut.PostMount != nil {
		shortcut.PostMount(cmd)
	}
}

// runShortcut is the execution pipeline for a declarative shortcut.
// Each step is a clear phase: identity → config → scopes → context → validate → execute.
func runShortcut(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut, botOnly bool) error {
	// --print-schema short-circuits everything below: it's pure local
	// introspection, no identity / scope / network needed. The flag is
	// only registered when the shortcut opts in via PrintFlagSchema.
	if s.PrintFlagSchema != nil {
		if want, _ := cmd.Flags().GetBool("print-schema"); want {
			flagName, _ := cmd.Flags().GetString("flag-name")
			out, err := s.PrintFlagSchema(strings.TrimSpace(flagName))
			if err != nil {
				// PrintFlagSchema implementations return bare errors; wrap as a
				// structured ExitError so --print-schema (an agent-facing
				// introspection path) yields a parseable envelope, not a plain
				// string.
				if _, ok := err.(*output.ExitError); !ok {
					err = output.Errorf(output.ExitValidation, "print_schema_error", "%s", err.Error())
				}
				return err
			}
			if len(out) == 0 {
				return nil
			}
			fmt.Fprintln(f.IOStreams.Out, string(out))
			return nil
		}
	}

	as, err := resolveShortcutIdentity(cmd, f, s)
	if err != nil {
		return err
	}

	config, err := f.Config()
	if err != nil {
		return err
	}
	// Identity info is now included in the JSON envelope; skip stderr printing.
	// cmdutil.PrintIdentity(f.IOStreams.ErrOut, as, config, false)

	if err := checkShortcutScopes(f, cmd.Context(), as, config, s.ScopesForIdentity(string(as))); err != nil {
		return err
	}

	rctx, err := newRuntimeContext(cmd, f, s, config, as, botOnly)
	if err != nil {
		return err
	}

	if err := validateEnumFlags(rctx, s.Flags); err != nil {
		return err
	}
	if err := resolveInputFlags(rctx, s.Flags); err != nil {
		return err
	}
	if err := output.ValidateJqFlags(rctx.JqExpr, "", rctx.Format); err != nil {
		return err
	}
	if s.Validate != nil {
		if err := s.Validate(rctx.ctx, rctx); err != nil {
			return err
		}
	}

	if rctx.Bool("dry-run") {
		return handleShortcutDryRun(f, rctx, s)
	}

	if s.Risk == "high-risk-write" && !rctx.Bool("yes") {
		return cmdutil.RequireConfirmation(s.Service + " " + s.Command)
	}

	if err := s.Execute(rctx.ctx, rctx); err != nil {
		return err
	}
	return rctx.outputErr
}

func resolveShortcutIdentity(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut) (core.Identity, error) {
	// Step 1: determine identity (--as > default-as > auto-detect).
	asFlag, _ := cmd.Flags().GetString("as")
	as := f.ResolveAs(cmd.Context(), cmd, core.Identity(asFlag))

	if err := f.CheckStrictMode(cmd.Context(), as); err != nil {
		return "", err
	}

	// Step 2: check if this shortcut supports the resolved identity.
	if err := f.CheckIdentity(as, s.AuthTypes); err != nil {
		return "", err
	}
	return as, nil
}

func checkShortcutScopes(f *cmdutil.Factory, ctx context.Context, as core.Identity, config *core.CliConfig, scopes []string) error {
	if len(scopes) == 0 {
		return nil
	}
	missing, err := checkScopePrereqs(f, ctx, config.AppID, as, scopes)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	return errs.NewPermissionError(errs.SubtypeMissingScope,
		"missing required scope(s): %s", strings.Join(missing, ", ")).
		WithIdentity(string(as)).
		WithMissingScopes(missing...).
		WithHint("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " "))
}

func newRuntimeContext(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut, config *core.CliConfig, as core.Identity, botOnly bool) (*RuntimeContext, error) {
	ctx := cmd.Context()
	ctx = cmdutil.ContextWithShortcut(ctx, s.Service+":"+s.Command, uuid.New().String())
	rctx := &RuntimeContext{ctx: ctx, Config: config, Cmd: cmd, botOnly: botOnly, resolvedAs: as, Factory: f}
	rctx.apiClientFunc = sync.OnceValues(func() (*client.APIClient, error) {
		return f.NewAPIClientWithConfig(config)
	})
	rctx.botInfoFunc = sync.OnceValues(rctx.fetchBotInfo)

	sdk, err := f.LarkClient()
	if err != nil {
		return nil, err
	}
	rctx.larkSDK = sdk

	rctx.Format = rctx.Str("format")
	rctx.JqExpr, _ = cmd.Flags().GetString("jq")
	return rctx, nil
}

// stripUTF8BOM removes a leading UTF-8 byte-order mark from content read from a
// file or stdin. A BOM that survives into a CSV cell corrupts the first value
// (e.g. "\ufeffNorth", which then makes a MAXIFS/lookup miss it), and a BOM at the
// head of a JSON payload makes json.Unmarshal fail with "invalid character 'ï'".
// Some editors and exporters add it silently. Only a leading BOM is removed; interior
// occurrences are left untouched.
func stripUTF8BOM(s string) string {
	return strings.TrimPrefix(s, "\uFEFF")
}

// resolveInputFlags resolves @file and - (stdin) for flags with Input sources.
// Must be called before Validate/DryRun/Execute so that runtime.Str() returns resolved content.
func resolveInputFlags(rctx *RuntimeContext, flags []Flag) error {
	stdinUsed := false
	for _, fl := range flags {
		if len(fl.Input) == 0 {
			continue
		}
		raw, err := rctx.Cmd.Flags().GetString(fl.Name)
		if err != nil {
			return ValidationErrorf("--%s: Input is only supported for string flags", fl.Name).
				WithParam("--" + fl.Name)
		}
		if raw == "" {
			continue
		}

		// stdin: -
		if raw == "-" {
			if !slices.Contains(fl.Input, Stdin) {
				return ValidationErrorf("--%s does not support stdin (-)", fl.Name).
					WithParam("--" + fl.Name)
			}
			if stdinUsed {
				return ValidationErrorf("--%s: stdin (-) can only be used by one flag", fl.Name).
					WithParam("--" + fl.Name)
			}
			stdinUsed = true
			data, err := io.ReadAll(rctx.IO().In)
			if err != nil {
				return ValidationErrorf("--%s: failed to read from stdin: %v", fl.Name, err).
					WithParam("--" + fl.Name).
					WithCause(err)
			}
			// strip a leading UTF-8 BOM so it can't corrupt the first CSV
			// cell or break JSON parsing downstream.
			rctx.Cmd.Flags().Set(fl.Name, stripUTF8BOM(string(data)))
			continue
		}

		// escape: @@ → literal @
		if strings.HasPrefix(raw, "@@") {
			rctx.Cmd.Flags().Set(fl.Name, raw[1:]) // strip first @
			continue
		}

		// file: @path
		if strings.HasPrefix(raw, "@") {
			if !slices.Contains(fl.Input, File) {
				return ValidationErrorf("--%s does not support file input (@path)", fl.Name).
					WithParam("--" + fl.Name)
			}
			path := strings.TrimSpace(raw[1:])
			if path == "" {
				return ValidationErrorf("--%s: file path cannot be empty after @", fl.Name).
					WithParam("--" + fl.Name)
			}
			data, err := cmdutil.ReadInputFile(rctx.FileIO(), path)
			if err != nil {
				return ValidationErrorf("--%s: %v", fl.Name, err).
					WithParam("--" + fl.Name).
					WithCause(err)
			}
			// strip a leading UTF-8 BOM so it
			// can't corrupt the first CSV cell or break JSON parsing downstream.
			rctx.Cmd.Flags().Set(fl.Name, stripUTF8BOM(string(data)))
			continue
		}
	}
	return nil
}

func validateEnumFlags(rctx *RuntimeContext, flags []Flag) error {
	for _, fl := range flags {
		if len(fl.Enum) == 0 {
			continue
		}
		val := rctx.Str(fl.Name)
		if val == "" {
			continue
		}
		valid := false
		for _, allowed := range fl.Enum {
			if val == allowed {
				valid = true
				break
			}
		}
		if !valid {
			return ValidationErrorf("invalid value %q for --%s, allowed: %s", val, fl.Name, strings.Join(fl.Enum, ", ")).
				WithParam("--" + fl.Name)
		}
	}
	return nil
}

func handleShortcutDryRun(f *cmdutil.Factory, rctx *RuntimeContext, s *Shortcut) error {
	if s.DryRun == nil {
		return ValidationErrorf("--dry-run is not supported for %s %s", s.Service, s.Command).
			WithParam("--dry-run")
	}
	fmt.Fprintln(f.IOStreams.ErrOut, "=== Dry Run ===")
	dryResult := s.DryRun(rctx.ctx, rctx)
	if rctx.Format == "pretty" {
		fmt.Fprint(f.IOStreams.Out, dryResult.Format())
	} else {
		output.PrintJson(f.IOStreams.Out, dryResult)
	}
	return nil
}

// rejectPositionalArgs returns a cobra.PositionalArgs that rejects any
// positional arguments. The error is intentionally a plain error (not
// ExitError) so that cobra prints usage and the root handler prints a
// simple "Error:" line instead of a JSON envelope.
func rejectPositionalArgs() cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("positional arguments are not supported (got %q); pass values via flags", args)
	}
}

func registerShortcutFlags(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut) {
	registerShortcutFlagsWithContext(context.Background(), cmd, f, s)
}

func registerShortcutFlagsWithContext(ctx context.Context, cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut) {
	for _, fl := range s.Flags {
		desc := fl.Desc
		if len(fl.Enum) > 0 {
			desc += " (" + strings.Join(fl.Enum, "|") + ")"
		}
		if len(fl.Input) > 0 {
			hints := make([]string, 0, 2)
			if slices.Contains(fl.Input, File) {
				hints = append(hints, "@file")
			}
			if slices.Contains(fl.Input, Stdin) {
				hints = append(hints, "- for stdin")
			}
			desc += " (supports " + strings.Join(hints, ", ") + ")"
		}
		switch fl.Type {
		case "bool":
			def := fl.Default == "true"
			cmd.Flags().Bool(fl.Name, def, desc)
		case "int":
			var d int
			fmt.Sscanf(fl.Default, "%d", &d)
			cmd.Flags().Int(fl.Name, d, desc)
		case "float64":
			var d float64
			fmt.Sscanf(fl.Default, "%g", &d)
			cmd.Flags().Float64(fl.Name, d, desc)
		case "string_array":
			cmd.Flags().StringArray(fl.Name, nil, desc)
		case "string_slice":
			cmd.Flags().StringSlice(fl.Name, nil, desc)
		default:
			cmd.Flags().String(fl.Name, fl.Default, desc)
		}
		if fl.Hidden {
			_ = cmd.Flags().MarkHidden(fl.Name)
		}
		if fl.Required {
			cmd.MarkFlagRequired(fl.Name)
		}
		if len(fl.Enum) > 0 {
			vals := fl.Enum
			cmdutil.RegisterFlagCompletion(cmd, fl.Name, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return vals, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}

	cmd.Flags().Bool("dry-run", false, "print request without executing")
	if cmd.Flags().Lookup("format") == nil {
		cmd.Flags().String("format", "json", "output format: json (default) | pretty | table | ndjson | csv")
		cmdutil.RegisterFlagCompletion(cmd, "format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "pretty", "table", "ndjson", "csv"}, cobra.ShellCompDirectiveNoFileComp
		})
		if cmd.Flags().Lookup("json") == nil {
			cmd.Flags().Bool("json", false, "shorthand for --format json")
		}
	}
	if s.Risk == "high-risk-write" {
		cmd.Flags().Bool("yes", false, "confirm high-risk operation")
	}
	if s.PrintFlagSchema != nil {
		// Guard against a shortcut that already declares these reserved
		// introspection flags: pflag panics on a duplicate registration.
		// Mirrors the Lookup guard on --format above.
		if cmd.Flags().Lookup("print-schema") == nil {
			cmd.Flags().Bool("print-schema", false, "print JSON Schema for a composite flag instead of executing")
		}
		if cmd.Flags().Lookup("flag-name") == nil {
			cmd.Flags().String("flag-name", "", "flag whose schema to print (omit to list introspectable flags); used with --print-schema")
		}
	}
	cmd.Flags().StringP("jq", "q", "", "jq expression to filter JSON output")
	cmdutil.AddShortcutIdentityFlag(ctx, cmd, f, s.AuthTypes)
}
