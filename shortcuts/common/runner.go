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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// RuntimeContext provides helpers for shortcut execution.
type RuntimeContext struct {
	ctx        context.Context // from cmd.Context(), propagated through the call chain
	Config     *core.CliConfig
	Cmd        *cobra.Command
	Format     string
	botOnly    bool              // set by framework for bot-only shortcuts
	resolvedAs core.Identity     // effective identity resolved by framework
	Factory    *cmdutil.Factory  // injected by framework
	apiClient  *client.APIClient // lazily initialized, cached
	larkSDK    *lark.Client      // eagerly initialized in mountDeclarative
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

// UserOpenId returns the current user's open_id from config.
func (ctx *RuntimeContext) UserOpenId() string { return ctx.Config.UserOpenId }

// Ctx returns the context.Context propagated from cmd.Context().
func (ctx *RuntimeContext) Ctx() context.Context { return ctx.ctx }

// getAPIClient returns the cached APIClient, creating it on first use.
func (ctx *RuntimeContext) getAPIClient() (*client.APIClient, error) {
	if ctx.apiClient != nil {
		return ctx.apiClient, nil
	}
	ac, err := ctx.Factory.NewAPIClient()
	if err != nil {
		return nil, err
	}
	// Override config with the one resolved for this context (may differ from Factory's)
	ac.Config = ctx.Config
	ctx.apiClient = ac
	return ac, nil
}

// AccessToken returns a valid access token for the current identity.
// For user: returns user access token (with auto-refresh).
// For bot: returns tenant access token.
func (ctx *RuntimeContext) AccessToken() (string, error) {
	if ctx.IsBot() {
		ac, err := ctx.getAPIClient()
		if err != nil {
			return "", output.ErrAuth("failed to get SDK: %s", err)
		}
		tatResp, err := ac.SDK.GetTenantAccessTokenBySelfBuiltApp(ctx.ctx, &larkcore.SelfBuiltTenantAccessTokenReq{
			AppID:     ctx.Config.AppID,
			AppSecret: ctx.Config.AppSecret,
		})
		if err != nil {
			return "", output.ErrAuth("failed to get tenant access token: %s", err)
		}
		return tatResp.TenantAccessToken, nil
	}
	httpClient, err := ctx.Factory.HttpClient()
	if err != nil {
		return "", output.ErrAuth("failed to get HTTP client: %s", err)
	}
	token, err := auth.GetValidAccessToken(httpClient, auth.NewUATCallOptions(ctx.Config, ctx.IO().ErrOut))
	if err != nil {
		return "", output.ErrAuth("failed to get access token: %s", err)
	}
	return token, nil
}

// LarkSDK returns the eagerly-initialized Lark SDK client.
func (ctx *RuntimeContext) LarkSDK() *lark.Client {
	return ctx.larkSDK
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

// StrArray returns a string-array flag value (repeated flag, no CSV splitting).
func (ctx *RuntimeContext) StrArray(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringArray(name)
	return v
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
	return ac.StreamPages(ctx.ctx, req, onItems, opts)
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
// regardless of the current --as flag. Use this for bot-only APIs (e.g. image/file upload)
// that must be called with TAT even when the surrounding shortcut runs as user.
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

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *cancelOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.cancel != nil {
		r.cancel()
	}
	return err
}

// DoAPIStream executes a streaming HTTP request against the Lark OpenAPI endpoint
// while preserving the framework's auth resolution, shortcut headers, and security headers.
func (ctx *RuntimeContext) DoAPIStream(callCtx context.Context, req *larkcore.ApiReq, timeout time.Duration, opts ...larkcore.RequestOptionFunc) (*http.Response, error) {
	httpClient, err := ctx.Factory.HttpClient()
	if err != nil {
		return nil, output.ErrNetwork("stream request failed: %s", err)
	}

	streamingClient := *httpClient
	if timeout > 0 {
		streamingClient.Timeout = timeout
	}

	requestCtx := callCtx
	cancel := func() {}
	if timeout > 0 {
		if _, hasDeadline := callCtx.Deadline(); !hasDeadline {
			requestCtx, cancel = context.WithTimeout(callCtx, timeout)
		}
	}

	var option larkcore.RequestOption
	for _, opt := range opts {
		opt(&option)
	}
	if option.Header == nil {
		option.Header = make(http.Header)
	}
	if shortcutHeaders := cmdutil.ShortcutHeaderOpts(ctx.ctx); shortcutHeaders != nil {
		shortcutHeaders(&option)
	}

	accessToken, err := ctx.AccessToken()
	if err != nil {
		cancel()
		return nil, err
	}

	requestURL, err := buildStreamRequestURL(ctx.Config.Brand, req)
	if err != nil {
		cancel()
		return nil, err
	}
	bodyReader, contentType, err := buildStreamRequestBody(req.Body)
	if err != nil {
		cancel()
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(requestCtx, req.HttpMethod, requestURL, bodyReader)
	if err != nil {
		cancel()
		return nil, output.ErrNetwork("stream request failed: %s", err)
	}
	for key, values := range cmdutil.BaseSecurityHeaders() {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	for key, values := range option.Header {
		for _, value := range values {
			httpReq.Header.Add(key, value)
		}
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	httpReq.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := streamingClient.Do(httpReq)
	if err != nil {
		cancel()
		return nil, output.ErrNetwork("stream request failed: %s", err)
	}
	resp.Body = &cancelOnCloseReadCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

func buildStreamRequestURL(brand core.LarkBrand, req *larkcore.ApiReq) (string, error) {
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
				return "", output.ErrValidation("missing path param %q for %s", pathKey, req.ApiPath)
			}
			if pathValue == "" {
				return "", output.ErrValidation("empty path param %q for %s", pathKey, req.ApiPath)
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

func buildStreamRequestBody(body interface{}) (io.Reader, string, error) {
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
			return nil, "", output.Errorf(output.ExitInternal, "api_error", "failed to encode request body: %s", err)
		}
		return bytes.NewReader(payload), "application/json", nil
	}
}

// DoAPIJSON calls the Lark API via DoAPI, parses the JSON response envelope,
// and returns the "data" field. Suitable for standard JSON APIs (non-file).
func (ctx *RuntimeContext) DoAPIJSON(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
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
	if resp.StatusCode >= 400 {
		if len(resp.RawBody) > 0 {
			var errEnv struct {
				Code int    `json:"code"`
				Msg  string `json:"msg"`
			}
			if json.Unmarshal(resp.RawBody, &errEnv) == nil && errEnv.Msg != "" {
				return nil, output.ErrAPI(errEnv.Code, fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errEnv.Msg), nil)
			}
		}
		return nil, output.ErrAPI(resp.StatusCode, fmt.Sprintf("HTTP %d", resp.StatusCode), nil)
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
		return nil, output.ErrAPI(envelope.Code, envelope.Msg, nil)
	}
	return envelope.Data, nil
}

// ── IO access ──

// IO returns the IOStreams from the Factory.
func (ctx *RuntimeContext) IO() *cmdutil.IOStreams {
	return ctx.Factory.IOStreams
}

// ── Output helpers ──

// Out prints a success JSON envelope to stdout.
func (ctx *RuntimeContext) Out(data interface{}, meta *output.Meta) {
	env := output.Envelope{OK: true, Identity: string(ctx.As()), Data: data, Meta: meta, Notice: output.GetNotice()}
	b, _ := json.MarshalIndent(env, "", "  ")
	fmt.Fprintln(ctx.IO().Out, string(b))
}

// OutFormat prints output based on --format flag.
// "json" (default) outputs JSON envelope; "pretty" calls prettyFn; others delegate to FormatValue.
func (ctx *RuntimeContext) OutFormat(data interface{}, meta *output.Meta, prettyFn func(w io.Writer)) {
	switch ctx.Format {
	case "pretty":
		if prettyFn != nil {
			prettyFn(ctx.IO().Out)
		} else {
			ctx.Out(data, meta)
		}
	case "json", "":
		ctx.Out(data, meta)
	default:
		// table, csv, ndjson — pass data directly; FormatValue handles both
		// plain arrays and maps with array fields (e.g. {"members":[…]})
		format, formatOK := output.ParseFormat(ctx.Format)
		if !formatOK {
			fmt.Fprintf(ctx.IO().ErrOut, "warning: unknown format %q, falling back to json\n", ctx.Format)
		}
		output.FormatValue(ctx.IO().Out, data, format)
	}
}

// ── Scope pre-check ──

// checkScopePrereqs performs a fast local check: does the stored token
// contain all scopes declared by the shortcut?  Returns the missing ones.
// If no token is stored, returns nil (let the normal auth flow handle it).
func checkScopePrereqs(appID, userOpenId string, required []string) []string {
	stored := auth.GetStoredToken(appID, userOpenId)
	if stored == nil {
		return nil // no token yet — auth flow will catch this later
	}
	return auth.MissingScopes(stored.Scope, required)
}

// enhancePermissionError enriches a permission / auth error with the
// shortcut's declared required scopes so the user knows exactly what to do.
func enhancePermissionError(err error, requiredScopes []string) error {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}

	// Detect permission-related errors by type or message keywords.
	isPermErr := exitErr.Detail.Type == "permission" || exitErr.Detail.Type == "missing_scope"
	if !isPermErr {
		lower := strings.ToLower(exitErr.Detail.Message)
		for _, kw := range []string{"permission", "scope", "authorization", "unauthorized"} {
			if strings.Contains(lower, kw) {
				isPermErr = true
				break
			}
		}
	}
	if !isPermErr {
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
	if s.Execute != nil {
		s.mountDeclarative(parent, f)
	}
}

func (s Shortcut) mountDeclarative(parent *cobra.Command, f *cmdutil.Factory) {
	shortcut := s
	if len(shortcut.AuthTypes) == 0 {
		shortcut.AuthTypes = []string{"user"}
	}
	botOnly := len(shortcut.AuthTypes) == 1 && shortcut.AuthTypes[0] == "bot"

	cmd := &cobra.Command{
		Use:   shortcut.Command,
		Short: shortcut.Description,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShortcut(cmd, f, &shortcut, botOnly)
		},
	}
	registerShortcutFlags(cmd, &shortcut)
	cmdutil.SetTips(cmd, shortcut.Tips)
	parent.AddCommand(cmd)
}

// runShortcut is the execution pipeline for a declarative shortcut.
// Each step is a clear phase: identity → config → scopes → context → validate → execute.
func runShortcut(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut, botOnly bool) error {
	as, err := resolveShortcutIdentity(cmd, f, s)
	if err != nil {
		return err
	}

	config, err := f.ResolveConfig(as)
	if err != nil {
		return err
	}
	// Identity info is now included in the JSON envelope; skip stderr printing.
	// cmdutil.PrintIdentity(f.IOStreams.ErrOut, as, config, false)

	if err := checkShortcutScopes(as, config, s.ScopesForIdentity(string(as))); err != nil {
		return err
	}

	rctx, err := newRuntimeContext(cmd, f, s, config, as, botOnly)
	if err != nil {
		return err
	}

	if err := validateEnumFlags(rctx, s.Flags); err != nil {
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

	if s.Risk == "high-risk-write" {
		if err := RequireConfirmation(s.Risk, rctx.Bool("yes"), s.Description); err != nil {
			return err
		}
	}

	return s.Execute(rctx.ctx, rctx)
}

func resolveShortcutIdentity(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut) (core.Identity, error) {
	// Step 1: determine identity (--as > default-as > auto-detect).
	asFlag, _ := cmd.Flags().GetString("as")
	as := f.ResolveAs(cmd, core.Identity(asFlag))

	// Step 2: check if this shortcut supports the resolved identity.
	if err := f.CheckIdentity(as, s.AuthTypes); err != nil {
		return "", err
	}
	return as, nil
}

func checkShortcutScopes(as core.Identity, config *core.CliConfig, scopes []string) error {
	if as != core.AsUser || len(scopes) == 0 || config.UserOpenId == "" {
		return nil
	}
	missing := checkScopePrereqs(config.AppID, config.UserOpenId, scopes)
	if len(missing) == 0 {
		return nil
	}
	return output.ErrWithHint(output.ExitAuth, "missing_scope",
		fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
		fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
}

func newRuntimeContext(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut, config *core.CliConfig, as core.Identity, botOnly bool) (*RuntimeContext, error) {
	ctx := cmd.Context()
	ctx = cmdutil.ContextWithShortcut(ctx, s.Service+":"+s.Command, uuid.New().String())
	rctx := &RuntimeContext{ctx: ctx, Config: config, Cmd: cmd, botOnly: botOnly, resolvedAs: as, Factory: f}

	sdk, err := f.LarkClient()
	if err != nil {
		return nil, err
	}
	rctx.larkSDK = sdk

	if s.HasFormat {
		rctx.Format = rctx.Str("format")
	}
	return rctx, nil
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
			return FlagErrorf("invalid value %q for --%s, allowed: %s", val, fl.Name, strings.Join(fl.Enum, ", "))
		}
	}
	return nil
}

func handleShortcutDryRun(f *cmdutil.Factory, rctx *RuntimeContext, s *Shortcut) error {
	if s.DryRun == nil {
		return FlagErrorf("--dry-run is not supported for %s %s", s.Service, s.Command)
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

func registerShortcutFlags(cmd *cobra.Command, s *Shortcut) {
	for _, fl := range s.Flags {
		desc := fl.Desc
		if len(fl.Enum) > 0 {
			desc += " (" + strings.Join(fl.Enum, "|") + ")"
		}
		switch fl.Type {
		case "bool":
			def := fl.Default == "true"
			cmd.Flags().Bool(fl.Name, def, desc)
		case "int":
			var d int
			fmt.Sscanf(fl.Default, "%d", &d)
			cmd.Flags().Int(fl.Name, d, desc)
		case "string_array":
			cmd.Flags().StringArray(fl.Name, nil, desc)
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
			_ = cmd.RegisterFlagCompletionFunc(fl.Name, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
				return vals, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}

	cmd.Flags().Bool("dry-run", false, "print request without executing")
	if s.HasFormat {
		cmd.Flags().String("format", "json", "output format: json (default) | pretty | table | ndjson | csv")
	}
	if s.Risk == "high-risk-write" {
		cmd.Flags().Bool("yes", false, "confirm high-risk operation")
	}
	cmd.Flags().String("as", s.AuthTypes[0], "identity type: user | bot")

	_ = cmd.RegisterFlagCompletionFunc("as", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return s.AuthTypes, cobra.ShellCompDirectiveNoFileComp
	})
	if s.HasFormat {
		_ = cmd.RegisterFlagCompletionFunc("format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "pretty", "table", "ndjson", "csv"}, cobra.ShellCompDirectiveNoFileComp
		})
	}
}
