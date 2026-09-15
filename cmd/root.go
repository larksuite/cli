// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"

	"github.com/larksuite/cli/cmd/service"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/deprecation"
	"github.com/larksuite/cli/internal/flagalias"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/skillscheck"
	"github.com/larksuite/cli/internal/suggest"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/internal/update"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Execute runs the root command and returns the process exit code.
// rawInvocationArgs holds os.Args[1:] captured at Execute() entry. cobra's
// UnknownFlags whitelist (installUnknownSubcommandGuard) swallows unknown flags
// before they reach a group's RunE, so unknownSubcommandRunE re-derives them
// from here. It stays nil in unit tests that invoke a RunE directly with
// explicit args — correct, since those don't exercise the whitelist path.
var rawInvocationArgs []string

// pendingHelpRejection carries a help-path rejection back to executeWithOptions.
// cobra's HelpFunc has no error return, so a `--help` that named an unknown
// subcommand records its typed error here instead of rendering help; Execute
// then fails structured on it exactly as the non---help path does. Reset at
// Execute() entry alongside rawInvocationArgs.
var pendingHelpRejection error

func Execute() int {
	return executeWithOptions(nil)
}

// ExecuteWithOptions is the standard entrypoint for wrapper distributions that
// need host-level Build options such as ConcealRestrictedCommands. Execute
// intentionally keeps its original non-variadic signature for source
// compatibility with callers that store it as a func() int value.
func ExecuteWithOptions(opts ...BuildOption) int {
	return executeWithOptions(opts)
}

func executeWithOptions(opts []BuildOption) int {
	rawInvocationArgs = os.Args[1:]
	pendingHelpRejection = nil
	inv, bootstrapErr := BootstrapInvocationContext(os.Args[1:])
	cfg := &buildConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}
	deferProfileError := cfg.presentation.enabled &&
		isDeferredBootstrapProfileError(bootstrapErr)
	if bootstrapErr != nil && !deferProfileError {
		fmt.Fprintln(os.Stderr, "Error:", bootstrapErr)
		return 1
	}
	if cfg.streams == nil {
		WithIO(os.Stdin, os.Stdout, os.Stderr)(cfg)
	}
	if !cfg.hideProfileSet {
		HideProfile(isSingleAppMode())(cfg)
	}
	configureFlagCompletions(os.Args)

	ctx, stopSignals := newExecutionContext(context.Background())
	defer stopSignals()
	if deferProfileError {
		cfg.deferStartup = true
	}
	result, buildErr := buildForArgsWithConfig(ctx, inv, rawInvocationArgs, cfg)
	if buildErr != nil {
		result = failedCatalogBuild(ctx, inv, cfg, buildErr)
	}
	runtime, rootCmd, reg := result.runtime, result.root, result.registry
	rootCmd.SetArgs(append([]string(nil), rawInvocationArgs...))
	f := runtime.Factory

	if deferProfileError {
		if runtime.surface.CanReference(surface.CommandProfile) {
			// The completed distribution still ships --profile. Replay the
			// exact pre-Build legacy failure and do not emit Startup, notices,
			// or Shutdown for an invocation that never passed bootstrap.
			fmt.Fprintln(os.Stderr, "Error:", bootstrapErr)
			return 1
		}
		if reg != nil {
			if err := emitStartup(ctx, reg); err != nil {
				installPluginLifecycleErrorGuard(rootCmd, err)
				reg = nil
			}
		}
	}

	// --- Notices (non-blocking) ---
	if !isCompletionCommand(os.Args) {
		setupNotices(runtime.surface)
	}

	runErr := rootCmd.Execute()
	if runErr == nil && pendingHelpRejection != nil {
		// cobra's ErrHelp path returns nil from Execute, so a `--help` that named
		// an unknown subcommand would otherwise exit 0 having printed the parent's
		// help. Surface the rejection the help func recorded.
		runErr = pendingHelpRejection
	}

	// Fire Shutdown lifecycle hooks regardless of run outcome.
	// emitShutdown imposes a 2s total deadline and never propagates handler
	// errors (Emit's documented Shutdown contract), so it cannot block exit
	// or alter the user-visible exit code.
	if reg != nil && !isCompletionCommand(os.Args) {
		_ = hook.Emit(ctx, reg, platform.Shutdown, runErr)
	}

	if runErr != nil {
		return handleRootError(f, rootUnknownCommandRewrite(rootCmd, runErr), runtime.recovery)
	}
	return 0
}

func newExecutionContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt)
}

// isDeferredBootstrapProfileError identifies the one bootstrap parse failure
// an explicitly concealed distribution may need the completed tree to render.
// Default and legacy builds never defer it.
func isDeferredBootstrapProfileError(err error) bool {
	return err != nil && err.Error() == "flag needs an argument: --profile"
}

