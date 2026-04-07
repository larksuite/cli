// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/larksuite/cli/cmd/api"
	"github.com/larksuite/cli/cmd/auth"
	"github.com/larksuite/cli/cmd/completion"
	cmdconfig "github.com/larksuite/cli/cmd/config"
	"github.com/larksuite/cli/cmd/doctor"
	"github.com/larksuite/cli/cmd/profile"
	"github.com/larksuite/cli/cmd/schema"
	"github.com/larksuite/cli/cmd/service"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/update"
	"github.com/larksuite/cli/shortcuts"
	"github.com/spf13/cobra"
)

const rootLong = `lark-cli — Lark/Feishu CLI tool.

USAGE:
    lark-cli <command> [subcommand] [method] [options]
    lark-cli api <method> <path> [--params <json>] [--data <json>]
    lark-cli schema <service.resource.method> [--format pretty]

EXAMPLES:
    # View upcoming events
    lark-cli calendar +agenda

    # List calendar events
    lark-cli calendar events instance_view --params '{"calendar_id":"primary","start_time":"1700000000","end_time":"1700086400"}'

    # Search users
    lark-cli contact +search-user --query "John"

    # Generic API call
    lark-cli api GET /open-apis/calendar/v4/calendars

FLAGS:
    --params <json>       URL/query parameters JSON
    --data <json>         request body JSON (POST/PATCH/PUT/DELETE)
    --as <type>           identity type: user | bot | auto (default: auto)
    --format <fmt>        output format: json (default) | ndjson | table | csv | pretty
    --page-all            automatically paginate through all pages
    --page-size <N>       page size (0 = use API default)
    --page-limit <N>      max pages to fetch with --page-all (default: 10, 0 for unlimited)
    --page-delay <MS>     delay in ms between pages (default: 200, only with --page-all)
    -o, --output <path>   output file path for binary responses
    --jq <expr>           jq expression to filter JSON output
    -q <expr>             shorthand for --jq
    --dry-run             print request without executing

AI AGENT SKILLS:
    lark-cli pairs with AI agent skills (Claude Code, etc.) that
    teach the agent Lark API patterns, best practices, and workflows.

    Install all skills:
        npx skills add larksuite/cli -g -y

    Or pick specific domains:
        npx skills add larksuite/cli -s lark-calendar -y
        npx skills add larksuite/cli -s lark-im -y

    Learn more: https://github.com/larksuite/cli#install-ai-agent-skills

COMMUNITY:
    GitHub:     https://github.com/larksuite/cli
    Issues:     https://github.com/larksuite/cli/issues
    Docs:       https://open.feishu.cn/document/

More help: lark-cli <command> --help`

// Execute runs the root command and returns the process exit code.
func Execute() int {
	inv, err := BootstrapInvocationContext(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	f := cmdutil.NewDefault(inv)

	globals := &GlobalOptions{Profile: inv.Profile}
	rootCmd := &cobra.Command{
		Use:     "lark-cli",
		Short:   "Lark/Feishu CLI — OAuth authorization, UAT management, API calls",
		Long:    rootLong,
		Version: build.Version,
	}
	installTipsHelpFunc(rootCmd)
	rootCmd.SilenceErrors = true

	RegisterGlobalFlags(rootCmd.PersistentFlags(), globals)
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		cmd.SilenceUsage = true
	}

	rootCmd.AddCommand(cmdconfig.NewCmdConfig(f))
	rootCmd.AddCommand(auth.NewCmdAuth(f))
	rootCmd.AddCommand(profile.NewCmdProfile(f))
	rootCmd.AddCommand(doctor.NewCmdDoctor(f))
	rootCmd.AddCommand(api.NewCmdApi(f, nil))
	rootCmd.AddCommand(schema.NewCmdSchema(f, nil))
	rootCmd.AddCommand(completion.NewCmdCompletion(f))
	service.RegisterServiceCommands(rootCmd, f)
	shortcuts.RegisterShortcuts(rootCmd, f)

	// Prune commands incompatible with strict mode.
	if mode := f.ResolveStrictMode(context.Background()); mode.IsActive() {
		pruneForStrictMode(rootCmd, mode)
	}

	// --- Update check (non-blocking) ---
	if !isCompletionCommand(os.Args) {
		setupUpdateNotice()
	}

	if err := rootCmd.Execute(); err != nil {
		return handleRootError(f, err)
	}
	return 0
}

