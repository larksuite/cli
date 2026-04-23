// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

// LoginOptions holds all inputs for auth login.
type LoginOptions struct {
	Factory    *cmdutil.Factory
	Ctx        context.Context
	JSON       bool
	Scope      string
	Recommend  bool
	Domains    []string
	NoWait     bool
	DeviceCode string
}

var (
	pollDeviceToken = larkauth.PollDeviceToken
	openBrowserFn   = openBrowser
)

// NewCmdAuthLogin creates the auth login subcommand.
func NewCmdAuthLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Device Flow authorization login",
		Long: `Device Flow authorization login.

For AI agents: this command blocks until the user completes authorization in the
browser. Run it in the background and retrieve the verification URL from its output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode := f.ResolveStrictMode(cmd.Context()); mode == core.StrictModeBot {
				return output.Errorf(output.ExitValidation, "strict_mode",
					"strict mode is %q, user login is not allowed. "+
						"This setting is managed by the administrator and must not be modified by AI agents.",
					mode)
			}
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLoginRun(opts)
		},
	}
	cmdutil.SetSupportedIdentities(cmd, []string{"user"})

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to request (space-separated)")
	cmd.Flags().BoolVar(&opts.Recommend, "recommend", false, "request only recommended (auto-approve) scopes")
	available := sortedKnownDomains()
	cmd.Flags().StringSliceVar(&opts.Domains, "domain", nil,
		fmt.Sprintf("domain (repeatable or comma-separated, e.g. --domain calendar,task)\navailable: %s, all", strings.Join(available, ", ")))
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.NoWait, "no-wait", false, "initiate device authorization and return immediately; use --device-code to complete")
	cmd.Flags().StringVar(&opts.DeviceCode, "device-code", "", "poll and complete authorization with a device code from a previous --no-wait call")

	cmdutil.RegisterFlagCompletion(cmd, "domain", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeDomain(toComplete), cobra.ShellCompDirectiveNoFileComp
	})

	return cmd
}

// completeDomain returns completions for comma-separated domain values.
func completeDomain(toComplete string) []string {
	allDomains := registry.ListFromMetaProjects()
	parts := strings.Split(toComplete, ",")
	prefix := parts[len(parts)-1]
	base := strings.Join(parts[:len(parts)-1], ",")

	var completions []string
	for _, d := range allDomains {
		if strings.HasPrefix(d, prefix) {
			if base == "" {
				completions = append(completions, d)
			} else {
				completions = append(completions, base+","+d)
			}
		}
	}
	return completions
}

// authLoginRun executes the login command logic.
func authLoginRun(opts *LoginOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	// Determine UI language from saved config
	lang := "zh"
	if multi, _ := core.LoadMultiAppConfig(); multi != nil {
		if app := multi.FindApp(config.ProfileName); app != nil {
			lang = app.Lang
		}
	}
	msg := getLoginMsg(lang)

	log := func(format string, a ...interface{}) {
		if !opts.JSON {
			fmt.Fprintf(f.IOStreams.ErrOut, format+"\n", a...)
		}
	}

	// --device-code: resume polling from a previous --no-wait call
	if opts.DeviceCode != "" {
		return authLoginPollDeviceCode(opts, config, msg, log)
	}

	selectedDomains := opts.Domains
	scopeLevel := "" // "common" or "all" (from interactive mode)

	// Expand --domain all to all available domains (from_meta projects + shortcut services)
	for _, d := range selectedDomains {
		if strings.EqualFold(d, "all") {
			selectedDomains = sortedKnownDomains()
			break
		}
	}

	// Validate domain names and suggest corrections for unknown ones
	if len(selectedDomains) > 0 {
		knownDomains := allKnownDomains()
		for _, d := range selectedDomains {
			if !knownDomains[d] {
				if suggestion := suggestDomain(d, knownDomains); suggestion != "" {
					return output.ErrValidation("unknown domain %q, did you mean %q?", d, suggestion)
				}
				available := make([]string, 0, len(knownDomains))
				for k := range knownDomains {
					available = append(available, k)
				}
				sort.Strings(available)
				return output.ErrValidation("unknown domain %q, available domains: %s", d, strings.Join(available, ", "))
			}
		}
	}

	hasAnyOption := opts.Scope != "" || opts.Recommend || len(selectedDomains) > 0

	if !hasAnyOption {
		if !opts.JSON && f.IOStreams.IsTerminal {
			result, err := runInteractiveLogin(f.IOStreams, lang, msg)
			if err != nil {
				return err
			}
			if result == nil {
				return output.ErrValidation("no login options selected")
			}
			selectedDomains = result.Domains
			scopeLevel = result.ScopeLevel
		} else {
			log(msg.HintHeader)
			log("Common options:")
			log(msg.HintCommon1)
			log(msg.HintCommon2)
			log(msg.HintCommon3)
			log(msg.HintCommon4)
			log("")
			log("View all options:")
			log(msg.HintFooter)
			log("")
			log("Note: this command blocks until authorization is complete. Run it in the background and retrieve the verification URL from its output.")
			return output.ErrValidation("please specify the scopes to authorize")
		}
	}

	finalScope := opts.Scope

	// Resolve scopes from domain/permission filters
	if len(selectedDomains) > 0 || opts.Recommend {
		if opts.Scope != "" {
			return output.ErrValidation("cannot use --scope together with --domain/--recommend")
		}

		var candidateScopes []string
		if len(selectedDomains) > 0 {
			candidateScopes = collectScopesForDomains(selectedDomains, "user")
		} else {
			// --recommend without --domain: all domains
			candidateScopes = collectScopesForDomains(sortedKnownDomains(), "user")
		}

		// Filter to auto-approve scopes if --recommend or interactive "common"
		if opts.Recommend || scopeLevel == "common" {
			candidateScopes = registry.FilterAutoApproveScopes(candidateScopes)
		}

		if len(candidateScopes) == 0 {
			return output.ErrValidation("no matching scopes found, check domain/scope options")
		}

		finalScope = strings.Join(candidateScopes, " ")
	}

	if config.UserTokenGetterUrl != "" && config.AppSecret == "" {
		return authLoginViaGetter(opts, config, finalScope, msg, log)
	}

	// Step 1: Request device authorization
	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}
	authResp, err := larkauth.RequestDeviceAuthorization(httpClient, config.AppID, config.AppSecret, config.Brand, finalScope, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("device authorization failed: %v", err)
	}

	// --no-wait: return immediately with device code and URL
	if opts.NoWait {
		if err := saveLoginRequestedScope(authResp.DeviceCode, finalScope); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth login: failed to cache requested scopes: %v\n", err)
		}
		data := map[string]interface{}{
			"verification_url": authResp.VerificationUriComplete,
			"device_code":      authResp.DeviceCode,
			"expires_in":       authResp.ExpiresIn,
			"hint":             fmt.Sprintf("Show verification_url to user, then immediately execute: lark-cli auth login --device-code %s (blocks until authorized or timeout). Do not instruct the user to run this command themselves.", authResp.DeviceCode),
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "failed to write JSON output: %v", err)
		}
		return nil
	}

	// Step 2: Show user code and verification URL
	if opts.JSON {
		data := map[string]interface{}{
			"event":                     "device_authorization",
			"verification_uri":          authResp.VerificationUri,
			"verification_uri_complete": authResp.VerificationUriComplete,
			"user_code":                 authResp.UserCode,
			"expires_in":                authResp.ExpiresIn,
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "failed to write JSON output: %v", err)
		}
	} else {
		fmt.Fprintf(f.IOStreams.ErrOut, msg.OpenURL)
		fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", authResp.VerificationUriComplete)
	}

	// Step 3: Poll for token
	log(msg.WaitingAuth)
	result := pollDeviceToken(opts.Ctx, httpClient, config.AppID, config.AppSecret, config.Brand,
		authResp.DeviceCode, authResp.Interval, authResp.ExpiresIn, f.IOStreams.ErrOut)

	if !result.OK {
		if opts.JSON {
			encoder := json.NewEncoder(f.IOStreams.Out)
			encoder.SetEscapeHTML(false)
			if err := encoder.Encode(map[string]interface{}{
				"event": "authorization_failed",
				"error": result.Message,
			}); err != nil {
				return output.Errorf(output.ExitInternal, "internal", "failed to write JSON output: %v", err)
			}
			return output.ErrBare(output.ExitAuth)
		}
		return output.ErrAuth("authorization failed: %s", result.Message)
	}
	if result.Token == nil {
		return output.ErrAuth("authorization succeeded but no token returned")
	}

	// Step 6: Get user info
	log(msg.AuthSuccess)
	sdk, err := f.LarkClient()
	if err != nil {
		return output.ErrAuth("failed to get SDK: %v", err)
	}
	openId, userName, err := getUserInfo(opts.Ctx, sdk, result.Token.AccessToken)
	if err != nil {
		return output.ErrAuth("failed to get user info: %v", err)
	}

	scopeSummary := loadLoginScopeSummary(config.AppID, openId, finalScope, result.Token.Scope)

	// Step 7: Store token
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:       openId,
		AppId:            config.AppID,
		AccessToken:      result.Token.AccessToken,
		RefreshToken:     result.Token.RefreshToken,
		ExpiresAt:        now + int64(result.Token.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(result.Token.RefreshExpiresIn)*1000,
		Scope:            result.Token.Scope,
		GrantedAt:        now,
	}
	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	// Step 8: Update config — overwrite Users to single user, clean old tokens
	if err := syncLoginUserToProfile(config.ProfileName, config.AppID, openId, userName); err != nil {
		_ = larkauth.RemoveStoredToken(config.AppID, openId)
		return output.Errorf(output.ExitInternal, "internal", "failed to update login profile: %v", err)
	}

	if issue := ensureRequestedScopesGranted(finalScope, result.Token.Scope, msg, scopeSummary); issue != nil {
		return handleLoginScopeIssue(opts, msg, f, issue, openId, userName)
	}

	writeLoginSuccess(opts, msg, f, openId, userName, scopeSummary)
	return nil
}

// UserTokenData structure for capturing user token response from auth getter.
type UserTokenData struct {
	AccessToken  *string `json:"access_token,omitempty"`  // user_access_token，用于获取用户资源
	TokenType    *string `json:"token_type,omitempty"`    // token 类型
	ExpiresIn    *int    `json:"expires_in,omitempty"`    // `access_token`的有效期，单位: 秒
	Name         *string `json:"name,omitempty"`          // 用户姓名
	EnName       *string `json:"en_name,omitempty"`       // 用户英文名称
	AvatarUrl    *string `json:"avatar_url,omitempty"`    // 用户头像
	AvatarThumb  *string `json:"avatar_thumb,omitempty"`  // 用户头像 72x72
	AvatarMiddle *string `json:"avatar_middle,omitempty"` // 用户头像 240x240
	AvatarBig    *string `json:"avatar_big,omitempty"`    // 用户头像 640x640
	OpenId       *string `json:"open_id,omitempty"`       // 用户在应用内的唯一标识
	UnionId      *string `json:"union_id,omitempty"`      // 用户统一ID
	UserId       *string `json:"user_id,omitempty"`       // 用户 user_id
	TenantKey    *string `json:"tenant_key,omitempty"`    // 当前企业标识
}

// authLoginViaGetter executes the login command logic via user token getter.
func authLoginViaGetter(opts *LoginOptions, config *core.CliConfig, finalScope string, msg *loginMsg, log func(string, ...interface{})) error {
	f := opts.Factory
	token, err := fetchTokenViaGetter(opts.Ctx, config.UserTokenGetterUrl, finalScope, log)
	if err != nil {
		return output.ErrAuth("failed to fetch user token via url: %v", err)
	}

	var gt UserTokenData
	if err := json.Unmarshal([]byte(token), &gt); err != nil {
		return output.ErrAuth("failed to unmarshal token JSON: %v", err)
	}

	if gt.AccessToken == nil || *gt.AccessToken == "" {
		return output.ErrAuth("authorization succeeded but no access_token returned")
	}

	openId := ""
	if gt.OpenId != nil {
		openId = *gt.OpenId
	}
	userName := ""
	if gt.Name != nil {
		userName = *gt.Name
	}
	expiresIn := 0
	if gt.ExpiresIn != nil {
		expiresIn = *gt.ExpiresIn
	}

	// 如果没有 open_id/name，依然需要通过 SDK 获取
	if openId == "" || userName == "" {
		log(msg.AuthSuccess)
		sdk, err := f.LarkClient()
		if err != nil {
			return output.ErrAuth("failed to get SDK: %v", err)
		}
		// NOTE: getUserInfo requires access token
		fetchedOpenId, fetchedUserName, err := getUserInfo(opts.Ctx, sdk, *gt.AccessToken)
		if err != nil {
			return output.ErrAuth("failed to get user info: %v", err)
		}
		if openId == "" {
			openId = fetchedOpenId
		}
		if userName == "" {
			userName = fetchedUserName
		}
	} else {
		log(msg.AuthSuccess)
	}

	now := time.Now().UnixMilli()
	expiresAt := now + int64(expiresIn)*1000
	if expiresIn <= 0 {
		expiresAt = now + 7200*1000 // 默认 2h
	}

	storedToken := &larkauth.StoredUAToken{
		UserOpenId:       openId,
		AppId:            config.AppID,
		AccessToken:      *gt.AccessToken,
		RefreshToken:     "", // 这种方式不支持 refresh token
		ExpiresAt:        expiresAt,
		RefreshExpiresAt: expiresAt,
		Scope:            "", // 这种方式暂不解析 scope
		GrantedAt:        now,
	}

	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	if err := syncLoginUserToProfile(config.ProfileName, config.AppID, openId, userName); err != nil {
		_ = larkauth.RemoveStoredToken(config.AppID, openId)
		return output.Errorf(output.ExitInternal, "internal", "failed to update login profile: %v", err)
	}

	writeLoginSuccess(opts, msg, f, openId, userName, nil)
	return nil
}

// fetchTokenViaGetter retrieves a user access token by opening a local server to receive the token via an OAuth callback.
func fetchTokenViaGetter(ctx context.Context, getterURL string, scope string, log func(string, ...interface{})) (string, error) {
	rand.Seed(time.Now().UnixNano())
	var listener net.Listener
	var port int
	var err error

	for i := 0; i < 10; i++ {
		p := 8000 + rand.Intn(2000)
		listener, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			port = p
			break
		}
	}
	if listener == nil {
		return "", fmt.Errorf("failed to start local server for token retrieval: %w", err)
	}

	tokenCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/user_access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		var tokenData string
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read body", http.StatusBadRequest)
				return
			}
			tokenData = string(body)
		} else {
			tokenData = r.URL.Query().Get("token")
		}

		if tokenData == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "<html><body><h2>Failed</h2><p>Missing token data</p></body></html>")
			select {
			case errCh <- fmt.Errorf("missing token data in callback request"):
			default:
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<html><body style="text-align:center;padding-top:100px;font-family:sans-serif">
<h2>✓ Success</h2><p>You can close this page and return to the terminal.</p></body></html>`)

		select {
		case tokenCh <- tokenData:
		default:
		}
	})

	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case errCh <- fmt.Errorf("local server error: %w", err):
			default:
			}
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	log("Waiting for authorization, local server started on http://127.0.0.1:%d/user_access_token...", port)

	u, err := url.Parse(getterURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse getterURL: %w", err)
	}
	q := u.Query()
	q.Set("state", strconv.Itoa(port))
	if scope != "" {
		q.Set("scope", scope)
	}
	u.RawQuery = q.Encode()
	finalURL := u.String()

	if err := openBrowserFn(ctx, finalURL); err != nil {
		log("Could not open browser automatically. Please visit the following link manually:")
	} else {
		log("Opening browser to get token...")
		log("If the browser does not open automatically, please visit:")
	}
	log("  %s\n", finalURL)

	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("context canceled: %w", ctx.Err())
	case token := <-tokenCh:
		return token, nil
	case err := <-errCh:
		return "", err
	case <-timer.C:
		return "", fmt.Errorf("timeout waiting for token callback (5 minutes)")
	}
}