// Notice provider seams keep the "concealed update means no cache, network, or
// skills-state access" contract directly testable. Production always uses the
// concrete implementations below.
var (
	checkCachedUpdate     = update.CheckCached
	refreshUpdateCache    = update.RefreshCache
	initializeSkillsCheck = skillscheck.Init
)

// setupNotices wires both the binary update notice and the skills
// staleness notice into output.PendingNotice as a composed function.
// Each provider populates an independent key under _notice; either
// or both may be present in any given envelope.
func setupNotices(plan *surface.Plan) {
	if plan.CanReference(surface.CommandUpdate) {
		// Binary update — synchronous cache check + async refresh.
		if info := checkCachedUpdate(build.Version); info != nil {
			update.SetPending(info)
		}
		ver := build.Version
		go func() {
			defer func() {
				if r := recover(); r != nil {
					fmt.Fprintf(os.Stderr, "update check panic: %v\n", r)
				}
			}()
			refreshUpdateCache(ver)
			if update.GetPending() == nil {
				if info := checkCachedUpdate(ver); info != nil {
					update.SetPending(info)
				}
			}
		}()

		// Skills drift has only one recovery action: lark-cli update. Do not
		// even inspect local drift state when that action is absent.
		initializeSkillsCheck(build.Version)
	}

	// Capture this build's immutable plan; never consult another Build's state.
	output.PendingNotice = func() map[string]interface{} {
		return composePendingNotice(plan)
	}
}

