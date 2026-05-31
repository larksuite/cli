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
	"github.com/larksuite/cli/internal/deprecation"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/errcompat"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/suggest"
	"github.com/larksuite/cli/internal/update"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const rootLong = `lark-cli — Lark/Feishu CLI tool.

USAGE:
    lark-cli <command> [subcommand] [method] [options]
    lark-cli api <method> <path> [--params <json>] [--data <json>]
    lark-cli schema <service.resource.method>

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
// rawInvocationArgs holds os.Args[1:] captured at Execute() entry. cobra's
// UnknownFlags whitelist (installUnknownSubcommandGuard) swallows unknown flags
// before they reach a group's RunE, so unknownSubcommandRunE re-derives them
// from here. It stays nil in unit tests that invoke a RunE directly with
// explicit args — correct, since those don't exercise the whitelist path.
var rawInvocationArgs []string

func Execute() int {
	rawInvocationArgs = os.Args[1:]
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
	output.PendingNotice = composePendingNotice
}

// composePendingNotice merges all process-level pending notices (available
// update, skills/binary drift, deprecated-command alias) into the map surfaced
// as the JSON "_notice" envelope field. Returns nil when nothing is pending.
// Extracted from Execute so the composition is unit-testable.
func composePendingNotice() map[string]interface{} {
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
	if dep := deprecation.GetPending(); dep != nil {
		entry := map[string]interface{}{
			"command": dep.Command,
			"message": dep.Message(),
			"action":  "lark-cli update",
		}
		if dep.Replacement != "" {
			entry["replacement"] = dep.Replacement
		}
		if dep.Skill != "" {
			entry["skill"] = dep.Skill
		}
		notice["deprecated_command"] = entry
	}
	if len(notice) == 0 {
		return nil
	}
	return notice
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

	// A backward-compat alias records its deprecation notice in PreRunE, which
	// runs before cobra's required-flag validation — but a missing required flag
	// fails before RunE and lands here, where the bare "Error:" line would drop
	// the notice. When a deprecation is pending, route through the structured
	// envelope so the migration hint still reaches the caller; all other errors
	// keep the existing plain output.
	if deprecation.GetPending() != nil {
		output.WriteErrorEnvelope(errOut, &output.ExitError{
			Code:   1,
			Detail: &output.ErrDetail{Type: "validation", Message: err.Error()},
		}, string(f.ResolvedIdentity))
		return 1
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
		// Route an unknown subcommand to unknownSubcommandRunE even when flags
		// are also present (e.g. `sheets +cells-find --url ...`). A pure group
		// consumes no flags itself, so unknown flags belong to the (missing)
		// subcommand; whitelisting them here prevents cobra from erroring on the
		// flag first and printing usage instead of our structured suggestion.
		cmd.FParseErrWhitelist.UnknownFlags = true
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
		// A bare group (e.g. `sheets`), or one carrying only group-valid flags
		// like the global --profile, legitimately prints help. But a flag that
		// belongs to a (missing) subcommand is a user error: the guard's
		// FParseErrWhitelist swallows such flags and leaves args empty, so without
		// the checks below they would silently fall through to help + exit 0 —
		// letting an agent mistake a malformed call (`im --format json`,
		// `sheets --badflag`) for success. Recover the swallowed tokens from the
		// raw invocation and fail structured instead.
		flags := flagTokensInArgs(rawInvocationArgs)
		if len(flags) == 0 {
			return cmd.Help()
		}
		if unknown := unknownFlagTokens(cmd, rawInvocationArgs); len(unknown) > 0 {
			return &output.ExitError{
				Code: output.ExitValidation,
				Detail: &output.ErrDetail{
					Type:    "unknown_flag",
					Message: fmt.Sprintf("unknown flag %s before a subcommand for %q", strings.Join(unknown, ", "), cmd.CommandPath()),
					Hint:    fmt.Sprintf("flags belong to a subcommand; run `%s --help` to list subcommands and their flags", cmd.CommandPath()),
					Detail: map[string]any{
						// Keep the same detail keys as flagDidYouMean's unknown_flag
						// so a consumer keyed on Type can read a stable shape. The
						// subcommand isn't resolved here, so suggestions/valid_flags
						// have no meaningful universe to draw from — emit empty
						// rather than the group's own (misleading) flags. unknown is
						// the back-compat singular field; unknown_flags carries the
						// full list when more than one flag was supplied.
						"unknown":       strings.Join(unknown, ", "),
						"unknown_flags": unknown,
						"command_path":  cmd.CommandPath(),
						"suggestions":   []string{},
						"valid_flags":   []string{},
					},
				},
			}
		}
		// The remaining flags are all defined somewhere in the tree. Those valid
		// on the group itself or inherited (e.g. the global --profile) do not
		// require a subcommand, so a bare group carrying only those still prints
		// help. Anything left belongs to a subcommand that was omitted
		// (e.g. `im --format json`): distinct from unknown_flag — the flags are
		// real, the subcommand is what's missing.
		misplaced := subcommandOnlyFlagTokens(cmd, rawInvocationArgs)
		if len(misplaced) == 0 {
			return cmd.Help()
		}
		return &output.ExitError{
			Code: output.ExitValidation,
			Detail: &output.ErrDetail{
				Type:    "missing_subcommand",
				Message: fmt.Sprintf("missing subcommand for %q; flag %s belongs to a subcommand, not the group", cmd.CommandPath(), strings.Join(misplaced, ", ")),
				Hint:    fmt.Sprintf("run `%s --help` to list subcommands and their flags", cmd.CommandPath()),
				Detail: map[string]any{
					"command_path": cmd.CommandPath(),
					"flags":        misplaced,
					"suggestions":  []string{},
				},
			},
		}
	}
	unknown := args[0]
	available, deprecated := availableSubcommandNames(cmd)
	// Rank suggestions across both current and deprecated names so a mistyped
	// legacy command (e.g. +raed → +read) still resolves; the alias stays
	// runnable and self-flags via the _notice on execution.
	suggestions := suggest.Closest(unknown, append(append([]string{}, available...), deprecated...), 6)
	msg := fmt.Sprintf("unknown subcommand %q for %q", unknown, cmd.CommandPath())
	hint := fmt.Sprintf("run `%s --help` to see available subcommands", cmd.CommandPath())
	if len(suggestions) > 0 {
		hint = fmt.Sprintf("did you mean one of: %s? (run `%s --help` for the full list)",
			strings.Join(suggestions, ", "), cmd.CommandPath())
	}
	detail := map[string]any{
		"unknown":      unknown,
		"command_path": cmd.CommandPath(),
		"suggestions":  suggestions,
		"available":    available,
	}
	// Only services with backward-compat aliases (currently sheets) carry a
	// deprecated bucket; omit the key elsewhere so every other service's
	// envelope is unchanged.
	if len(deprecated) > 0 {
		detail["deprecated"] = deprecated
	}
	return &output.ExitError{
		Code: output.ExitValidation,
		Detail: &output.ErrDetail{
			Type:    "unknown_subcommand",
			Message: msg,
			Hint:    hint,
			Detail:  detail,
		},
	}
}

// flagTokensInArgs returns the flag-like tokens (-x, --foo, --foo=bar) in
// rawArgs, stopping at the "--" positional terminator. Whether a flag is
// defined is not considered (see unknownFlagTokens for that). A pure group
// with any flag token but no subcommand is a user error — a pure group
// consumes no flags of its own, so the flag must belong to a subcommand — so
// the caller fails structured instead of falling through to help.
func flagTokensInArgs(rawArgs []string) []string {
	var toks []string
	for _, a := range rawArgs {
		if a == "--" {
			break // everything after -- is positional
		}
		if len(a) < 2 || a[0] != '-' {
			continue
		}
		toks = append(toks, a)
	}
	return toks
}

// unknownFlagTokens returns the flag tokens in rawArgs that cmd does not define
// (on itself, inherited, or any direct subcommand). installUnknownSubcommandGuard
// whitelists unknown flags on pure groups so a mistyped subcommand still reaches
// the suggestion path; the side effect is that flags before a subcommand are
// swallowed. This recovers the genuinely-unknown ones so the caller can name
// them in a "did you mean" envelope.
func unknownFlagTokens(cmd *cobra.Command, rawArgs []string) []string {
	var unknown []string
	for _, a := range flagTokensInArgs(rawArgs) {
		name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
		if name != "" && !flagDefinedInTree(cmd, name) {
			unknown = append(unknown, a)
		}
	}
	return unknown
}

// flagKnownOnGroup reports whether name is a flag defined on cmd itself or
// inherited (a global persistent flag like --profile) — i.e. valid on the bare
// group and therefore not requiring a subcommand.
func flagKnownOnGroup(cmd *cobra.Command, name string) bool {
	short := len(name) == 1
	lookup := func(fs *pflag.FlagSet) bool {
		if short {
			return fs.ShorthandLookup(name) != nil
		}
		return fs.Lookup(name) != nil
	}
	return lookup(cmd.Flags()) || lookup(cmd.InheritedFlags())
}

// subcommandOnlyFlagTokens returns the flag tokens in rawArgs that are valid on
// a subcommand of cmd but not on cmd itself/inherited — flags supplied while
// omitting the subcommand they belong to (`im --format json`). Global flags
// valid on the bare group (e.g. --profile) are excluded so
// `lark-cli --profile p im` still prints help rather than erroring.
func subcommandOnlyFlagTokens(cmd *cobra.Command, rawArgs []string) []string {
	var misplaced []string
	for _, a := range flagTokensInArgs(rawArgs) {
		name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
		if name == "" || flagKnownOnGroup(cmd, name) {
			continue
		}
		if flagDefinedInTree(cmd, name) {
			misplaced = append(misplaced, a)
		}
	}
	return misplaced
}

// flagDefinedInTree reports whether name is defined on cmd, its inherited
// (persistent) flags, or any direct subcommand. The subcommand case covers a
// user who merely omitted the subcommand — e.g. `sheets --format json`, where
// --format is injected on every leaf shortcut, not on the group — so only a
// genuinely unknown flag like `sheets --badflag` is reported.
func flagDefinedInTree(cmd *cobra.Command, name string) bool {
	short := len(name) == 1
	known := func(c *cobra.Command, inherited bool) bool {
		fs := c.Flags()
		if inherited {
			fs = c.InheritedFlags()
		}
		if short {
			return fs.ShorthandLookup(name) != nil
		}
		return fs.Lookup(name) != nil
	}
	if known(cmd, false) || known(cmd, true) {
		return true
	}
	for _, c := range cmd.Commands() {
		if known(c, false) {
			return true
		}
	}
	return false
}

// availableSubcommandNames returns the invokable subcommand names of cmd, split
// into current commands and backward-compatibility aliases (those tagged into
// the deprecated cobra group via cmdutil.DeprecatedGroupID). Both slices are
// sorted; hidden commands plus help/completion are omitted.
func availableSubcommandNames(cmd *cobra.Command) (available, deprecated []string) {
	for _, c := range cmd.Commands() {
		if c.Hidden || !c.IsAvailableCommand() {
			continue
		}
		name := c.Name()
		if name == "help" || name == "completion" {
			continue
		}
		if cmdutil.IsDeprecatedCommand(c) {
			deprecated = append(deprecated, name)
		} else {
			available = append(available, name)
		}
	}
	sort.Strings(available)
	sort.Strings(deprecated)
	return available, deprecated
}

// flagDidYouMean is the root FlagErrorFunc (inherited by all subcommands). It
// converts cobra's flag-parse errors into the structured ErrorEnvelope: an
// unknown flag gets a focused "did you mean" hint plus the full valid-flag list
// in detail (so agents recover even when the typo is semantic, e.g. --query vs
// --find, where edit distance alone finds nothing). Other flag errors stay
// structured but generic.
func flagDidYouMean(c *cobra.Command, ferr error) error {
	name, isUnknown := unknownFlagName(ferr)
	if !isUnknown {
		return &output.ExitError{
			Code: output.ExitValidation,
			Detail: &output.ErrDetail{
				Type:    "flag_error",
				Message: ferr.Error(),
				Hint:    fmt.Sprintf("run `%s --help` for valid flags", c.CommandPath()),
			},
		}
	}
	valid := visibleFlagNames(c)
	suggestions := suggest.Closest(name, valid, 3)
	hint := fmt.Sprintf("run `%s --help` to see valid flags", c.CommandPath())
	if len(suggestions) > 0 {
		for i := range suggestions {
			suggestions[i] = "--" + suggestions[i]
		}
		hint = fmt.Sprintf("did you mean %s? (run `%s --help` for all flags)",
			strings.Join(suggestions, ", "), c.CommandPath())
	}
	return &output.ExitError{
		Code: output.ExitValidation,
		Detail: &output.ErrDetail{
			Type:    "unknown_flag",
			Message: fmt.Sprintf("unknown flag %q for %q", "--"+name, c.CommandPath()),
			Hint:    hint,
			Detail: map[string]any{
				"unknown":      "--" + name,
				"command_path": c.CommandPath(),
				"suggestions":  suggestions,
				"valid_flags":  valid,
			},
		},
	}
}

// unknownFlagName extracts the offending long-flag name from cobra's flag-parse
// error text ("unknown flag: --query" → "query"). Returns ok=false for anything
// else (missing argument, invalid value, unknown shorthand) so the caller keeps
// those structured but generic — hallucinated flags are essentially always long.
//
// CONTRACT: this matches cobra's English wording "unknown flag: --" (go.mod
// pins github.com/spf13/cobra). If cobra rewords this or gains i18n the match
// silently fails and unknown flags degrade to a generic flag_error — re-verify
// this prefix when bumping cobra.
func unknownFlagName(err error) (string, bool) {
	const p = "unknown flag: --"
	msg := err.Error()
	i := strings.Index(msg, p)
	if i < 0 {
		return "", false
	}
	rest := msg[i+len(p):]
	if j := strings.IndexAny(rest, " \t"); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

// visibleFlagNames lists the non-hidden flag names of c (for suggestions and
// the valid_flags detail).
func visibleFlagNames(c *cobra.Command) []string {
	var names []string
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if !f.Hidden {
			names = append(names, f.Name)
		}
	})
	sort.Strings(names)
	return names
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
