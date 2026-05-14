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
browser. Run it in the background and retrieve the verification URL from its output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode := f.ResolveStrictMode(cmd.Context()); mode == core.StrictModeBot {
				return output.ErrWithHint(output.ExitValidation, "strict_mode",
					fmt.Sprintf("strict mode is %q, user login is disabled in this profile", mode),
					"if the user explicitly wants to switch to user identity, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)")
			}
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLoginRun(opts)
		},
	}
	cmdutil.SetSupportedIdentities(cmd, []string{"user"})

	cmd.Flags().StringVar(&opts.Scope, "scope", "", "scopes to request (space- or comma-separated). Combines additively with --domain/--recommend")
	cmd.Flags().BoolVar(&opts.Recommend, "recommend", false, "request only recommended (auto-approve) scopes")
	available := sortedKnownDomains()
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

	if len(opts.Exclude) > 0 && !hasAnyOption {
		return output.ErrValidation("--exclude requires --scope, --domain, or --recommend to be specified")
	}

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
			candidateScopes = collectScopesForDomains(selectedDomains, "user")
		} else {
			// --recommend without --domain: all domains
			candidateScopes = collectScopesForDomains(sortedKnownDomains(), "user")
		}

		// Filter to auto-approve scopes if --recommend or interactive "common"
		if opts.Recommend || scopeLevel == "common" {
			candidateScopes = registry.FilterAutoApproveScopes(candidateScopes)
		}

		if len(candidateScopes) == 0 && opts.Scope == "" {
			return output.ErrValidation("no matching scopes found, check domain/scope options")
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
			return output.ErrValidation(
				"these --exclude scopes are not present in the requested set: %s",
				strings.Join(unknown, ", "))
		}
		finalScope = excluded
		if strings.TrimSpace(finalScope) == "" {
			return output.ErrValidation("no scopes left after applying --exclude; nothing to authorize")
		}
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
			"hint":             fmt.Sprintf("Show verification_url to the user exactly as returned by the CLI and treat it as an opaque string. Do not URL-encode or decode it, do not normalize or rewrite it, do not add %%20, spaces, or punctuation, and do not wrap it as Markdown link text; prefer a fenced code block containing only the raw URL. Then immediately execute: lark-cli auth login --device-code %s (blocks until authorized or timeout). Do not instruct the user to run this command themselves.", authResp.DeviceCode),
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(data); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "failed to write JSON output: %v", err)
		}
		return nil
	}

	// Step 2: Show user code and verification URL.
	// Both branches surface AgentTimeoutHint, but on different channels:
	// JSON mode embeds it as a structured field (so an agent that captures
	// stdout into a JSON parser sees it without stream-mixing surprises),
	// text mode prints to stderr (alongside the URL prompt).
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
			return output.Errorf(output.ExitInternal, "internal", "failed to write JSON output: %v", err)
		}
	} else {
		fmt.Fprintf(f.IOStreams.ErrOut, msg.OpenURL)
		fmt.Fprintf(f.IOStreams.ErrOut, "  %s\n\n", authResp.VerificationUriComplete)
		fmt.Fprintln(f.IOStreams.ErrOut, msg.AgentTimeoutHint)
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
	// Skip the stderr hint in JSON mode — the --no-wait call that issued the
	// device_code already returned the hint as a JSON field, and writing
	// text to stderr would pollute consumers that combine streams via 2>&1.
	if !opts.JSON {
		fmt.Fprintln(f.IOStreams.ErrOut, msg.AgentTimeoutHint)
	}
	log(msg.WaitingAuth)
	result := pollDeviceToken(opts.Ctx, httpClient, config.AppID, config.AppSecret, config.Brand,
		opts.DeviceCode, 5, 600, f.IOStreams.ErrOut)

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