// composePendingNotice merges all process-level pending notices (available
// update, skills/binary drift, deprecated-command alias) into the map surfaced
// as the JSON "_notice" envelope field. Returns nil when nothing is pending.
// Extracted from Execute so the composition is unit-testable.
func composePendingNotice(plan *surface.Plan) map[string]interface{} {
	notice := map[string]interface{}{}
	canUpdate := plan.CanReference(surface.CommandUpdate)
	// Update and skills-drift notices have no recovery path of their own:
	// both exist solely to steer the caller to `lark-cli update`.
	if canUpdate {
		if info := update.GetPending(); info != nil {
			notice["update"] = map[string]interface{}{
				"current": info.Current,
				"latest":  info.Latest,
				"message": info.Message(),
				"command": "lark-cli update",
			}
		}
		if stale := skillscheck.GetPending(); stale != nil {
			entry := map[string]interface{}{
				"current": stale.Current,
				"target":  stale.Target,
				"message": stale.Message(),
				"command": "lark-cli update",
			}
			if stale.OfficialUnknown {
				entry["official_unknown"] = true
			}
			notice["skills"] = entry
		}
	}
	if dep := deprecation.GetPending(); dep != nil {
		entry := map[string]interface{}{
			"command": dep.Command,
			"message": dep.MessageWithoutUpdateAction(),
		}
		if canUpdate {
			entry["message"] = dep.Message()
			entry["action"] = "lark-cli update"
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
//  1. Typed errors from errs/ (e.g. *errs.PermissionError, *errs.APIError,
//     *errs.SecurityPolicyError, *errs.AuthenticationError, *errs.ConfigError):
//     render via the typed envelope writer, which lifts extension fields
//     (missing_scopes, console_url, challenge_url, ...) to the top level.
//     Routed by errs.CategoryOf via ExitCodeOf. Auth and config errors are
//     constructed typed at their origin (internal/auth, internal/core), so the
//     dispatcher no longer promotes any legacy shape here.
//  2. PartialFailure / BareError signals: the result envelope is already on
//     stdout; honor the exit code and write nothing to stderr.
//  3. Residual cobra usage errors (missing required flag, unknown command,
//     argument validation): typed as an invalid_argument envelope (exit 2),
//     matching the explicit flag/subcommand guards. Flag parse errors are
//     already typed upstream by the root FlagErrorFunc.
func handleRootError(
	f *cmdutil.Factory,
	err error,
	projector *recovery.Projector,
) int {
	errOut := f.IOStreams.ErrOut
	renderedErr := err

	// When the typed error is a need_user_authorization signal, fold in the
	// current command's declared scopes as a Hint so the user/AI sees the
	// concrete scope(s) to re-auth with. The hint is computed on the fly from
	// local shortcut/service metadata. Both semantic recovery filtering and
	// dynamic enrichment operate on a concrete clone, never the producer's
	// reusable error value.
	if !errs.IsRaw(err) {
		renderedErr = presentRootError(f, err, projector)
	}

	// Staged dispatch: capture the typed exit code BEFORE attempting the
	// envelope write. WriteTypedErrorEnvelope is best-effort on the wire
	// (partial-write still returns true) so the exit code we read here is
	// preserved even if stderr is torn — torn stderr must not downgrade
	// typed exits 3/4/6/10 to the plain "Error:" path with exit 1.
	// WriteTypedErrorEnvelope still returns false when err carries no
	// Problem; in that case we fall through to the signal / plain-text paths.
	typedExit := output.ExitCodeOf(err)
	if output.WriteTypedErrorEnvelope(errOut, renderedErr, string(f.ResolvedIdentity)) {
		return typedExit
	}

	// Partial-failure (batch / multi-status): the ok:false result envelope is
	// already on stdout; set the exit code and write nothing to stderr.
	var pfErr *output.PartialFailureError
	if errors.As(err, &pfErr) {
		return pfErr.Code
	}

	// Silent-exit signal (e.g. `auth check` predicate, or `update --json`):
	// stdout already carries the result; honor the requested exit code and
	// write nothing to stderr.
	var bareErr *output.BareError
	if errors.As(err, &bareErr) {
		return bareErr.Code
	}

	// Errors reaching here are untyped: every RunE returns a typed errs.* error
	// and flag-parse errors are typed by the root FlagErrorFunc. The remainder
	// is either a cobra usage mistake (missing required flag, unknown command,
	// wrong arg count), which cobra surfaces as a plain error identified by its
	// stable text — the same external contract unknownFlagName relies on — or an
	// untyped error that leaked past the typed boundary. Classify the former as
	// invalid_argument (exit 2, like the explicit guards); treat the latter as an
	// internal fault (exit 5) rather than blaming the user's input. The message
	// is preserved either way, and the typed envelope still carries any pending
	// deprecation notice.
	var fallback error
	if isCobraUsageError(err) {
		fallback = errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err.Error())
	} else {
		fallback = errs.NewInternalError(errs.SubtypeUnknown, "%s", err.Error()).WithCause(err)
	}
	output.WriteTypedErrorEnvelope(errOut, fallback, string(f.ResolvedIdentity))
	return output.ExitCodeOf(fallback)
}

// cobraUsageErrorMarkers are the stable error-text fragments cobra / pflag
// (pinned at v1.10.2) emit for usage mistakes — missing required flag, unknown
// command / flag, wrong argument count. Cobra surfaces these as plain errors,
// not a typed value we can match on, so the dispatcher recognizes them by text;
// this is the same external contract unknownFlagName already depends on. A
// residual error matching none of these has leaked the typed boundary and is
// treated as an internal fault, not a user error.
var cobraUsageErrorMarkers = []string{
	"unknown command ",
	"unknown flag: ",
	"unknown shorthand",
	"required flag(s) ",
	"flag needs an argument",
	"bad flag syntax:",
	"no such flag ",
	"invalid argument ",
	"arg(s), ", // accepts / requires N arg(s), received / only received M
}

// isCobraUsageError reports whether err is a cobra / pflag usage mistake,
// identified by the stable error text of the pinned cobra version.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	for _, m := range cobraUsageErrorMarkers {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return false
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

// unknownSubcommandRunE replaces cobra's silent help fallback on group commands
// with a typed *errs.ValidationError: a flag that belongs to a missing
// subcommand, a misplaced subcommand-only flag, or an unknown subcommand name
// each fail structured (exit 2) instead of degrading to help + exit 0.
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
			verr := errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown flag %s before a subcommand for %q", strings.Join(unknown, ", "), cmd.CommandPath()).
				WithHint("flags belong to a subcommand; run `%s --help` to list subcommands and their flags", cmd.CommandPath())
			for _, flag := range unknown {
				verr.WithParams(errs.InvalidParam{Name: flag, Reason: "unknown flag before a subcommand"})
			}
			return verr
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
		verr := errs.NewValidationError(errs.SubtypeInvalidArgument,
			"missing subcommand for %q; flag %s belongs to a subcommand, not the group", cmd.CommandPath(), strings.Join(misplaced, ", ")).
			WithHint("run `%s --help` to list subcommands and their flags", cmd.CommandPath())
		for _, flag := range misplaced {
			verr.WithParams(errs.InvalidParam{Name: flag, Reason: "flag belongs to a subcommand, not the group"})
		}
		return verr
	}
	// The whole remainder, not just its first token: a caller who split a
	// resource path into separate arguments ("chat members get") named a real
	// method, and only the full sequence can show that.
	return unknownSubcommandErrorFor(cmd, args)
}

// unknownSubcommandError builds the typed rejection for a single subcommand name
// cmd cannot resolve. Shared by the --help path (unknownSubcommandInHelp) so one
// bad name yields the same message, hint, and params however it was typed.
func unknownSubcommandError(cmd *cobra.Command, unknown string) error {
	return unknownSubcommandErrorFor(cmd, []string{unknown})
}

