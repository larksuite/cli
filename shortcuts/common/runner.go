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

type RuntimeContext struct {
	ctx           context.Context
	Config        *core.CliConfig
	Cmd           *cobra.Command
	Format        string
	JqExpr        string
	outputErrOnce sync.Once
	outputErr     error
	botOnly       bool
	resolvedAs    core.Identity
	Factory       *cmdutil.Factory
	apiClientFunc func() (*client.APIClient, error) // sync.OnceValues
	botInfoFunc   func() (*BotInfo, error)          // sync.OnceValues; /bot/v3/info
	larkSDK       *lark.Client
}

// As: bot-only shortcuts always return AsBot; otherwise honours --as / DefaultAs.
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

func (ctx *RuntimeContext) IsBot() bool {
	return ctx.As().IsBot()
}

func (ctx *RuntimeContext) UserOpenId() string { return ctx.Config.UserOpenId }

type BotInfo struct {
	OpenID  string
	AppName string
}

// BotInfo lazily calls /bot/v3/info; thread-safe via sync.OnceValues, called at most once.
func (ctx *RuntimeContext) BotInfo() (*BotInfo, error) {
	if ctx.botInfoFunc == nil {
		return nil, fmt.Errorf("BotInfo not available (runtime context not fully initialized)")
	}
	return ctx.botInfoFunc()
}

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

func (ctx *RuntimeContext) Ctx() context.Context { return ctx.ctx }

// getAPIClient: sync.OnceValues path; falls back to direct construction in test contexts.
func (ctx *RuntimeContext) getAPIClient() (*client.APIClient, error) {
	if ctx.apiClientFunc != nil {
		return ctx.apiClientFunc()
	}
	return ctx.Factory.NewAPIClientWithConfig(ctx.Config)
}

// AccessToken returns UAT (with auto-refresh) for user or TAT for bot.
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

func (ctx *RuntimeContext) LarkSDK() *lark.Client {
	return ctx.larkSDK
}

func (ctx *RuntimeContext) Str(name string) string {
	v, _ := ctx.Cmd.Flags().GetString(name)
	return v
}

func (ctx *RuntimeContext) Bool(name string) bool {
	v, _ := ctx.Cmd.Flags().GetBool(name)
	return v
}

func (ctx *RuntimeContext) Int(name string) int {
	v, _ := ctx.Cmd.Flags().GetInt(name)
	return v
}

// StrArray: repeated flag, no CSV splitting.
func (ctx *RuntimeContext) StrArray(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringArray(name)
	return v
}

// StrSlice: supports CSV splitting and repeated flags.
func (ctx *RuntimeContext) StrSlice(name string) []string {
	v, _ := ctx.Cmd.Flags().GetStringSlice(name)
	return v
}

// Changed reports whether the user explicitly set the flag (vs default).
func (ctx *RuntimeContext) Changed(name string) bool {
	f := ctx.Cmd.Flags().Lookup(name)
	if f == nil {
		return false
	}
	return f.Changed
}

// CallAPI is the legacy HTTP wrapper; prefer DoAPI for new code (file upload/download support).
func (ctx *RuntimeContext) CallAPI(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error) {
	result, err := ctx.callRaw(method, url, params, data)
	return HandleApiResult(result, err, "API call failed")
}

// Deprecated: prefer DoAPI for new code.
func (ctx *RuntimeContext) RawAPI(method, url string, params map[string]interface{}, data interface{}) (interface{}, error) {
	return ctx.callRaw(method, url, params, data)
}

func (ctx *RuntimeContext) PaginateAll(method, url string, params map[string]interface{}, data interface{}, opts client.PaginationOptions) (interface{}, error) {
	ac, err := ctx.getAPIClient()
	if err != nil {
		return nil, err
	}
	req := ctx.buildRequest(method, url, params, data)
	return ac.PaginateAll(ctx.ctx, req, opts)
}

// StreamPages: returns the last result and whether any list items were found.
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

// DoAPI returns raw *larkcore.ApiResp; suitable for file uploads/downloads.
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

// DoAPIAsBot forces tenant token regardless of --as.
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

// DoAPIStream returns a live *http.Response for streaming consumption (no full-body buffering).
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

