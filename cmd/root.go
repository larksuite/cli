// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	internalauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/errcompat"
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
//
// Dispatch order:
//  1. Legacy shapes (*core.ConfigError, *internalauth.NeedAuthorizationError)
//     are promoted via errcompat to their typed errs/ counterparts, with the
//     original preserved in the Cause chain.
//  2. Typed errors from errs/ (e.g. *errs.PermissionError, *errs.APIError,
//     *errs.SecurityPolicyError, *errs.AuthenticationError): render via the
//     typed envelope writer, which lifts extension fields (missing_scopes,
//     console_url, challenge_url, ...) to the top level. Routed by
//     errs.CategoryOf via ExitCodeOf.
//  3. Legacy *output.ExitError: asExitError adapts it to the legacy
//     envelope, written via WriteErrorEnvelope.
//  4. Cobra errors (required flags, unknown commands, etc.): plain text.
func handleRootError(f *cmdutil.Factory, err error) int {
	errOut := f.IOStreams.ErrOut

	// Promote legacy error shapes into typed errs/ before envelope marshal.
	// NeedAuthorizationError check is first because it is the more specific
	// shape; *core.ConfigError check follows. errors.As preserves the original
	// in the Cause chain, so external errors.As(&core.ConfigError{}) consumers
	// (cmd/auth/list.go, cmd/doctor/doctor.go, ...) still match.
	//
	// Outer-typed short-circuit: if err is already a typed *errs.* error,
	// skip PromoteXxxError so the producer's Subtype / Hint / extension
	// fields are not overwritten by a coarser promoted shape derived from a
	// legacy error buried in its Cause chain. Promotion is only for legacy
	// untyped entry points.
	if !isOuterTypedError(err) {
		var needAuthErr *internalauth.NeedAuthorizationError
		if errors.As(err, &needAuthErr) {
			err = errcompat.PromoteAuthError(needAuthErr)
		} else {
			var cfgErr *core.ConfigError
			if errors.As(err, &cfgErr) {
				err = errcompat.PromoteConfigError(cfgErr)
			}
		}
	}

	// When the typed error is a need_user_authorization signal, fold in the
	// current command's declared scopes as a Hint so the user/AI sees the
	// concrete scope(s) to re-auth with. The hint is computed on the fly from
	// local shortcut/service metadata — it never depends on server state.
	applyNeedAuthorizationHint(f, err)

	// Staged dispatch: capture the typed exit code BEFORE attempting the
	// envelope write. WriteTypedErrorEnvelope is best-effort on the wire
	// (partial-write still returns true) so the exit code we read here is
	// preserved even if stderr is torn — torn stderr must not downgrade
	// typed exits 3/4/6/10 to the legacy "Error:" path with exit 1.
	// WriteTypedErrorEnvelope still returns false when err carries no
	// Problem; in that case we fall through to the legacy bridge below.
	typedExit := output.ExitCodeOf(err)
	if output.WriteTypedErrorEnvelope(errOut, err, string(f.ResolvedIdentity)) {
		return typedExit
	}

	// Partial-failure (batch / multi-status): the ok:false result envelope is
	// already on stdout; set the exit code and write nothing to stderr.
	var pfErr *output.PartialFailureError
	if errors.As(err, &pfErr) {
		return pfErr.Code
	}

	if exitErr := asExitError(err); exitErr != nil {
		if !exitErr.Raw {
			// Raw errors (e.g. from `api` command via output.MarkRaw)
			// preserve the original API error detail; skip enrichment
			// which would clear it.
			enrichMissingScopeError(f, exitErr)
			enrichPermissionError(f, exitErr)
		}
		output.WriteErrorEnvelope(errOut, exitErr, string(f.ResolvedIdentity))
		return exitErr.Code
	}

	fmt.Fprintln(errOut, "Error:", err)
	return 1
}

// isOuterTypedError returns true if err is a typed *errs.* error AT THE
// TOP OF THE CHAIN (not buried inside Unwrap). Used by handleRootError
// to gate PromoteXxxError so a producer's outer typed envelope is never
// overwritten by a coarser shape derived from its legacy Cause.
func isOuterTypedError(err error) bool {
	_, ok := err.(errs.TypedError)
	return ok
}

// asExitError converts known structured error types to *output.ExitError.
// Returns nil for unrecognized errors (e.g. cobra flag errors).
//
// Deprecated: legacy *output.ExitError bridge.
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

// Deprecated: unknownSubcommandRunE produces a legacy *output.ExitError that
// predates the typed error contract introduced by errs/. New code MUST NOT
// add producers of this shape — unknown-subcommand signals should move to
// a typed *errs.ValidationError (or a dedicated typed error) carrying the
// agent-protocol metadata as typed extension fields. This helper is retained
// only while existing dispatch sites are migrated; it will be removed once
// they have moved to the typed surface.
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

// enrichPermissionError rewrites the legacy *output.ExitError envelope so its
// Message + Hint match the per-subtype canonical text produced by the typed
// dispatcher path (errclass.CanonicalPermissionMessage / errclass.PermissionHint).
// This guarantees a caller observing the wire envelope cannot tell whether
// the error reached the dispatcher via the legacy *ExitError bridge or via
// the typed *errs.PermissionError fast path.
//
// Deprecated: legacy *output.ExitError enrichment; typed PermissionError
// values produced by errclass.BuildAPIError already carry MissingScopes +
// ConsoleURL directly.
func enrichPermissionError(f *cmdutil.Factory, exitErr *output.ExitError) {
	if exitErr.Detail == nil {
		return
	}
	// Only the legacy permission-class envelope types route here. "app_status"
	// covers 99991662 (app_disabled) / 99991673 (app_unavailable); "permission"
	// covers the four scope-class codes (99991672 / 99991676 / 99991679 / 230027).
	if exitErr.Detail.Type != "permission" && exitErr.Detail.Type != "app_status" {
		return
	}

	larkCode := exitErr.Detail.Code
	meta, ok := errclass.LookupCodeMeta(larkCode)
	if !ok || meta.Category != errs.CategoryAuthorization {
		return
	}

	// Extract required scopes from API error detail (shared helper). May be
	// empty for app-status codes — canonical message + hint still apply.
	missing := registry.ExtractRequiredScopes(exitErr.Detail.Detail)

	cfg, err := f.Config()
	if err != nil {
		return
	}

	// Reuse the same console URL builder as the typed path so both wire
	// envelopes carry identical console_url values for the same input.
	consoleURL := errclass.ConsoleURL(string(cfg.Brand), cfg.AppID, missing)

	// Clear raw API detail — useful info is now in message/hint/console_url.
	exitErr.Detail.Detail = nil

	identity := string(f.ResolvedIdentity)
	if identity == "" {
		identity = "user"
	}

	exitErr.Detail.Message = errclass.CanonicalPermissionMessage(meta.Subtype, cfg.AppID, missing, exitErr.Detail.Message)
	exitErr.Detail.Hint = errclass.PermissionHint(missing, identity, meta.Subtype, consoleURL)
	exitErr.Detail.ConsoleURL = consoleURL
}
