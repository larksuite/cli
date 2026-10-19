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
	Factory     *cmdutil.Factory
	Ctx         context.Context
	JSON        bool
	Scope       string
	Recommend   bool
	Domains     []string
	NoWait      bool
	DeviceCode  string
	RedirectURI string
	Timeout     int
	OIDC        bool // OIDC authentication flag
}

// NewCmdAuthLogin creates the auth login subcommand.
func NewCmdAuthLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authorization Code Flow login (with Device Flow fallback)",
		Long: `Authorization Code Flow login with local callback server.

For backwards compatibility, --no-wait flag enables Device Flow.
For AI agents: this command blocks until the user completes authorization in the
browser. Run it in the background and retrieve the verification URL from its output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLoginRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to request (space-separated)")
	cmd.Flags().BoolVar(&opts.Recommend, "recommend", false, "request only recommended (auto-approve) scopes")
	available := sortedKnownDomains()
	cmd.Flags().StringSliceVar(&opts.Domains, "domain", nil,
		fmt.Sprintf("domain (repeatable or comma-separated, e.g. --domain calendar,task)\navailable: %s, all", strings.Join(available, ", ")))
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmd.Flags().BoolVar(&opts.NoWait, "no-wait", false, "initiate device authorization and return immediately; use --device-code to complete")
	cmd.Flags().StringVar(&opts.DeviceCode, "device-code", "", "poll and complete authorization with a device code from a previous --no-wait call")
	cmd.Flags().StringVar(&opts.RedirectURI, "redirect-uri", "http://localhost:3000/callback", "OAuth callback URI (for auth-code flow)")
	cmd.Flags().IntVar(&opts.Timeout, "timeout", 300, "timeout in seconds (for auth-code flow)")
	cmd.Flags().BoolVar(&opts.OIDC, "oidc", false, "use OIDC (OpenID Connect) authentication instead of OAuth2")

	_ = cmd.RegisterFlagCompletionFunc("domain", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

func authLoginRun(opts *LoginOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	// If using --device-code, use device flow
	if opts.DeviceCode != "" {
		return authLoginPollDeviceCode(opts, config, getLoginMsg(getUILang(config)), func(format string, a ...interface{}) {
			if !opts.JSON {
				fmt.Fprintf(f.IOStreams.ErrOut, format+"\n", a...)
			}
		})
	}

	// Determine UI language from saved config
	lang := getUILang(config)
	msg := getLoginMsg(lang)

	log := func(format string, a ...interface{}) {
		if !opts.JSON {
			fmt.Fprintf(f.IOStreams.ErrOut, format+"\n", a...)
		}
	}

	// Get selected domains and scope level
	selectedDomains := opts.Domains
	scopeLevel := "" // "common" or "all" (from interactive mode)

	// Expand --domain all to all available domains (from_meta projects + shortcut services)
	for _, d := range selectedDomains {
		if strings.EqualFold(d, "all") {
			domainSet := make(map[string]bool)
			for _, p := range registry.ListFromMetaProjects() {
				domainSet[p] = true
			}
			for _, sc := range shortcuts.AllShortcuts() {
				domainSet[sc.Service] = true
			}
			selectedDomains = make([]string, 0, len(domainSet))
			for d := range domainSet {
				selectedDomains = append(selectedDomains, d)
			}
			sort.Strings(selectedDomains)
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
		// When no options are specified, provide a hint to the user instead of automatically starting auth flow
		// This prevents the process from being killed when trying to start a local server unnecessarily
		if !opts.JSON && f.IOStreams.IsTerminal {
			// Show a message with common usage examples instead of launching interactive or auth flow
			fmt.Fprintf(f.IOStreams.ErrOut, "To login, specify scopes, domains, or use recommended permissions:\n")
			fmt.Fprintf(f.IOStreams.ErrOut, "  lark-cli auth login --scope \"your-scopes\"\n")
			fmt.Fprintf(f.IOStreams.ErrOut, "  lark-cli auth login --domain base,calendar\n")
			fmt.Fprintf(f.IOStreams.ErrOut, "  lark-cli auth login --recommend\n")
			fmt.Fprintf(f.IOStreams.ErrOut, "Or use the simplified flow directly:\n")
			fmt.Fprintf(f.IOStreams.ErrOut, "  lark-cli auth login-code\n")
			return output.ErrValidation("no login options specified")
		} else {
			// In non-terminal environments, provide minimal guidance
			fmt.Fprintf(f.IOStreams.Out, "{\"event\":\"error\",\"message\":\"no login options specified\"}\n")
			return output.ErrValidation("no login options specified")
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

	// Use Authorization Code Flow by default (new behavior)
	// --no-wait now triggers device flow (for backwards compatibility)
	if opts.NoWait {
		return authLoginWithDeviceFlow(opts, config, finalScope, msg, log)
	}

	// Use OIDC flow if --oidc flag is set
	if opts.OIDC {
		return authLoginWithOIDCFlow(opts, config, finalScope)
	}

	// Use Authorization Code Flow by default
	return authLoginWithAuthCodeFlow(opts, config, finalScope)
}

// authLoginPollDeviceCode resumes the device flow by polling with a device code
// obtained from a previous --no-wait call.
func authLoginPollDeviceCode(opts *LoginOptions, config *core.CliConfig, msg *loginMsg, log func(string, ...interface{})) error {
	f := opts.Factory

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}
	log(msg.WaitingAuth)
	result := larkauth.PollDeviceToken(opts.Ctx, httpClient, config.AppID, config.AppSecret, config.Brand,
		opts.DeviceCode, 5, 180, f.IOStreams.ErrOut)

	if !result.OK {
		return output.ErrAuth("authorization failed: %s", result.Message)
	}
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

	// Store token
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:   openId,
		AppId:        config.AppID,
		AuthProtocol: "oauth_v2", Authority: core.ResolveOpenBaseURL(config.Brand),
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
	if err := core.MergeAppUser(config.AppID, core.AppUser{UserOpenId: openId, UserName: userName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf(msg.LoginSuccess, userName, openId))
	return nil
}

// collectScopesForDomains collects API scopes (from from_meta projects) and
// shortcut scopes for the given domain names.
func collectScopesForDomains(domains []string, identity string) []string {
	scopeSet := make(map[string]bool)

	// 1. API scopes from from_meta projects
	for _, s := range registry.CollectScopesForProjects(domains, identity) {
		scopeSet[s] = true
	}

	// 2. Shortcut scopes matching by Service (only include shortcuts supporting the identity)
	domainSet := make(map[string]bool, len(domains))
	for _, d := range domains {
		domainSet[d] = true
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if domainSet[sc.Service] && shortcutSupportsIdentity(sc, identity) {
			for _, s := range sc.ScopesForIdentity(identity) {
				scopeSet[s] = true
			}
		}
	}

	// 3. Deduplicate and sort
	result := make([]string, 0, len(scopeSet))
	for s := range scopeSet {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}

// allKnownDomains returns all valid domain names (from_meta projects + shortcut services).
func allKnownDomains() map[string]bool {
	domains := make(map[string]bool)
	for _, p := range registry.ListFromMetaProjects() {
		domains[p] = true
	}
	for _, sc := range shortcuts.AllShortcuts() {
		domains[sc.Service] = true
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

// getUILang gets the UI language from config
func getUILang(config *core.CliConfig) string {
	lang := "zh"
	if multi, _ := core.LoadMultiAppConfig(); multi != nil && len(multi.Apps) > 0 {
		lang = multi.Apps[0].Lang
	}
	return lang
}

// authLoginWithAuthCodeFlow performs authentication using the authorization code flow
func authLoginWithAuthCodeFlow(opts *LoginOptions, config *core.CliConfig, scope string) error {
	f := opts.Factory

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}

	// Get domain from Brand (which may be a custom domain URL)
	endpoints, err := config.ResolveServiceEndpoints()
	if err != nil {
		return err
	}

	flowOpts := &larkauth.AuthCodeFlowOptions{
		AppID:       config.AppID,
		AppSecret:   config.AppSecret,
		Domain:      endpoints.Open,
		RedirectURI: opts.RedirectURI,
		Scope:       scope,
		Timeout:     time.Duration(opts.Timeout) * time.Second,
	}

	result, err := larkauth.StartAuthCodeFlow(opts.Ctx, flowOpts, httpClient, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("authorization failed: %v", err)
	}

	// Store token
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:   result.OpenID,
		AppId:        config.AppID,
		AuthProtocol: "oauth_v2", Authority: endpoints.Open,
		AccessToken:      result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresAt:        now + int64(result.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(result.RefreshExpiresIn)*1000,
		Scope:            result.Scope,
		GrantedAt:        now,
	}
	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	// Update config
	if err := core.MergeAppUser(config.AppID, core.AppUser{UserOpenId: result.OpenID, UserName: result.UserName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	if opts.JSON {
		b, _ := json.Marshal(map[string]interface{}{
			"event":        "authorization_complete",
			"user_open_id": result.OpenID,
			"user_name":    result.UserName,
			"scope":        result.Scope,
		})
		fmt.Fprintln(f.IOStreams.Out, string(b))
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Login successful: %s (%s)", result.UserName, result.OpenID))
	}
	return nil
}

// authLoginWithOIDCFlow performs authentication using the OIDC flow
func authLoginWithOIDCFlow(opts *LoginOptions, config *core.CliConfig, scope string) error {
	f := opts.Factory

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}

	endpoints, err := config.ResolveServiceEndpoints()
	if err != nil {
		return err
	}

	flowOpts := &larkauth.OIDCFlowOptions{
		AuthProtocol: config.AuthProtocol,
		AppID:        config.AppID,
		AppSecret:    config.AppSecret,
		Domain:       endpoints.Accounts,
		OpenDomain:   endpoints.Open,
		RedirectURI:  opts.RedirectURI,
		Timeout:      time.Duration(opts.Timeout) * time.Second,
		Scope:        scope, // 传递 scope 参数以请求 API 权限
	}

	result, err := larkauth.StartOIDCFlow(opts.Ctx, flowOpts, httpClient, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("OIDC authentication failed: %v", err)
	}

	// Store token using the UpdateFromOIDCResult helper
	storedToken := &larkauth.StoredUAToken{
		AppId:      config.AppID,
		UserOpenId: result.OpenID,
	}
	storedToken.UpdateFromOIDCResult(result)

	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	// Update config
	if err := core.MergeAppUser(config.AppID, core.AppUser{UserOpenId: result.OpenID, UserName: result.UserName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	if opts.JSON {
		b, _ := json.Marshal(map[string]interface{}{
			"event":        "oidc_authorization_complete",
			"user_open_id": result.OpenID,
			"user_name":    result.UserName,
			"email":        result.Email,
			"scope":        result.Scope,
			"expires_in":   result.ExpiresIn,
		})
		fmt.Fprintln(f.IOStreams.Out, string(b))
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("OIDC login successful: %s (%s)", result.UserName, result.OpenID))

	}
	return nil
}

// authLoginWithDeviceFlow performs authentication using the device flow (original implementation)
func authLoginWithDeviceFlow(opts *LoginOptions, config *core.CliConfig, scope string, msg *loginMsg, log func(string, ...interface{})) error {
	f := opts.Factory

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}

	authResp, err := larkauth.RequestDeviceAuthorization(httpClient, config.AppID, config.AppSecret, config.Brand, scope, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("device authorization failed: %v", err)
	}

	// --no-wait: return immediately with device code and URL
	b, _ := json.Marshal(map[string]interface{}{
		"verification_url": authResp.VerificationUriComplete,
		"device_code":      authResp.DeviceCode,
		"expires_in":       authResp.ExpiresIn,
		"hint":             fmt.Sprintf("Show verification_url to user, then immediately execute: lark-cli auth login --device-code %s (blocks until authorized or timeout). Do not instruct the user to run this command themselves.", authResp.DeviceCode),
	})
	fmt.Fprintln(f.IOStreams.Out, string(b))
	return nil
}
