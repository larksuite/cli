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
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/extension/platform"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/update"
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
    --as <type>           identity type: user | bot
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

    Learn more: https://github.com/larksuite/cli#agent-skills

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
	configureFlagCompletions(os.Args)

	ctx := context.Background()
	f, rootCmd, reg := buildInternal(
		ctx, inv,
		WithIO(os.Stdin, os.Stdout, os.Stderr),
		HideProfile(isSingleAppMode()),
	)

	// --- Notices (non-blocking) ---
	if !isCompletionCommand(os.Args) {
		setupNotices()
	}

	runErr := rootCmd.Execute()

	// Fire Shutdown lifecycle hooks regardless of run outcome.
	// emitShutdown imposes a 2s total deadline and never propagates handler
	// errors (Emit's documented Shutdown contract), so it cannot block exit
	// or alter the user-visible exit code.
	if reg != nil && !isCompletionCommand(os.Args) {
		_ = hook.Emit(ctx, reg, platform.Shutdown, runErr)
	}

	if runErr != nil {
		return handleRootError(f, runErr)
	}
	return 0
}

// setupNotices wires both the binary update notice and the skills
// staleness notice into output.PendingNotice as a composed function.
// Each provider populates an independent key under _notice; either
// or both may be present in any given envelope.
func setupNotices() {
	// Binary update — synchronous cache check + async refresh
	if info := update.CheckCached(build.Version); info != nil {
		update.SetPending(info)
	}
	ver := build.Version
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "update check panic: %v\n", r)
			}
		}()
		update.RefreshCache(ver)
		if update.GetPending() == nil {
			if info := update.CheckCached(ver); info != nil {
				update.SetPending(info)
			}
		}
	}()

	// Skills check — synchronous, local-only (no network, no goroutine).
	skillscheck.Init(build.Version)

	// Composed notice provider — emits keys only when each pending is set.
	output.PendingNotice = func() map[string]interface{} {
		notice := map[string]interface{}{}
		if info := update.GetPending(); info != nil {
			notice["update"] = map[string]interface{}{
				"current": info.Current,
				"latest":  info.Latest,
				"message": info.Message(),
				"command": "lark-cli update",
			}
		}
		if stale := skillscheck.GetPending(); stale != nil {
			notice["skills"] = map[string]interface{}{
				"current": stale.Current,
				"target":  stale.Target,
				"message": stale.Message(),
				"command": "lark-cli update",
			}
		}
		if len(notice) == 0 {
			return nil
		}
		return notice
	}
}

// isCompletionCommand returns true if args indicate a shell completion request.
// Update notifications and Shutdown lifecycle emits must be suppressed for
// these to avoid corrupting machine-parseable completion output and to avoid
// firing plugin Shutdown handlers on every Tab keystroke.
//
// Cobra dispatches BOTH "__complete" and its alias "__completeNoDesc" through
// the same hidden subcommand (see cobra/completions.go ShellCompRequestCmd /
// ShellCompNoDescRequestCmd). Check both, otherwise bash/zsh completion
// (which often uses NoDesc) silently bypasses the gate.
func isCompletionCommand(args []string) bool {
	for _, arg := range args {
		if arg == "completion" || arg == "__complete" || arg == "__completeNoDesc" {
			return true
		}
	}
	return false
}

// configureFlagCompletions enables cmdutil.RegisterFlagCompletion only when
// the invocation will actually serve a __complete request.
func configureFlagCompletions(args []string) {
	cmdutil.SetFlagCompletionsEnabled(isCompletionCommand(args))
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
			enrichMissingScopeError(f, exitErr)
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

// installUnknownSubcommandGuard replaces cobra's silent help fallback on
// group commands (no Run/RunE) with an unknown_subcommand error.
//
// IMPORTANT: every command modified here is also tagged with
// cmdpolicy.AnnotationPureGroup so the user-layer policy engine
// continues to treat the command as a pure parent group. Without the
// tag, the RunE injection here would flip Runnable()=true and a user
// rule like `max_risk: read` would deny every `<group> --help` call
// with reason_code=risk_not_annotated.
func installUnknownSubcommandGuard(cmd *cobra.Command) {
	if cmd.HasSubCommands() && cmd.Run == nil && cmd.RunE == nil {
		cmd.RunE = unknownSubcommandRunE
		if cmd.Annotations == nil {
			cmd.Annotations = map[string]string{}
		}
		cmd.Annotations[cmdpolicy.AnnotationPureGroup] = "true"
	}
	for _, c := range cmd.Commands() {
		installUnknownSubcommandGuard(c)
	}
}

func unknownSubcommandRunE(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	unknown := args[0]
	available := availableSubcommandNames(cmd)
	msg := fmt.Sprintf("unknown subcommand %q for %q", unknown, cmd.CommandPath())
	hint := fmt.Sprintf("run `%s --help` to see available subcommands", cmd.CommandPath())
	if len(available) > 0 {
		hint = fmt.Sprintf("available subcommands: %s", strings.Join(available, ", "))
	}
	return &output.ExitError{
		Code: output.ExitValidation,
		Detail: &output.ErrDetail{
			Type:    "unknown_subcommand",
			Message: msg,
			Hint:    hint,
			Detail: map[string]any{
				"unknown":      unknown,
				"command_path": cmd.CommandPath(),
				"available":    available,
			},
		},
	}
}

func availableSubcommandNames(cmd *cobra.Command) []string {
	subs := make([]string, 0, len(cmd.Commands()))
	for _, c := range cmd.Commands() {
		if c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		name := c.Name()
		if name == "help" || name == "completion" {
			continue
		}
		subs = append(subs, name)
	}
	sort.Strings(subs)
	return subs
}

// installTipsHelpFunc wraps the default help function to append a TIPS section
// when a command has tips set via cmdutil.SetTips. It also force-shows global
// flags that are normally hidden in single-app mode (currently --profile)
// when rendering the root command's own help, so users discovering the CLI
// still see them at `lark-cli --help`.
func installTipsHelpFunc(root *cobra.Command) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == root {
			if f := root.PersistentFlags().Lookup("profile"); f != nil && f.Hidden {
				f.Hidden = false
				defer func() { f.Hidden = true }()
			}
		}
		defaultHelp(cmd, args)
		out := cmd.OutOrStdout()
		if level, ok := cmdutil.GetRisk(cmd); ok {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Risk:", level)
		}
		tips := cmdutil.GetTips(cmd)
		if len(tips) == 0 {
			return
		}
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
	// Extract required scopes from API error detail (shared helper)
	scopes := registry.ExtractRequiredScopes(exitErr.Detail.Detail)
	if len(scopes) == 0 {
		return
	}

	cfg, err := f.Config()
	if err != nil {
		return
	}

	// Select the recommended (least-privilege) scope
	recommended := registry.SelectRecommendedScopeFromStrings(scopes, "tenant")

	// Build admin console URL with the recommended scope
	consoleURL := registry.BuildConsoleScopeURL(cfg.Brand, cfg.AppID, recommended)

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