// unknownSubcommandErrorFor builds the typed rejection for the argv remainder cmd
// could not resolve. The message and the offending param name stay on args[0] —
// that is the token cobra stopped at — while the guidance considers the whole
// remainder.
func unknownSubcommandErrorFor(cmd *cobra.Command, args []string) error {
	msg := fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.CommandPath())
	hint, suggestions := unknownNameGuidance(cmd, args)
	// Record the offending subcommand and its ranked candidates as a param with
	// machine-readable Suggestions so an agent can retry without parsing the
	// hint; the hint carries the same candidates as prose.
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg).
		WithParams(errs.InvalidParam{Name: args[0], Reason: "unknown subcommand", Suggestions: suggestions}).
		WithHint("%s", hint)
}

// unknownNameGuidance produces the hint and the machine-readable suggestions for
// an argv remainder cmd cannot resolve, in decreasing order of certainty:
// a determinate rewrite, a ranked did-you-mean, or the set itself.
func unknownNameGuidance(cmd *cobra.Command, args []string) (hint string, suggestions []string) {
	paths := methodPathsUnder(cmd)
	if spaced, ok := normalizedPathRewrite(paths, args); ok {
		// The tree really holds this method — the caller only separated its path
		// segments the wrong way. That makes the correction certain, so name it
		// outright instead of ranking guesses around it.
		return fmt.Sprintf("run `%s %s`; a method's path segments are separated by spaces, not dots",
			cmd.CommandPath(), spaced), []string{spaced}
	}
	available, deprecated := availableSubcommandNames(cmd)
	// Rank suggestions across both current and deprecated names so a mistyped
	// legacy command (e.g. +raed → +read) still resolves; the alias stays
	// runnable and self-flags via the _notice on execution. Methods living
	// under a hidden resource group are ranked too — availableSubcommandNames
	// cannot see them, which is why a merely-misspelt method name
	// (nodes.craete) used to come back with no suggestion at all.
	candidates := append(append([]string{}, available...), deprecated...)
	for _, p := range paths {
		candidates = append(candidates, p.spaced)
	}
	if ranked := suggest.Closest(args[0], candidates, 6); len(ranked) > 0 {
		return fmt.Sprintf("did you mean one of: %s? (run `%s --help` for the full list)",
			strings.Join(ranked, ", "), cmd.CommandPath()), ranked
	}
	// Nothing ranked: the edit distance to every real name is too large, which is
	// what happens when the name does not exist at all — and that is precisely
	// when sending the caller back to `--help` answers nothing. A caller who needs
	// a second call to learn the set concludes the method is missing and reaches
	// for the raw `api` channel instead. Name the set here so one call closes the
	// question.
	//
	// The set must name what a caller can actually write after cmd, and the
	// visible children are not always that: a domain's resource groups are hidden
	// by design, so its visible children are only the shortcuts while every API
	// method sits one level below, unreachable by name from this listing. Stating
	// "the set is this" there omits every method the domain has — the exact false
	// conclusion this hint exists to prevent, asserted more confidently than the
	// message it replaced.
	//
	// So a nested path is added exactly when its own first segment is not visible,
	// which is the case where descending from a listed name cannot reach it. When
	// that segment IS listed — the root, whose children are the domains — the
	// nested path answers a question one level deeper than the one being asked and
	// buys nothing: at the root it is 766 extra entries and an 18 KB envelope,
	// past this repository's largest output budget for a listing that fits in a
	// few hundred bytes. Note the condition is about visibility, not about depth:
	// a hidden group anywhere earns its methods a place here, and a visible one
	// anywhere does not.
	//
	// Deprecated aliases stay out: rankable above, not worth recommending. Note
	// the exclusion covers the alias itself, not paths beneath one — `visible` is
	// built from `available` alone, so a deprecated group holding nested methods
	// would have them listed. No such group exists today.
	//
	// The --help pointer stays, since only help carries the descriptions.
	visible := make(map[string]bool, len(available))
	for _, n := range available {
		visible[n] = true
	}
	names := append([]string{}, available...)
	for _, p := range paths {
		if first, _, _ := strings.Cut(p.spaced, " "); !visible[first] {
			names = append(names, p.spaced)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("run `%s --help` to see available subcommands", cmd.CommandPath()), nil
	}
	sort.Strings(names)
	return fmt.Sprintf("%q has no subcommand %q; it has: %s (run `%s --help` for these with descriptions)",
		cmd.CommandPath(), args[0], strings.Join(names, ", "), cmd.CommandPath()), nil
}

// rootUnknownCommandRewrite upgrades cobra's bare "unknown command" into the
// structured rejection a domain group already gives. The root is where a caller
// lands after copying a dotted path out of `schema` — root help's own quickstart
// sends them there — so the forms arriving here are the fully dotted one, the
// partly dotted one, the one with the domain prefix dropped, and the whole path
// inside a single quoted argument. They are one mistake, a separator the tree
// does not use, and one normalization identifies all of them.
//
// The remainder cannot come from root.Flags(): cobra rejects this case inside
// Find, before any command parses flags, so the parsed remainder is empty here.
// It is rebuilt from the raw invocation, anchored on the name cobra itself
// rejected — recovered from its message, the same stable-text contract
// cobraUsageErrorMarkers already relies on. The anchor matters because a flag
// written apart from its value ("--profile work badcmd") leaves that value among
// the raw positionals, ahead of the real one; starting at cobra's own token
// keeps the param naming what the message names. A flag trailing the path still
// leaves its value in the tail, where it costs only a rewrite that does not
// happen. The message stays cobra's, being the accurate account of what failed;
// only the hint and the machine-readable suggestion are added.
func rootUnknownCommandRewrite(root *cobra.Command, err error) error {
	if root == nil {
		return err
	}
	rejected, ok := unknownCommandName(err.Error())
	if !ok {
		return err
	}
	args := positionalsFrom(positionalArgs(rawInvocationArgs), rejected)
	hint, suggestions := unknownNameGuidance(root, args)
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err.Error()).
		WithParams(errs.InvalidParam{Name: args[0], Reason: "unknown command", Suggestions: suggestions}).
		WithHint("%s", hint)
}

