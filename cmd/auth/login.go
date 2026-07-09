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

	"github.com/larksuite/cli/errs"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/i18n"
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
	Exclude    []string
	NoWait     bool
	DeviceCode string
}

var pollDeviceToken = larkauth.PollDeviceToken

// NewCmdAuthLogin creates the auth login subcommand.
func NewCmdAuthLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Device Flow authorization login",
		Long: `Device Flow authorization login.

For AI agents: this command blocks until the user completes authorization in the
browser. If your harness or agent tool only delivers final turn messages, use --no-wait --json,
send the verification URL (or QR code) to the user as your final message, end the turn, then
run --device-code in a later step after the user confirms authorization. Use 'lark-cli auth qrcode'
to generate QR codes (supports ASCII and PNG formats).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode := f.ResolveStrictMode(cmd.Context()); mode == core.StrictModeBot {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"strict mode is %q, user login is disabled in this profile", mode).
					WithHint("if the user explicitly wants to switch to user identity, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)")
			}
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLoginRun(opts)
		},
	}
	cmdutil.SetSupportedIdentities(cmd, []string{"user"})
	cmdutil.SetRisk(cmd, "write")

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to request (space- or comma-separated). Combines additively with --domain/--recommend")
	cmd.Flags().BoolVar(&opts.Recommend, "recommend", false, "request only recommended (auto-approve) scopes")
	var helpBrand core.LarkBrand
	if f != nil && f.Config != nil {
		if cfg, err := f.Config(); err == nil && cfg != nil {
			helpBrand = cfg.Brand
		}
	}
	available := sortedKnownDomains(helpBrand)
	cmd.Flags().StringSliceVar(&opts.Domains, "domain", nil,
		fmt.Sprintf("domain (repeatable or comma-separated, e.g. --domain calendar,task)\navailable: %s, all", strings.Join(available, ", ")))
	cmd.Flags().StringSliceVar(&opts.Exclude, "exclude", nil,
		"scopes to exclude from the request (repeatable or comma-separated, e.g. --exclude drive:file:download)")
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
	var lang i18n.Lang
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
			selectedDomains = sortedKnownDomains(config.Brand)
			break
		}
	}

	// Validate domain names and suggest corrections for unknown ones
	if len(selectedDomains) > 0 {
		knownDomains := allKnownDomains(config.Brand)
		for _, d := range selectedDomains {
			if !knownDomains[d] {
				if suggestion := suggestDomain(d, knownDomains); suggestion != "" {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown domain %q, did you mean %q?", d, suggestion).WithParam("--domain")
				}
				available := make([]string, 0, len(knownDomains))
				for k := range knownDomains {
					available = append(available, k)
				}
				sort.Strings(available)
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown domain %q, available domains: %s", d, strings.Join(available, ", ")).WithParam("--domain")
			}
		}
	}

	hasAnyOption := opts.Scope != "" || opts.Recommend || len(selectedDomains) > 0

	if len(opts.Exclude) > 0 && !hasAnyOption {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--exclude requires --scope, --domain, or --recommend to be specified").WithParam("--exclude")
	}

	if !hasAnyOption {
		if !opts.JSON && f.IOStreams.IsTerminal {
			result, err := runInteractiveLogin(f.IOStreams, lang.Base(), msg, config.Brand)
			if err != nil {
				return err
			}
			if result == nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "no login options selected")
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
			log("Note: this command blocks until authorization is complete. For non-streaming agent harnesses, use --no-wait --json, send the verification URL as the final message of the turn, then run --device-code in a later step after the user confirms authorization.")
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "please specify the scopes to authorize").WithParam("--scope")
		}
	}

	// Normalize --scope so users can pass either OAuth-standard space-separated
	// values or the more natural comma-separated list. RFC 6749 §3.3 mandates
	// space-delimited scopes in the wire request, so the device authorization
	// endpoint rejects raw "a,b" strings as a single malformed scope.
	finalScope := normalizeScopeInput(opts.Scope)

	// Resolve scopes from domain/permission filters and merge with --scope.
	// --scope, --domain, and --recommend combine additively so callers can,
	// for example, request all `docs` scopes plus a few specific `drive`
	// scopes in a single command.
	if len(selectedDomains) > 0 || opts.Recommend {
		var candidateScopes []string
		if len(selectedDomains) > 0 {
			candidateScopes = collectScopesForDomains(selectedDomains, "user", config.Brand)
		} else {
			// --recommend without --domain: all domains
			candidateScopes = collectScopesForDomains(sortedKnownDomains(config.Brand), "user", config.Brand)
		}

		// Filter to auto-approve scopes if --recommend or interactive "common"
		if opts.Recommend || scopeLevel == "common" {
			candidateScopes = registry.FilterAutoApproveScopes(candidateScopes)
		}

		if len(candidateScopes) == 0 && opts.Scope == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "no matching scopes found, check domain/scope options")
		}

		// Merge --scope additively with the resolved domain scopes.
		merged := make(map[string]bool, len(candidateScopes)+len(strings.Fields(finalScope)))
		for _, s := range candidateScopes {
			merged[s] = true
		}
		for _, s := range strings.Fields(finalScope) {
			merged[s] = true
		}
		finalScope = joinSortedScopeSet(merged)
	}

	// Apply --exclude on top of the resolved scope set. We honour exclude
	// regardless of whether scopes came from --scope, --domain, --recommend,
	// or any combination thereof.
	if len(opts.Exclude) > 0 {
		excluded, unknown := applyExcludeScopes(finalScope, opts.Exclude)
		if len(unknown) > 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"these --exclude scopes are not present in the requested set: %s",
				strings.Join(unknown, ", ")).WithParam("--exclude")
		}
		finalScope = excluded
		if strings.TrimSpace(finalScope) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "no scopes left after applying --exclude; nothing to authorize").WithParam("--exclude")
		}
	}

	// Step 1: Request device authorization
	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}
	authResp, err := larkauth.RequestDeviceAuthorization(httpClient, config.AppID, config.AppSecret, config.Brand, finalScope, f.IOStreams.ErrOut)
	if err != nil {
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "device authorization failed: %v", err).WithCause(err)
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
			"hint": "**MUST generate QR code AND display it:** You MUST call lark-cli auth qrcode to convert verification_url into a QR code. This is a required step, do NOT skip it. Prefer PNG QR code (--output); use ASCII (--ascii) only when the user explicitly requests it." +
				"**CRITICAL: You MUST include the QR image in your response.** Generating the file alone is NOT enough—use image tags, inline images, or file attachments to display it." +
				"**Display order:** Output the URL first, then place the QR code image below the URL." +
				"**URL Output Rules:** Treat verification_url as an opaque string that cannot be modified. Do NOT URL-encode/decode or add spaces/punctuation." +
				"For agent harnesses that only deliver final turn messages, make the QR code image (or URL) the final message of the turn and return control to the user; do not block on --device-code in the same turn. **Before ending the turn, tell the user to come back and notify you after completing authorization.**" +
				"**After the user confirms authorization:** YOU must execute `lark-cli auth login --device-code <device_code>` yourself." +
				"**Do NOT cache verification_url or device_code for future use.** Always run `lark-cli auth login --no-wait --json` fresh when authorization is needed.",
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "failed to write JSON output: %v", err).WithCause(err)
		}
		return nil
	}

	// Step 2: Show user code and verification URL.
	// JSON mode embeds AgentTimeoutHint as a structured field so agents that
	// capture stdout into a JSON parser see it without stream-mixing surprises.
	// Text mode prints the hint to stderr only when running under a non-TTY
	// (i.e. piped / agent harness), since humans reading a terminal don't need
	// the agent-oriented instructions.
	if opts.JSON {
		data := map[string]interface{}{
			"event":                     "device_authorization",
			"verification_uri":          authResp.VerificationUri,
			"verification_uri_complete": authResp.VerificationUriComplete,
			"user_code":                 authResp.UserCode,
			"expires_in":                authResp.ExpiresIn,
			"agent_hint":                msg.AgentTimeoutHint,
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "failed to write JSON output: %v", err).WithCause(err)
		}
	} else {
		fmt.Fprintf(f.IOStreams.ErrOut, msg.OpenURL)
		fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", authResp.VerificationUriComplete)
		if f.IOStreams != nil && !f.IOStreams.IsTerminal {
			fmt.Fprintln(f.IOStreams.ErrOut, msg.AgentTimeoutHint)
		}
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
				return errs.NewInternalError(errs.SubtypeSDKError, "failed to write JSON output: %v", err).WithCause(err)
			}
			return output.ErrBare(output.ExitAuth)
		}
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "authorization failed: %s", result.Message)
	}
	if result.Token == nil {
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "authorization succeeded but no token returned")
	}

	// Step 6: Get user info
	log(msg.AuthSuccess)
	sdk, err := f.LarkClient()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to get SDK: %v", err).WithCause(err)
	}
	openId, userName, err := getUserInfo(opts.Ctx, sdk, result.Token.AccessToken)
	if err != nil {
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "failed to get user info: %v", err).WithCause(err)
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
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save token: %v", err).WithCause(err)
	}

	// Step 8: Update config — overwrite Users to single user, clean old tokens
	if err := syncLoginUserToProfile(config.ProfileName, config.AppID, openId, userName); err != nil {
		_ = larkauth.RemoveStoredToken(config.AppID, openId)
		return err
	}

	if issue := ensureRequestedScopesGranted(finalScope, result.Token.Scope, msg, scopeSummary); issue != nil {
		return handleLoginScopeIssue(opts, msg, f, issue, openId, userName)
	}

	writeLoginSuccess(opts, msg, f, openId, userName, scopeSummary)
	return nil
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
	// Skip the stderr hint in JSON mode (the --no-wait call that issued
	// the device_code already surfaced it as a JSON field), and also skip it
	// when running on an interactive terminal — the agent-oriented
	// instructions only matter for piped / harness environments.
	if !opts.JSON && f.IOStreams != nil && !f.IOStreams.IsTerminal {
		fmt.Fprintln(f.IOStreams.ErrOut, msg.AgentTimeoutHint)
	}
	log(msg.WaitingAuth)
	result := pollDeviceToken(opts.Ctx, httpClient, config.AppID, config.AppSecret, config.Brand,
		opts.DeviceCode, 5, 600, f.IOStreams.ErrOut)

	if !result.OK {
		if shouldRemoveLoginRequestedScope(result) {
			cleanupRequestedScope()
		}
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "authorization failed: %s", result.Message)
	}
	defer cleanupRequestedScope()
	if result.Token == nil {
		return errs.NewAuthenticationError(errs.SubtypeTokenMissing, "authorization succeeded but no token returned")
	}

	// Get user info
	log(msg.AuthSuccess)
	sdk, err := f.LarkClient()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to get SDK: %v", err).WithCause(err)
	}
	openId, userName, err := getUserInfo(opts.Ctx, sdk, result.Token.AccessToken)
	if err != nil {
		return errs.NewAuthenticationError(errs.SubtypeUnknown, "failed to get user info: %v", err).WithCause(err)
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
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to save token: %v", err).WithCause(err)
	}

	// Update config — overwrite Users to single user, clean old tokens
	if err := syncLoginUserToProfile(config.ProfileName, config.AppID, openId, userName); err != nil {
		_ = larkauth.RemoveStoredToken(config.AppID, openId)
		return errs.NewInternalError(errs.SubtypeSDKError, "failed to update login profile: %v", err).WithCause(err)
	}

	if issue := ensureRequestedScopesGranted(requestedScope, result.Token.Scope, msg, scopeSummary); issue != nil {
		return handleLoginScopeIssue(opts, msg, f, issue, openId, userName)
	}

	writeLoginSuccess(opts, msg, f, openId, userName, scopeSummary)
	return nil
}

// syncLoginUserToProfile persists the logged-in user info into the named profile.
func syncLoginUserToProfile(profileName, appID, openID, userName string) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "load config: %v", err).WithCause(err)
	}

	app := findProfileByName(multi, profileName)
	if app == nil {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "profile %q not found in config", profileName)
	}

	oldUsers := append([]core.AppUser(nil), app.Users...)
	app.Users = []core.AppUser{{UserOpenId: openID, UserName: userName}}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "save config: %v", err).WithCause(err)
	}

	for _, oldUser := range oldUsers {
		if oldUser.UserOpenId != openID {
			_ = larkauth.RemoveStoredToken(appID, oldUser.UserOpenId)
		}
	}
	return nil
}

// findProfileByName returns the AppConfig matching profileName, or nil.
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
func collectScopesForDomains(domains []string, identity string, brand core.LarkBrand) []string {
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
		if !shortcuts.IsShortcutServiceAvailable(sc.Service, brand) {
			continue
		}
		if domainSet[sc.Service] && shortcutSupportsIdentity(sc, identity) {
			for _, s := range sc.DeclaredScopesForIdentity(identity) {
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
func allKnownDomains(brand core.LarkBrand) map[string]bool {
	domains := make(map[string]bool)
	for _, p := range registry.ListFromMetaProjects() {
		if !registry.HasAuthDomain(p) {
			domains[p] = true
		}
	}
	for _, sc := range shortcuts.AllShortcuts() {
		if !shortcuts.IsShortcutServiceAvailable(sc.Service, brand) {
			continue
		}
		if !registry.HasAuthDomain(sc.Service) {
			domains[sc.Service] = true
		}
	}
	return domains
}

// sortedKnownDomains returns all valid domain names sorted alphabetically.
func sortedKnownDomains(brand core.LarkBrand) []string {
	m := allKnownDomains(brand)
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

// normalizeScopeInput accepts a user-supplied --scope value that may use
// commas, spaces, tabs, or newlines (or any mix) as separators and returns the
// canonical OAuth 2.0 wire form: a single space-joined string with empties
// trimmed and duplicates removed (first occurrence wins; order preserved).
//
// Examples:
//
//	"vc:note:read,vc:meeting.meetingevent:read" -> "vc:note:read vc:meeting.meetingevent:read"
//	"a, b ,  c"                                 -> "a b c"
//	"a b a"                                     -> "a b"
//	""                                          -> ""
func normalizeScopeInput(raw string) string {
	if raw == "" {
		return ""
	}
	// Treat both commas and any whitespace as separators.
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return ""
	}
	seen := make(map[string]struct{}, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	return strings.Join(out, " ")
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

// joinSortedScopeSet returns a deterministic, space-separated scope string
// from a set, sorted alphabetically. Empty/blank scopes are dropped.
func joinSortedScopeSet(set map[string]bool) string {
	out := make([]string, 0, len(set))
	for s := range set {
		if strings.TrimSpace(s) == "" {
			continue
		}
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// applyExcludeScopes removes the provided exclude entries from the requested
// scope string. Each --exclude flag value may itself contain comma- or
// whitespace-separated scopes. Returns the filtered scope string and any
// exclude entries that were not present in the requested set (callers can
// surface those as a validation error to catch typos like
// `--exclude drive:file:downlod`).
func applyExcludeScopes(requested string, excludes []string) (string, []string) {
	requestedSet := make(map[string]bool)
	for _, s := range strings.Fields(requested) {
		requestedSet[s] = true
	}

	excludeSet := make(map[string]bool)
	for _, raw := range excludes {
		// --exclude already splits on commas (StringSliceVar), but also
		// tolerate whitespace-separated entries inside a single value.
		for _, s := range strings.Fields(strings.ReplaceAll(raw, ",", " ")) {
			excludeSet[s] = true
		}
	}

	var unknown []string
	for s := range excludeSet {
		if !requestedSet[s] {
			unknown = append(unknown, s)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return requested, unknown
	}

	kept := make(map[string]bool, len(requestedSet))
	for s := range requestedSet {
		if !excludeSet[s] {
			kept[s] = true
		}
	}
	return joinSortedScopeSet(kept), nil
}
