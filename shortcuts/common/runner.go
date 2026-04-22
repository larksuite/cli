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

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
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

// UserOpenId returns the current user's open_id from config.
func (ctx *RuntimeContext) UserOpenId() string { return ctx.Config.UserOpenId }

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
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			OpenID  string `json:"open_id"`
			AppName string `json:"app_name"`
		} `json:"data"`
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

// StrSlice returns a string-slice flag value (supports CSV splitting and repeated flags).
func (ctx *RuntimeContext) StrSlice(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringSlice(name)
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

// WrapSaveError matches a FileIO.Save error against known categories and wraps
// it with the caller-provided message prefix, preserving backward-compatible
// error text per shortcut.
func WrapSaveError(err error, pathMsg, mkdirMsg, writeMsg string) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	var we *fileio.WriteError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return fmt.Errorf("%s: %w", pathMsg, err)
	case errors.As(err, &me):
		return fmt.Errorf("%s: %w", mkdirMsg, err)
	case errors.As(err, &we):
		return fmt.Errorf("%s: %w", writeMsg, err)
	default:
		return fmt.Errorf("%s: %w", writeMsg, err)
	}
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

// WrapSaveErrorByCategory maps a FileIO.Save error to structured output errors,
// using standardized messages and the given error category (e.g. "api_error", "io").
// Path validation errors always use ErrValidation (exit code 2).
func WrapSaveErrorByCategory(err error, category string) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return output.ErrValidation("unsafe output path: %s", err)
	case errors.As(err, &me):
		return output.Errorf(output.ExitInternal, category, "cannot create parent directory: %s", err)
	default:
		return output.Errorf(output.ExitInternal, category, "cannot create file: %s", err)
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
	env := output.Envelope{OK: true, Identity: string(ctx.As()), Data: data, Meta: meta, Notice: output.GetNotice()}
	if ctx.JqExpr != "" {
		if err := output.JqFilter(ctx.IO().Out, env, ctx.JqExpr); err != nil {
			fmt.Fprintf(ctx.IO().ErrOut, "error: %v\n", err)
			ctx.outputErrOnce.Do(func() { ctx.outputErr = err })
		}
		return
	}
	b, _ := json.MarshalIndent(env, "", "  ")
	fmt.Fprintln(ctx.IO().Out, string(b))
}

// OutFormat prints output based on --format flag.
// "json" (default) outputs JSON envelope; "pretty" calls prettyFn; others delegate to FormatValue.
// When JqExpr is set, routes through Out() regardless of format.
func (ctx *RuntimeContext) OutFormat(data interface{}, meta *output.Meta, prettyFn func(w io.Writer)) {
	if ctx.JqExpr != "" {
		ctx.Out(data, meta)
		return
	}
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
		Use:   shortcut.Command,
		Short: shortcut.Description,
		Args:  rejectPositionalArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShortcut(cmd, f, &shortcut, botOnly)
		},
	}
	cmdutil.SetSupportedIdentities(cmd, shortcut.AuthTypes)
	registerShortcutFlagsWithContext(ctx, cmd, f, &shortcut)
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

	if s.Risk == "high-risk-write" {
		if err := RequireConfirmation(s.Risk, rctx.Bool("yes"), s.Description); err != nil {
			return err
		}
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
	return output.ErrWithHint(output.ExitAuth, "missing_scope",
		fmt.Sprintf("missing required scope(s): %s", strings.Join(missing, ", ")),
		fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", strings.Join(missing, " ")))
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

	if s.HasFormat {
		rctx.Format = rctx.Str("format")
	}
	rctx.JqExpr, _ = cmd.Flags().GetString("jq")
	return rctx, nil
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
			return FlagErrorf("--%s: Input is only supported for string flags", fl.Name)
		}
		if raw == "" {
			continue
		}

		// stdin: -
		if raw == "-" {
			if !slices.Contains(fl.Input, Stdin) {
				return FlagErrorf("--%s does not support stdin (-)", fl.Name)
			}
			if stdinUsed {
				return FlagErrorf("--%s: stdin (-) can only be used by one flag", fl.Name)
			}
			stdinUsed = true
			data, err := io.ReadAll(rctx.IO().In)
			if err != nil {
				return FlagErrorf("--%s: failed to read from stdin: %v", fl.Name, err)
			}
			rctx.Cmd.Flags().Set(fl.Name, string(data))
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
				return FlagErrorf("--%s does not support file input (@path)", fl.Name)
			}
			path := strings.TrimSpace(raw[1:])
			if path == "" {
				return FlagErrorf("--%s: file path cannot be empty after @", fl.Name)
			}
			f, err := rctx.FileIO().Open(path)
			if err != nil {
				if errors.Is(err, fileio.ErrPathValidation) {
					return FlagErrorf("--%s: invalid file path %q: %v", fl.Name, path, err)
				}
				return FlagErrorf("--%s: cannot read file %q: %v", fl.Name, path, err)
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return FlagErrorf("--%s: cannot read file %q: %v", fl.Name, path, err)
			}
			rctx.Cmd.Flags().Set(fl.Name, string(data))
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
	if s.HasFormat {
		cmd.Flags().String("format", "json", "output format: json (default) | pretty | table | ndjson | csv")
	}
	if s.Risk == "high-risk-write" {
		cmd.Flags().Bool("yes", false, "confirm high-risk operation")
	}
	cmd.Flags().StringP("jq", "q", "", "jq expression to filter JSON output")
	cmdutil.AddShortcutIdentityFlag(ctx, cmd, f, s.AuthTypes)
	if s.HasFormat {
		cmdutil.RegisterFlagCompletion(cmd, "format", func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return []string{"json", "pretty", "table", "ndjson", "csv"}, cobra.ShellCompDirectiveNoFileComp
		})
	}
}