// unknownCommandName recovers the token cobra rejected from its own message.
// cobra renders it with %q, so it is read back with the matching verb rather
// than by scanning for a delimiter — the name may itself contain spaces, which
// is exactly the quoted-whole-path form this rewrite exists to catch.
func unknownCommandName(msg string) (string, bool) {
	var name, parent string
	if _, err := fmt.Sscanf(msg, "unknown command %q for %q", &name, &parent); err != nil {
		return "", false
	}
	return name, name != ""
}

// positionalsFrom returns the positionals starting at first, or just first when
// it is absent — the caller needs a remainder that begins with the name cobra
// rejected, never one that begins ahead of it.
func positionalsFrom(all []string, first string) []string {
	for i, a := range all {
		if a == first {
			return all[i:]
		}
	}
	return []string{first}
}

// unknownSubcommandInHelp returns the rejection a `--help` invocation earned by
// naming a subcommand the tree does not have, or nil when help should render.
//
// cobra routes --help through flag.ErrHelp, which never reaches RunE and so
// never reaches unknownSubcommandRunE: without this check `lark-cli drive
// file.comment.replys.create --help` degrades to drive's own help at exit 0,
// telling the caller nothing about the name it got wrong. That silence is worse
// than a plain error, because the unknown_subcommand hint sends callers to
// --help precisely to learn the right name.
//
// Only a pure group is considered. Such a group consumes no positional argument
// of its own, so anything cobra left unconsumed is a subcommand name it could
// not resolve; a bare group leaves nothing, and a real method command is not a
// pure group. cmd.Flags() has already been parsed on this path (cobra checks
// --help only after ParseFlags), so Args() holds the leftovers — the HelpFunc
// args parameter is not used, being cobra's post-Find flag slice rather than the
// positional remainder.
// positionalSubjectInHelp returns the rejection a `--help` earned by naming the
// very subject the command answers about, or nil when help should render. See
// cmdutil.MarkPositionalSubject: `schema im.messages --help` asks what that
// method takes, and cobra replies with a description of `schema` — the tool the
// question was asked with — having dropped the path without a word.
//
// The path is not echoed back. It is caller-supplied text on its way into an
// error envelope, and the caller already knows what they typed; naming the fix
// is what they do not have.
func positionalSubjectInHelp(cmd *cobra.Command) error {
	if !cmdutil.HasPositionalSubject(cmd) || len(cmd.Flags().Args()) == 0 {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"`--help` on %q describes the command, not the path given to it", cmd.CommandPath()).
		WithHint("re-run the same command without `--help`: `%s` prints the full parameter contract for that path",
			cmd.CommandPath())
}

func unknownSubcommandInHelp(cmd *cobra.Command) error {
	if cmd.Annotations[cmdpolicy.AnnotationPureGroup] != "true" {
		return nil
	}
	rest := cmd.Flags().Args()
	if len(rest) == 0 {
		return nil
	}
	// A leading --help is parsed before Cobra descends, so the remainder can
	// still name a perfectly good command (`lark-cli --help drive`). That is a
	// help request for a command that exists, not an unknown name: only a
	// remainder Find cannot consume is one the caller got wrong.
	if target, leftover, err := cmd.Find(rest); err == nil && target != cmd && len(leftover) == 0 {
		return nil
	}
	return unknownSubcommandError(cmd, rest[0])
}