// openBrowser opens the specified URL in the user's default browser.
// It tries to use system-specific commands depending on the OS (linux, darwin, windows).
func openBrowser(ctx context.Context, url string) error {
	// 简单的跨平台打开浏览器实现
	var err error
	switch runtime.GOOS {
	case "linux":
		err = exec.CommandContext(ctx, "xdg-open", url).Start()
	case "darwin":
		err = exec.CommandContext(ctx, "open", url).Start()
	case "windows":
		err = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		// Go fallback
		err = exec.CommandContext(ctx, "open", url).Start()
		if err != nil {
			err = exec.CommandContext(ctx, "xdg-open", url).Start()
		}
	}
	return err
}

// authLoginPollDeviceCode resumes the device flow by polling with a device code
// obtained from a previous --no-wait call.
func authLoginPollDeviceCode(opts *LoginOptions, config *core.CliConfig, msg *loginMsg, log func(string, ...interface{})) error {
	f := opts.Factory

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}
	requestedScope, err := loadLoginRequestedScope(opts.DeviceCode)
	if err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth login: failed to load cached requested scopes: %v\n", err)
	}
	cleanupRequestedScope := func() {
		if err := removeLoginRequestedScope(opts.DeviceCode); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "[lark-cli] [WARN] auth login: failed to remove cached requested scopes: %v\n", err)
		}
	}
	log(msg.WaitingAuth)
	result := pollDeviceToken(opts.Ctx, httpClient, config.AppID, config.AppSecret, config.Brand,
		opts.DeviceCode, 5, 180, f.IOStreams.ErrOut)

	if !result.OK {
		if shouldRemoveLoginRequestedScope(result) {
			cleanupRequestedScope()
		}
		return output.ErrAuth("authorization failed: %s", result.Message)
	}
	defer cleanupRequestedScope()
	if result.Token == nil {
		return output.ErrAuth("authorization succeeded but no token returned")
	}

	// Get user info
	log(msg.AuthSuccess)
	sdk, err := f.LarkClient()
	if err != nil {
		return output.ErrAuth("failed to get SDK: %v", err)
	}
	openId, userName, err := getUserInfo(opts.Ctx, sdk, result.Token.AccessToken)
	if err != nil {
		return output.ErrAuth("failed to get user info: %v", err)
	}

	scopeSummary := loadLoginScopeSummary(config.AppID, openId, requestedScope, result.Token.Scope)

	// Store token
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:       openId,
		AppId:            config.AppID,
		AccessToken:      result.Token.AccessToken,
		RefreshToken:     result.Token.RefreshToken,
		ExpiresAt:        now + int64(result.Token.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(result.Token.RefreshExpiresIn)*1000,
		Scope:            result.Token.Scope,
		GrantedAt:        now,
	}
	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	// Update config — overwrite Users to single user, clean old tokens
	if err := syncLoginUserToProfile(config.ProfileName, config.AppID, openId, userName); err != nil {
		_ = larkauth.RemoveStoredToken(config.AppID, openId)
		return output.Errorf(output.ExitInternal, "internal", "failed to update login profile: %v", err)
	}

	if issue := ensureRequestedScopesGranted(requestedScope, result.Token.Scope, msg, scopeSummary); issue != nil {
		return handleLoginScopeIssue(opts, msg, f, issue, openId, userName)
	}

	writeLoginSuccess(opts, msg, f, openId, userName, scopeSummary)
	return nil
}

