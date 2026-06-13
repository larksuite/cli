// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/hook"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

// installFatalGuard wires a fail-closed guard at every cobra dispatch
// path on rootCmd. Used by the three abort-side fatal paths:
//
//   - FailClosed plugin install failure  (installPluginInstallErrorGuard)
//   - Plugin Restrict conflict           (installPluginConflictGuard)
//   - Startup lifecycle handler failure  (installPluginLifecycleErrorGuard)
//
// **Why we walk the tree rather than set PersistentPreRunE on root**:
// cobra's PersistentPreRunE has "first PersistentPreRunE wins"
// semantics -- the lookup starts at the invoked command and walks UP,
// stopping at the first non-nil PersistentPreRunE. Subcommands that
// declare their own PersistentPreRunE (cmd/auth/auth.go and
// cmd/config/config.go both do) would shadow root's, letting a
// fail-closed condition silently bypass via `lark-cli auth foo`.
//
// The fix: replace the RunE of every runnable command with one that
// returns makeErr(). Subcommands cannot bypass because the dispatch
// lands directly on their RunE, which now carries the guard.
//
// makeErr is called for every guarded dispatch; it must return a fresh
// typed error each time.
func installFatalGuard(rootCmd *cobra.Command, makeErr func() error) {
	// Two cobra subcommands are injected lazily at Execute() time and
	// would otherwise slip past walkGuard. We pre-register both so
	// walkGuard catches them.
	//
	//   - "completion" (user-visible): InitDefaultCompletionCmd
	//   - "__complete" (internal shell-completion RPC): no public
	//     constructor; we add our own stub with the same name. cobra's
	//     internal initCompleteCmd checks for an existing "__complete"
	//     and skips registration if found, so our stub stays in place.
	//     (Cobra dispatches the "__completeNoDesc" alias through the
	//     same RunE, so guarding "__complete" covers both.)
	rootCmd.InitDefaultCompletionCmd()
	alreadyPresent := false
	for _, c := range rootCmd.Commands() {
		if c.Name() == "__complete" {
			alreadyPresent = true
			break
		}
	}
	if !alreadyPresent {
		rootCmd.AddCommand(&cobra.Command{
			Use:    "__complete",
			Hidden: true,
			RunE:   func(*cobra.Command, []string) error { return makeErr() },
		})
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return makeErr()
	}
	rootCmd.PersistentPreRun = nil
	walkGuard(rootCmd, makeErr)
}

// installPluginInstallErrorGuard surfaces a FailClosed plugin install
// failure as a typed validation error (failed_precondition) before any
// command runs.
func installPluginInstallErrorGuard(rootCmd *cobra.Command, installErr error) {
	makeErr := func() error {
		var pi *internalplatform.PluginInstallError
		if errors.As(installErr, &pi) {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", pi.Error()).
				WithHint("plugin %q failed to install (reason_code %s); fix or remove the plugin before running commands", pi.PluginName, pi.ReasonCode).
				WithCause(installErr)
		}
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", installErr.Error()).
			WithHint("a plugin failed to install (reason_code %s); fix or remove the plugin before running commands", internalplatform.ReasonInstallFailed).
			WithCause(installErr)
	}
	installFatalGuard(rootCmd, makeErr)
}

// installPluginConflictGuard surfaces a Plugin.Restrict() configuration
// error (single plugin invalid Rule or multiple plugins each contributing
// Restrict). The hint separates the two failure modes by reason code:
//
//   - "invalid_rule"              - single bad rule
//   - "multiple_restrict_plugins" - multiple Restrict plugins conflict
//
// Either way the CLI must NOT silently continue with a broken policy.
func installPluginConflictGuard(rootCmd *cobra.Command, err error) {
	makeErr := func() error {
		reasonCode := internalplatform.ReasonInvalidRule
		if errors.Is(err, cmdpolicy.ErrMultipleRestricts) {
			reasonCode = internalplatform.ReasonMultipleRestricts
		}
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", err.Error()).
			WithHint("plugin policy configuration is broken (reason_code %s); fix the plugin's Restrict rule or remove the conflicting plugin", reasonCode).
			WithCause(err)
	}
	installFatalGuard(rootCmd, makeErr)
}