// methodPath is one leaf command's path below a group, in both the form a
// caller may have typed (dotted) and the form that runs (spaced).
type methodPath struct{ dotted, spaced string }

// methodPathsUnder returns every leaf command reachable below cmd, descending
// through hidden resource groups. Resource groups are hidden from domain help by
// design (cmd/service/service.go) yet stay invocable, so a suggestion has to be
// able to name a method that lives under one — availableSubcommandNames, which
// skips hidden children, structurally cannot. A hidden leaf is skipped: a policy
// layer took it away, and suggesting it would point at a path that rejects even
// --help.
func methodPathsUnder(cmd *cobra.Command) []methodPath {
	var out []methodPath
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		for _, ch := range c.Commands() {
			name := ch.Name()
			if name == "help" || name == "completion" {
				continue
			}
			segs := append(append([]string{}, path...), name)
			if ch.HasSubCommands() {
				walk(ch, segs)
				continue
			}
			// A direct child needs no path form; availableSubcommandNames covers it.
			if ch.Hidden || len(segs) < 2 {
				continue
			}
			out = append(out, methodPath{strings.Join(segs, "."), strings.Join(segs, " ")})
		}
	}
	walk(cmd, nil)
	return out
}

// normalizeSegments splits args into the flat token sequence a method path is
// made of, accepting both separators a caller may have used: the dot (copied out
// of a schema path or a domain-help row) and the space (including the spaces
// inside one quoted argument). Empty tokens are dropped, so a doubled or
// trailing separator does not change the sequence.
func normalizeSegments(args []string) []string {
	var out []string
	for _, arg := range args {
		for _, field := range strings.Fields(arg) {
			for _, seg := range strings.Split(field, ".") {
				if seg != "" {
					out = append(out, seg)
				}
			}
		}
	}
	return out
}