// setupUpdateNotice starts an async update check and wires the output decorator.
func setupUpdateNotice() {
	// Sync: check cache immediately (no network, fast).
	if info := update.CheckCached(build.Version); info != nil {
		update.SetPending(info)
	}

	// Async: refresh cache for this run (and future runs).
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "update check panic: %v\n", r)
			}
		}()
		update.RefreshCache(build.Version)
		// If cache was just populated for the first time, set pending now.
		if update.GetPending() == nil {
			if info := update.CheckCached(build.Version); info != nil {
				update.SetPending(info)
			}
		}
	}()

	// Wire the output decorator so JSON envelopes include "_notice".
	output.PendingNotice = func() map[string]interface{} {
		info := update.GetPending()
		if info == nil {
			return nil
		}
		return map[string]interface{}{
			"update": map[string]interface{}{
				"current": info.Current,
				"latest":  info.Latest,
				"message": info.Message(),
			},
		}
	}
}

// isCompletionCommand returns true if args indicate a shell completion request.
// Update notifications must be suppressed for these to avoid corrupting
// machine-parseable completion output.
func isCompletionCommand(args []string) bool {
	for _, arg := range args {
		if arg == "completion" || arg == "__complete" {
			return true
		}
	}
	return false
}

// handleRootError dispatches a command error to the appropriate handler
// and returns the process exit code.
func handleRootError(f *cmdutil.Factory, err error) int {
	errOut := f.IOStreams.ErrOut

	// SecurityPolicyError uses a custom envelope format (string codes, challenge_url, retryable)
	// that differs from the standard ErrDetail, so it's handled separately.
	var spErr *internalauth.SecurityPolicyError
	if errors.As(err, &spErr) {
		writeSecurityPolicyError(errOut, spErr)
		return 1
	}

	// All other structured errors normalize to ExitError.
	if exitErr := asExitError(err); exitErr != nil {
		if !exitErr.Raw {
			// Raw errors (e.g. from `api` command) preserve the original API
			// error detail; skip enrichment which would clear it.
			enrichPermissionError(f, exitErr)
		}
		output.WriteErrorEnvelope(errOut, exitErr, string(f.ResolvedIdentity))
		return exitErr.Code
	}

	// Cobra errors (required flags, unknown commands, etc.)
	fmt.Fprintln(errOut, "Error:", err)
	return 1
}

// asExitError converts known structured error types to *output.ExitError.
// Returns nil for unrecognized errors (e.g. cobra flag errors).
func asExitError(err error) *output.ExitError {
	var cfgErr *core.ConfigError
	if errors.As(err, &cfgErr) {
		return output.ErrWithHint(cfgErr.Code, cfgErr.Type, cfgErr.Message, cfgErr.Hint)
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return exitErr
	}
	return nil
}

// writeSecurityPolicyError writes the security-policy-specific JSON envelope to w.
// This format intentionally differs from the standard ErrDetail envelope:
// it uses string codes ("challenge_required"/"access_denied") and extra fields
// (retryable, challenge_url) for machine-readable policy error handling.
func writeSecurityPolicyError(w io.Writer, spErr *internalauth.SecurityPolicyError) {
	var codeStr string
	switch spErr.Code {
	case internalauth.LarkErrBlockByPolicyTryAuth:
		codeStr = "challenge_required"
	case internalauth.LarkErrBlockByPolicy:
		codeStr = "access_denied"
	default:
		codeStr = strconv.Itoa(spErr.Code)
	}

	errData := map[string]interface{}{
		"type":      "auth_error",
		"code":      codeStr,
		"message":   spErr.Message,
		"retryable": false,
	}
	if spErr.ChallengeURL != "" {
		errData["challenge_url"] = spErr.ChallengeURL
	}
	if spErr.CLIHint != "" {
		errData["hint"] = spErr.CLIHint
	}

	env := map[string]interface{}{"ok": false, "error": errData}

	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	err := encoder.Encode(env)

	if err != nil {
		fmt.Fprintln(w, `{"ok":false,"error":{"type":"internal_error","code":"marshal_error","message":"failed to marshal error"}}`)
		return
	}
	fmt.Fprint(w, buffer.String())
}

