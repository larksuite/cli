// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/hook"
	"github.com/larksuite/cli/internal/output"
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
// *output.ExitError each time (the envelope writer mutates a few fields
// as it serialises).
// Deprecated: installFatalGuard accepts a *output.ExitError-producing lambda,
// which is part of the legacy error surface that predates the typed error
// contract introduced by errs/. New code MUST NOT add new callers — the
// platform-extension fatal-guard plumbing will switch to typed errs.* errors
// when the platform-extension framework migrates. This wrapper is retained
// only for the existing in-tree call sites; it will be removed once they
// have moved to the typed surface.
func installFatalGuard(rootCmd *cobra.Command, makeErr func() *output.ExitError) {
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
// failure as a structured plugin_install envelope before any command
// runs.
// Deprecated: installPluginInstallErrorGuard produces a legacy
// *output.ExitError via its internal makeErr lambda. New code MUST NOT add
// such producers — plugin install failures should surface as a typed
// *errs.XxxError once the platform-extension framework migrates. This
// helper is retained only while existing call sites are migrated; it will
// be removed once they have moved to the typed surface.
func installPluginInstallErrorGuard(rootCmd *cobra.Command, installErr error) {
	makeErr := func() *output.ExitError {
		var pi *internalplatform.PluginInstallError
		if errors.As(installErr, &pi) {
			return &output.ExitError{
				Code: output.ExitValidation,
				Detail: &output.ErrDetail{
					Type:    "plugin_install",
					Message: pi.Error(),
					Detail: map[string]any{
						"plugin":      pi.PluginName,
						"reason_code": pi.ReasonCode,
						"reason":      pi.Reason,
					},
				},
				Err: installErr,
			}
		}
		return &output.ExitError{
			Code: output.ExitValidation,
			Detail: &output.ErrDetail{
				Type:    "plugin_install",
				Message: installErr.Error(),
				Detail: map[string]any{
					"reason_code": internalplatform.ReasonInstallFailed,
				},
			},
			Err: installErr,
		}
	}
	installFatalGuard(rootCmd, makeErr)
}

// installPluginConflictGuard surfaces a Plugin.Restrict() configuration
// error (single plugin invalid Rule or multiple plugins each contributing
// Restrict). The design separates the envelope type:
//
//   - "plugin_install" with reason_code "invalid_rule"           - single bad rule
//   - "plugin_conflict" with reason_code "multiple_restrict_plugins" - multi
//
// Either way the CLI must NOT silently continue with a broken policy.
// Deprecated: installPluginConflictGuard produces a legacy *output.ExitError
// via its internal makeErr lambda. New code MUST NOT add such producers —
// plugin conflict failures should surface as a typed *errs.XxxError once the
// platform-extension framework migrates. This helper is retained only while
// existing call sites are migrated; it will be removed once they have moved
// to the typed surface.
func installPluginConflictGuard(rootCmd *cobra.Command, err error) {
	makeErr := func() *output.ExitError {
		envelopeType := "plugin_install"
		reasonCode := internalplatform.ReasonInvalidRule
		if errors.Is(err, cmdpolicy.ErrMultipleRestricts) {
			envelopeType = "plugin_conflict"
			reasonCode = internalplatform.ReasonMultipleRestricts
		}
		return &output.ExitError{
			Code: output.ExitValidation,
			Detail: &output.ErrDetail{
				Type:    envelopeType,
				Message: err.Error(),
				Detail: map[string]any{
					"reason_code": reasonCode,
				},
			},
			Err: err,
		}
	}
	installFatalGuard(rootCmd, makeErr)
}

// installPluginLifecycleErrorGuard surfaces a Startup lifecycle handler
// failure as a plugin_lifecycle envelope. The reason_code splits
// returned-error vs panic so consumers (audit / on-call) can tell the
// two failure modes apart.
// Deprecated: installPluginLifecycleErrorGuard produces a legacy
// *output.ExitError via its internal makeErr lambda. New code MUST NOT add
// such producers — plugin lifecycle failures should surface as a typed
// *errs.XxxError once the platform-extension framework migrates. This
// helper is retained only while existing call sites are migrated; it will
// be removed once they have moved to the typed surface.
func installPluginLifecycleErrorGuard(rootCmd *cobra.Command, err error) {
	makeErr := func() *output.ExitError {
		reasonCode := "lifecycle_failed"
		detail := map[string]any{
			"reason_code": reasonCode,
		}
		var le *hook.LifecycleError
		if errors.As(err, &le) {
			if le.Panic {
				reasonCode = "lifecycle_panic"
			}
			detail = map[string]any{
				"reason_code": reasonCode,
				"hook_name":   le.HookName,
				"event":       "startup",
			}
		}
		return &output.ExitError{
			Code: output.ExitValidation,
			Detail: &output.ErrDetail{
				Type:    "plugin_lifecycle",
				Message: err.Error(),
				Detail:  detail,
			},
			Err: err,
		}
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
// Deprecated: walkGuard accepts a *output.ExitError-producing lambda, part
// of the legacy error surface that predates the typed error contract
// introduced by errs/. New code MUST NOT add new callers — the platform-
// extension guard plumbing will switch to typed errs.* errors when the
// platform-extension framework migrates. This wrapper is retained only for
// the existing in-tree call sites; it will be removed once they have moved
// to the typed surface.
func walkGuard(cmd *cobra.Command, makeErr func() *output.ExitError) {
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