// normalizedPathRewrite returns the executable, space-separated form of the
// method a caller named, when their argv — normalized on both separators —
// identifies exactly one path. Normalizing the whole sequence rather than
// substituting one separator is what makes this correct for a resource whose own
// name contains dots ("chat.members") as well as for a genuinely nested one.
//
// A trailing match is accepted after a full one fails: dropping the domain
// prefix ("chat.members.get") is the form a caller produces by copying a
// service-relative domain-help row up to the root. It requires at least two
// segments, because that is what makes it a copied path rather than a guess — a
// single token like "update" would otherwise "match" whichever domain happens to
// hold exactly one method by that name, and the rewrite would assert a path the
// caller never typed while explaining a separator their input never contained.
// Only a unique hit rewrites — with more than one candidate a ranked
// did-you-mean is honest about the uncertainty in a way a confident rewrite
// would not be.
func normalizedPathRewrite(paths []methodPath, args []string) (string, bool) {
	want := normalizeSegments(args)
	if len(want) == 0 {
		return "", false
	}
	var exact, trailing []string
	for _, p := range paths {
		segs := normalizeSegments([]string{p.dotted})
		switch {
		case slices.Equal(segs, want):
			exact = append(exact, p.spaced)
		case len(want) >= 2 && len(want) < len(segs) && slices.Equal(segs[len(segs)-len(want):], want):
			trailing = append(trailing, p.spaced)
		}
	}
	if len(exact) == 1 {
		return exact[0], true
	}
	// A full match existing at all means the caller did name a complete path, so
	// a trailing hit is a different method that merely ends the same way.
	if len(exact) == 0 && len(trailing) == 1 {
		return trailing[0], true
	}
	return "", false
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

// positionalArgs returns the non-flag tokens of a raw invocation, in order, and
// treats everything after the "--" terminator as positional. It is the
// complement of flagTokensInArgs and shares its judgement: a token is a flag
// because it looks like one, not because it is defined. The value of a flag
// written apart from its name ("--profile work") therefore survives here; the
// extra token then matches no method path, so the caller sees the guidance they
// saw before this rewrite existed rather than a wrong correction.
func positionalArgs(rawArgs []string) []string {
	var out []string
	for i, a := range rawArgs {
		if a == "--" {
			return append(out, rawArgs[i+1:]...)
		}
		if len(a) >= 2 && a[0] == '-' {
			continue
		}
		out = append(out, a)
	}
	return out
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

// Root command help groups, so an agent sees content domains, agent tooling, and
// CLI management as distinct blocks instead of one flat alphabetical dump.
const (
	groupDomains    = "lark-domains"
	groupTooling    = "agent-tooling"
	groupManagement = "cli-management"
)

// classifyRootCommands assigns root children to help groups after registration.
// Group definitions are attached separately, after optional distribution
// projection, so a concealed build can omit a now-empty heading.
func classifyRootCommands(root *cobra.Command) {
	tooling := map[string]bool{"api": true, "schema": true, "skills": true}
	management := map[string]bool{"auth": true, "config": true, "profile": true, "doctor": true, "update": true}
	for _, c := range root.Commands() {
		if c.GroupID != "" {
			continue
		}
		switch {
		case tooling[c.Name()]:
			c.GroupID = groupTooling
		case management[c.Name()]:
			c.GroupID = groupManagement
		case isLarkDomain(c):
			c.GroupID = groupDomains
		}
	}
}

// finalizeRootCommandGroups attaches Cobra group definitions once. A group is
// omitted only when this build's surface plan concealed all its children.
// Hidden legacy/YAML commands remain referenceable and therefore keep the
// historical (possibly empty) heading.
func finalizeRootCommandGroups(root *cobra.Command, plan *surface.Plan) {
	if root == nil || len(root.Groups()) != 0 {
		return
	}
	groups := []*cobra.Group{
		{ID: groupDomains, Title: "Lark domains:"},
		{ID: groupTooling, Title: "Agent tooling:"},
		{ID: groupManagement, Title: "CLI management:"},
	}
	for _, group := range groups {
		if plan != nil && !rootGroupHasReferenceableChild(root, group.ID, plan) {
			// Cobra validates that every non-empty child GroupID has a
			// matching definition before dispatch, including hidden children.
			// If presentation removes an entire group, clear those now-hidden
			// assignments as well as omitting the heading.
			for _, child := range root.Commands() {
				if child.GroupID == group.ID {
					child.GroupID = ""
				}
			}
			continue
		}
		root.AddGroup(group)
	}
}

func rootGroupHasReferenceableChild(root *cobra.Command, groupID string, plan *surface.Plan) bool {
	for _, child := range root.Commands() {
		if child.GroupID == groupID &&
			plan.CanReference(surface.CommandID(cmdpolicy.CanonicalPath(child))) {
			return true
		}
	}
	return false
}

// isLarkDomain reports whether a root child is a Lark domain (service-sourced or
// shortcut-tagged), not CLI tooling. Mirrors service.PrepareDomainHelp.
func isLarkDomain(c *cobra.Command) bool {
	if src, _ := cmdmeta.SourceOf(c); src == cmdmeta.SourceService {
		return true
	}
	return cmdmeta.Domain(c) != ""
}

// flagDidYouMean is the root FlagErrorFunc (inherited by all subcommands). It
// converts cobra's flag-parse errors into a typed validation envelope: an
// unknown flag gets a focused "did you mean" hint (so agents recover even when
// the typo is semantic, e.g. --query vs --find, where edit distance alone finds
// nothing) and the offending flag in `params`. Invalid values on alias-backed
// flags retain the caller's spelling; all other flag errors stay typed but
// generic.
func flagDidYouMean(c *cobra.Command, ferr error) error {
	name, isUnknown := unknownFlagName(ferr)
	if !isUnknown {
		// A policy-gated flag invoked bare ("flag needs an argument")
		// never reaches its rejecting Value; it still presents as
		// unregistered, exactly like a set one.
		if gated, ok := gatedFlagFromNeedsArg(c, ferr); ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown flag %q for %q", "--"+gated, c.CommandPath()).
				WithParams(errs.InvalidParam{Name: "--" + gated, Reason: "unknown flag"}).
				WithHint("run `%s --help` to see valid flags", c.CommandPath())
		}
		validationErr := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", ferr.Error()).
			WithHint("run `%s --help` for valid flags", c.CommandPath())
		if attribution, ok := flagalias.InvalidValueAttributionOf(ferr); ok {
			validationErr.WithParam("--" + attribution.Source)
			if attribution.Source != attribution.Canonical {
				validationErr.WithHint("--%s maps to canonical flag --%s; run `%s --help` for valid values", attribution.Source, attribution.Canonical, c.CommandPath())
			}
		}
		return validationErr
	}
	valid := visibleFlagNames(c)
	suggestions := suggest.Closest(name, valid, 3)
	for i := range suggestions {
		suggestions[i] = "--" + suggestions[i]
	}
	hint := fmt.Sprintf("run `%s --help` to see valid flags", c.CommandPath())
	if len(suggestions) > 0 {
		hint = fmt.Sprintf("did you mean %s? (run `%s --help` for all flags)",
			strings.Join(suggestions, ", "), c.CommandPath())
	}
	// The ranked candidates ride on the param as machine-readable Suggestions so
	// an agent can retry without parsing the hint; the hint carries the same
	// candidates as prose. The full valid-flag list stays recoverable via --help.
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"unknown flag %q for %q", "--"+name, c.CommandPath()).
		WithParams(errs.InvalidParam{Name: "--" + name, Reason: "unknown flag", Suggestions: suggestions}).
		WithHint("%s", hint)
}