// installTipsHelpFunc wraps the default help function to append a TIPS section
// when a command has tips set via cmdutil.SetTips.
func installTipsHelpFunc(root *cobra.Command) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		tips := cmdutil.GetTips(cmd)
		if len(tips) == 0 {
			return
		}
		out := cmd.OutOrStdout()
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Tips:")
		for _, tip := range tips {
			fmt.Fprintf(out, "    • %s\n", tip)
		}
	})
}

// enrichPermissionError adds console_url and improves the hint for permission errors.
// It differentiates between:
//   - LarkErrAppScopeNotEnabled (99991672): app has not enabled the API scope → hint to admin console
//   - LarkErrUserScopeInsufficient (99991679): user has not authorized the scope → hint to auth login --scope
func enrichPermissionError(f *cmdutil.Factory, exitErr *output.ExitError) {
	if exitErr.Detail == nil || exitErr.Detail.Type != "permission" {
		return
	}
	// Extract required scopes from API error detail
	scopes := extractRequiredScopes(exitErr.Detail.Detail)
	if len(scopes) == 0 {
		return
	}

	cfg, err := f.Config()
	if err != nil {
		return
	}

	// Select the recommended (least-privilege) scope
	scopeIfaces := make([]interface{}, len(scopes))
	for i, s := range scopes {
		scopeIfaces[i] = s
	}
	recommended := registry.SelectRecommendedScope(scopeIfaces, "tenant")
	if recommended == "" {
		recommended = scopes[0]
	}

	// Build admin console URL with the recommended scope
	host := "open.feishu.cn"
	if cfg.Brand == "lark" {
		host = "open.larksuite.com"
	}
	consoleURL := fmt.Sprintf("https://%s/page/scope-apply?clientID=%s&scopes=%s", host, url.QueryEscape(cfg.AppID), url.QueryEscape(recommended))

	// Clear raw API detail — useful info is now in message/hint/console_url
	exitErr.Detail.Detail = nil

	isBot := f.ResolvedIdentity.IsBot()

	larkCode := exitErr.Detail.Code
	switch larkCode {
	case output.LarkErrUserScopeInsufficient, output.LarkErrUserNotAuthorized:
		// User has not authorized the scope → re-authorize
		exitErr.Detail.Message = fmt.Sprintf("User not authorized: required scope %s [%d]", recommended, larkCode)
		if isBot {
			exitErr.Detail.Hint = "enable the scope in developer console (see console_url)"
		} else {
			exitErr.Detail.Hint = fmt.Sprintf("run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", recommended)
		}
		exitErr.Detail.ConsoleURL = consoleURL

	case output.LarkErrAppScopeNotEnabled:
		// App has not enabled the API scope → admin console
		exitErr.Detail.Message = fmt.Sprintf("App scope not enabled: required scope %s [%d]", recommended, larkCode)
		exitErr.Detail.Hint = "enable the scope in developer console (see console_url)"
		exitErr.Detail.ConsoleURL = consoleURL

	default:
		// Other permission errors (matched by keyword)
		exitErr.Detail.Message = fmt.Sprintf("Permission denied: required scope %s [%d]", recommended, larkCode)
		if isBot {
			exitErr.Detail.Hint = "enable the scope in developer console (see console_url)"
		} else {
			exitErr.Detail.Hint = fmt.Sprintf(
				"enable scope in console (see console_url), or run `lark-cli auth login --scope \"%s\"` in the background. It blocks and outputs a verification URL — retrieve the URL and open it in a browser to complete login.", recommended)
		}
		exitErr.Detail.ConsoleURL = consoleURL
	}
}

// extractRequiredScopes extracts scope names from the API error's permission_violations field.
func extractRequiredScopes(detail interface{}) []string {
	m, ok := detail.(map[string]interface{})
	if !ok {
		return nil
	}
	violations, ok := m["permission_violations"].([]interface{})
	if !ok {
		return nil
	}
	var scopes []string
	for _, v := range violations {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		if subject, ok := vm["subject"].(string); ok {
			scopes = append(scopes, subject)
		}
	}
	return scopes
}