// syncLoginUserToProfile updates the profile configuration to only contain the provided user.
// It removes any old stored tokens for other users associated with this app ID.
func syncLoginUserToProfile(profileName, appID, openID, userName string) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	app := findProfileByName(multi, profileName)
	if app == nil {
		return fmt.Errorf("profile %q not found in config", profileName)
	}

	oldUsers := append([]core.AppUser(nil), app.Users...)
	app.Users = []core.AppUser{{UserOpenId: openID, UserName: userName}}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	for _, oldUser := range oldUsers {
		if oldUser.UserOpenId != openID {
			_ = larkauth.RemoveStoredToken(appID, oldUser.UserOpenId)
		}
	}
	return nil
}

// findProfileByName retrieves an app configuration by its profile name.
// It returns nil if the profile is not found.
func findProfileByName(multi *core.MultiAppConfig, profileName string) *core.AppConfig {
	for i := range multi.Apps {
		if multi.Apps[i].ProfileName() == profileName {
			return &multi.Apps[i]
		}
	}
	return nil
}

// collectScopesForDomains collects API scopes (from from_meta projects) and
// shortcut scopes for the given domain names.
// Domains with auth_domain children are automatically expanded to include
// their children's scopes.
func collectScopesForDomains(domains []string, identity string) []string {
	scopeSet := make(map[string]bool)

	// 1. API scopes from from_meta projects
	for _, s := range registry.CollectScopesForProjects(domains, identity) {
		scopeSet[s] = true
	}

	// 2. Expand domains: include auth_domain children
	domainSet := make(map[string]bool, len(domains))
	for _, d := range domains {
		domainSet[d] = true
		for _, child := range registry.GetAuthChildren(d) {
			domainSet[child] = true
		}
	}

	// 3. Shortcut scopes matching by Service (only include shortcuts supporting the identity)
	for _, sc := range shortcuts.AllShortcuts() {
		if domainSet[sc.Service] && shortcutSupportsIdentity(sc, identity) {
			for _, s := range sc.ScopesForIdentity(identity) {
				scopeSet[s] = true
			}
		}
	}

	// 4. Deduplicate and sort
	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// allKnownDomains returns all valid auth domain names (from_meta projects +
// shortcut services), excluding domains that have auth_domain set (they are
// folded into their parent domain).
func allKnownDomains() map[string]bool {
	domains := make(map[string]bool)
	for _, p := range registry.ListFromMetaProjects() {
		if !registry.HasAuthDomain(p) {
			domains[p] = true
		}
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if !registry.HasAuthDomain(sc.Service) {
			domains[sc.Service] = true
		}
	}
	return domains
}

// sortedKnownDomains returns all valid domain names sorted alphabetically.
func sortedKnownDomains() []string {
	m := allKnownDomains()
	domains := make([]string, 0, len(m))
	for d := range m {
		domains = append(domains, d)
	}
	sort.Strings(domains)
	return domains
}

// shortcutSupportsIdentity checks if a shortcut supports the given identity ("user" or "bot").
// Empty AuthTypes defaults to ["user"].
func shortcutSupportsIdentity(sc common.Shortcut, identity string) bool {
	authTypes := sc.AuthTypes
	if len(authTypes) == 0 {
		authTypes = []string{"user"}
	}
	for _, t := range authTypes {
		if t == identity {
			return true
		}
	}
	return false
}

// suggestDomain finds the best "did you mean" match for an unknown domain.
func suggestDomain(input string, known map[string]bool) string {
	// Check common cases: prefix match or input is a substring
	for k := range known {
		if strings.HasPrefix(k, input) || strings.HasPrefix(input, k) {
			return k
		}
	}
	return ""
}