// DoAPIJSON parses the response envelope and returns the "data" field.
func (ctx *RuntimeContext) DoAPIJSON(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	return ctx.doAPIJSON(method, apiPath, query, body, false)
}

// DoAPIJSONWithLogID merges x-tt-logid into data/error detail.
func (ctx *RuntimeContext) DoAPIJSONWithLogID(method, apiPath string, query larkcore.QueryParams, body any) (map[string]any, error) {
	return ctx.doAPIJSON(method, apiPath, query, body, true)
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

func (ctx *RuntimeContext) IO() *cmdutil.IOStreams {
	return ctx.Factory.IOStreams
}

// FileIO falls back to the global provider for lightweight test helpers.
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

// ResolveSavePath delegates to FileIO.ResolvePath; rejects traversal/symlink escape.
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

// WrapSaveError categorises FileIO.Save errors with caller-provided prefixes.
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

func WrapOpenError(err error, pathMsg, readMsg string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return fmt.Errorf("%s: %w", pathMsg, err)
	}
	return fmt.Errorf("%s: %w", readMsg, err)
}

// WrapInputStatError returns ErrValidation; path-validation errors get "unsafe file path".
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

// WrapSaveErrorByCategory: path validation always returns ErrValidation (exit 2).
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

// ValidatePath validates input (read) paths; for output paths use ResolveSavePath.
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

func (ctx *RuntimeContext) Out(data interface{}, meta *output.Meta) {
	ctx.emit(data, meta, false)
}

// OutRaw disables JSON HTML escaping; use for XML/HTML payloads.
func (ctx *RuntimeContext) OutRaw(data interface{}, meta *output.Meta) {
	ctx.emit(data, meta, true)
}

func (ctx *RuntimeContext) emit(data interface{}, meta *output.Meta, raw bool) {
	scanResult := output.ScanForSafety(ctx.Cmd.CommandPath(), data, ctx.IO().ErrOut)
	if scanResult.Blocked {
		ctx.outputErrOnce.Do(func() { ctx.outputErr = scanResult.BlockErr })
		return
	}

	env := output.Envelope{OK: true, Identity: string(ctx.As()), Data: data, Meta: meta, Notice: output.GetNotice()}
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

// OutFormat dispatches by --format flag; jq routes through Out() regardless.
func (ctx *RuntimeContext) OutFormat(data interface{}, meta *output.Meta, prettyFn func(w io.Writer)) {
	ctx.outFormat(data, meta, prettyFn, false)
}

// OutFormatRaw disables HTML escaping in JSON output.
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

// checkScopePrereqs returns the missing scopes; nil when scope data is unavailable.
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

// enhancePermissionError enriches permission errors with the required scopes.
func enhancePermissionError(err error, requiredScopes []string) error {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return err
	}

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
	return output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
}

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
	cmdutil.SetSupportedIdentities(cmd, shortcut.AuthTypes)
	registerShortcutFlagsWithContext(ctx, cmd, f, &shortcut)
	cmdutil.SetTips(cmd, shortcut.Tips)
	parent.AddCommand(cmd)
	if shortcut.PostMount != nil {
		shortcut.PostMount(cmd)
	}
}

// runShortcut: identity → config → scopes → context → validate → execute.
func runShortcut(cmd *cobra.Command, f *cmdutil.Factory, s *Shortcut, botOnly bool) error {
	as, err := resolveShortcutIdentity(cmd, f, s)
	if err != nil {
		return err
	}

	config, err := f.Config()
	if err != nil {
		return err
	}

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
	asFlag, _ := cmd.Flags().GetString("as")
	as := f.ResolveAs(cmd.Context(), cmd, core.Identity(asFlag))

	if err := f.CheckStrictMode(cmd.Context(), as); err != nil {
		return "", err
	}

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

// resolveInputFlags must run before Validate/DryRun/Execute so Str() returns resolved content.
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

		if strings.HasPrefix(raw, "@@") {
			rctx.Cmd.Flags().Set(fl.Name, raw[1:]) // @@ → literal @
			continue
		}

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

// rejectPositionalArgs returns a plain error (not ExitError) so cobra prints usage.
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