// gatedFlagFromNeedsArg reports whether ferr is pflag's "flag needs an
// argument: --name" for a policy-gated flag on this command's flag set.
func gatedFlagFromNeedsArg(c *cobra.Command, ferr error) (string, bool) {
	const p = "flag needs an argument: --"
	msg := ferr.Error()
	i := strings.Index(msg, p)
	if i < 0 {
		return "", false
	}
	name := msg[i+len(p):]
	if j := strings.IndexAny(name, " \t"); j >= 0 {
		name = name[:j]
	}
	if fl := c.Root().PersistentFlags().Lookup(name); isPolicyGatedFlag(fl) {
		return name, true
	}
	return "", false
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

// installHelpCommand upgrades Cobra's default help command, which has no error
// channel: it can only print and exit 0. Two rejections need one, and both would
// otherwise be delivered as silence — `help <plugin-restricted-cmd>`, which must
// return a typed error (exit 2) rather than an envelope at exit 0, and a help
// topic naming a subcommand the tree does not have, which the stock command
// answers either by rendering the parent's help or by printing "Unknown help
// topic" over the root usage, both at exit 0 and neither saying what was wrong.
func installHelpCommand(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	helpCmd := findByPath(root, "help")
	if helpCmd == nil {
		return
	}
	helpCmd.Run = nil
	helpCmd.RunE = func(c *cobra.Command, args []string) error {
		target, rest, err := root.Find(args)
		if err != nil || target == nil {
			// `help im.chat.members.get`: Find rejects the argv before it can name
			// any target, and answering "Unknown help topic" at exit 0 says nothing
			// about which part was wrong. Route it through the guidance the run path
			// gives, so a dotted path earns its determinate rewrite here too.
			if len(args) > 0 {
				return unknownSubcommandErrorFor(root, args)
			}
			c.Printf("Unknown help topic %#q\n", args)
			return root.Usage()
		}
		if msg, ok := unavailableHelpMessage(target); ok {
			return errs.NewValidationError(errs.SubtypeCommandUnavailable, "%s", msg)
		}
		// `help im chat.members.gett`: Find resolved as far as `im` and left the
		// name unconsumed. Rendering im's help at exit 0 is the same silent
		// fallback the --help path already rejects, reached through the help
		// subcommand instead — and the caller is left with nothing to correct.
		// Only a pure group is considered, for the reason unknownSubcommandInHelp
		// states: anything it leaves over is a subcommand name it could not resolve.
		if len(rest) > 0 && target.Annotations[cmdpolicy.AnnotationPureGroup] == "true" {
			return unknownSubcommandErrorFor(target, rest)
		}
		target.SetContext(c.Context())
		target.InitDefaultHelpFlag()
		target.InitDefaultVersionFlag()
		return target.Help()
	}
	// help attaches after policy evaluation (framework meta command, never
	// policy-evaluated). No risk annotation: it would render a "Risk:"
	// line that stock cobra help output does not carry.
	cmdutil.DisableAuthCheck(helpCmd)
}

// installTipsHelpFunc wraps the default help function to append a TIPS section
// when a command has tips set via cmdutil.SetTips. It also force-shows global
// flags that are normally hidden in single-app mode (currently --profile)
// when rendering the root command's own help, so users discovering the CLI
// still see them at `lark-cli --help`.
//
// help is this build's renderer; its skill and reference fields are read lazily
// at help-render time (not captured up front) so the domain-guide pointer
// reflects the resolved skill tree -- the same f.SkillContent that `skills
// list`/`read` serve -- even though plugin skill customization is applied after
// this help func is installed.
func installTipsHelpFunc(root *cobra.Command, help *service.HelpRenderer) {
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if err := unknownSubcommandInHelp(cmd); err != nil {
			// Deliberately no help output: rendering this group's help is the
			// silent fallback that leaves the caller with nothing to correct.
			pendingHelpRejection = err
			return
		}
		if err := positionalSubjectInHelp(cmd); err != nil {
			pendingHelpRejection = err
			return
		}
		if cmd == root {
			// Force-show flags hidden by single-app mode; never a
			// policy-retired one.
			if f := root.PersistentFlags().Lookup("profile"); f != nil && f.Hidden && !isPolicyGatedFlag(f) {
				f.Hidden = false
				defer func() { f.Hidden = true }()
			}
		}
		// Domain and method commands compose their agent guidance into Long lazily
		// here (shortcuts attach after service registration); both skip the generic
		// bottom-of-help append below.
		if help.PrepareDomainHelp(cmd) || help.PrepareMethodHelp(cmd) || help.PrepareShortcutHelp(cmd) {
			defaultHelp(cmd, args)
			return
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