// installPluginLifecycleErrorGuard surfaces a Startup lifecycle handler
// failure as a typed validation error (failed_precondition). The hint's
// reason code splits returned-error vs panic so consumers (audit /
// on-call) can tell the two failure modes apart.
func installPluginLifecycleErrorGuard(rootCmd *cobra.Command, err error) {
	makeErr := func() error {
		reasonCode := "lifecycle_failed"
		hookName := ""
		var le *hook.LifecycleError
		if errors.As(err, &le) {
			if le.Panic {
				reasonCode = "lifecycle_panic"
			}
			hookName = le.HookName
		}
		typed := errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", err.Error()).
			WithCause(err)
		if hookName != "" {
			return typed.WithHint("plugin startup hook %q failed (reason_code %s); fix or remove the plugin before running commands", hookName, reasonCode)
		}
		return typed.WithHint("a plugin startup hook failed (reason_code %s); fix or remove the plugin before running commands", reasonCode)
	}
	installFatalGuard(rootCmd, makeErr)
}

// walkGuard recurses through cmd's subtree and installs the guard at
// EVERY level cobra might dispatch to. The cobra execution order is:
//
//  1. PersistentPreRunE (looked up from leaf, walking up; "first wins")
//  2. PreRunE
//  3. RunE
//  4. PostRunE
//  5. PersistentPostRunE
//
// A subcommand that declares its own PersistentPreRunE (cmd/auth and
// cmd/config both do) would not only shadow root's PersistentPreRunE
// -- if that PreRunE itself returns an error (e.g. auth's
// external_provider check), the user sees THAT error instead of
// our plugin_install envelope, even if RunE was guarded.
//
// To close every dispatch hole we replace:
//   - every command's PersistentPreRunE (including non-runnable groups)
//   - every runnable command's PreRunE and RunE
//
// This way the very first non-nil step in cobra's chain is always our
// guard, regardless of which leaf the user invoked.
func walkGuard(cmd *cobra.Command, makeErr func() error) {
	if cmd == nil {
		return
	}
	// PersistentPreRunE is the first step cobra runs (after Args /
	// flag validation -- see below). Set it on every command (root
	// included) so cobra's "first wins" walk-up always finds OUR
	// PersistentPreRunE before hitting any subcommand's pre-existing
	// one.
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		c.SilenceUsage = true
		return makeErr()
	}
	cmd.PersistentPreRun = nil

	// **Cobra dispatch order before PersistentPreRunE:**
	//   1. ValidateArgs(cmd.Args)            -- can return arg error
	//   2. ParsePersistentFlags / ParseFlags -- can return flag error
	//   3. Find legacyArgs check for unknown-command at root
	//   4. PersistentPreRunE / PreRunE / RunE
	//   5. Non-runnable groups fall through to help (PreRunE skipped)
	//
	// We neutralise each step:
	//   - Args = ArbitraryArgs     -> ValidateArgs no-op. **Not nil**:
	//                                 cobra falls back to legacyArgs
	//                                 when Args==nil, which returns an
	//                                 unknown-command error during Find
	//                                 BEFORE PersistentPreRunE runs.
	//                                 ArbitraryArgs explicitly accepts
	//                                 everything, suppressing that path.
	//   - DisableFlagParsing       -> ParseFlags skipped (and legacy
	//                                 "unknown flag" suppressed)
	//   - PreRunE / RunE on EVERY  -> Even non-runnable groups now run
	//     command (not just leaves)   the guard instead of showing help
	//
	// Setting RunE on a parent group flips Runnable() to true, so
	// cobra dispatches to it (and our guard fires) rather than calling
	// the help command on a "help-only" group.
	cmd.Args = cobra.ArbitraryArgs
	cmd.DisableFlagParsing = true
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		c.SilenceUsage = true
		return makeErr()
	}
	cmd.PreRun = nil
	cmd.RunE = func(*cobra.Command, []string) error { return makeErr() }
	cmd.Run = nil
	for _, c := range cmd.Commands() {
		walkGuard(c, makeErr)
	}
}
