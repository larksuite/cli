// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// NewCmdConfigStrictMode creates the "config strict-mode" subcommand.
func NewCmdConfigStrictMode(f *cmdutil.Factory) *cobra.Command {
	var global bool
	var reset bool

	cmd := &cobra.Command{
		Use:   "strict-mode [bot|user|off]",
		Short: "View or set strict mode (identity restriction policy)",
		Long: `View or set strict mode — the identity restriction policy.

  bot   only bot identity allowed (user commands hidden)
  user  only user identity allowed (bot commands hidden)
  off   no restriction (default)

No args: show current mode. Switching does NOT require re-bind.

For AI agents: this is a security policy. DO NOT switch without
explicit user confirmation — never run on your own initiative.`,
		Example: `  lark-cli config strict-mode               # show current
  lark-cli config strict-mode user          # switch (after user confirms)
  lark-cli config strict-mode bot --global  # set globally
  lark-cli config strict-mode --reset       # clear profile override`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read-only show path: skip the flock so a peer holding it across
			// a TUI prompt (e.g. `config bind`) doesn't turn an instant status
			// query into a 30s timeout. Every mutating arm below saves and
			// MUST take the lock.
			if !reset && len(args) == 0 {
				multi, err := core.LoadOrNotConfigured()
				if err != nil {
					return err
				}
				app := multi.CurrentAppConfig(f.Invocation.Profile)
				if app == nil {
					return core.NoActiveProfileError()
				}
				return showStrictMode(cmd.Context(), f, multi, app)
			}

			root := larkauth.NewLocalRoot(core.GetConfigDir())
			flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeStorage, "strict-mode: acquire flock: %v", err).WithCause(err)
			}
			defer lk.Release()

			multi, err := core.LoadOrNotConfigured()
			if err != nil {
				return err
			}

			if reset {
				app := multi.CurrentAppConfig(f.Invocation.Profile)
				if app == nil {
					return core.NoActiveProfileError()
				}
				return resetStrictMode(f, multi, app, global, args)
			}
			app := multi.CurrentAppConfig(f.Invocation.Profile)
			if !global && app == nil {
				return core.NoActiveProfileError()
			}
			return setStrictMode(f, multi, app, args[0], global)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "set at global level (applies to all profiles)")
	cmd.Flags().BoolVar(&reset, "reset", false, "reset profile setting to inherit global")
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func resetStrictMode(f *cmdutil.Factory, multi *core.MultiAppConfig, app *core.AppConfig, global bool, args []string) error {
	if global {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reset cannot be used with --global").WithParam("--reset")
	}
	if len(args) > 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--reset cannot be used with a value argument").WithParam("--reset")
	}
	app.StrictMode = nil
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}
	fmt.Fprintln(f.IOStreams.ErrOut, "Profile strict-mode reset (inherits global)")
	return nil
}

func showStrictMode(ctx context.Context, f *cmdutil.Factory, multi *core.MultiAppConfig, app *core.AppConfig) error {
	// Runtime effective mode from credential provider chain is the source of truth.
	runtime := f.ResolveStrictMode(ctx)
	configMode, configSource := resolveStrictModeStatus(multi, app)

	if runtime != configMode {
		fmt.Fprintf(f.IOStreams.Out, "strict-mode: %s (source: credential provider)\n", runtime)
		return nil
	}
	fmt.Fprintf(f.IOStreams.Out, "strict-mode: %s (source: %s)\n", configMode, configSource)
	return nil
}

func setStrictMode(f *cmdutil.Factory, multi *core.MultiAppConfig, app *core.AppConfig, value string, global bool) error {
	mode := core.StrictMode(value)
	switch mode {
	case core.StrictModeBot, core.StrictModeUser, core.StrictModeOff:
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid value %q, valid values: bot | user | off", value)
	}

	// Capture the old mode at the SAME scope being changed, so we can warn
	// only when the policy actually expands user-identity at that scope.
	//   --global → compare raw multi.StrictMode (profiles with explicit
	//     overrides are unaffected; their warning comes from the existing
	//     "profile %q has strict-mode explicitly set" notice below).
	//   profile  → compare effective mode (override > global > default), so
	//     a profile flipping from inherited bot to explicit off still warns.
	// The previous version always used the profile's effective mode, which
	// false-positived (--global change while current profile has an explicit
	// override) and false-negatived (--global broadening that doesn't affect
	// the current profile but does affect other inheriting profiles).
	var oldMode core.StrictMode
	if global {
		oldMode = multi.StrictMode
	} else {
		oldMode, _ = resolveStrictModeStatus(multi, app)
	}

	if global {
		multi.StrictMode = mode
		for _, a := range multi.Apps {
			if a.StrictMode != nil && *a.StrictMode != mode {
				fmt.Fprintf(f.IOStreams.ErrOut,
					"Warning: profile %q has strict-mode explicitly set to %q, "+
						"which overrides the global setting. "+
						"Use --reset in that profile to inherit global.\n",
					a.ProfileName(), *a.StrictMode)
			}
		}
	} else {
		if app == nil {
			return core.NoActiveProfileError()
		}
		app.StrictMode = &mode
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	if oldMode == core.StrictModeBot && (mode == core.StrictModeUser || mode == core.StrictModeOff) {
		fmt.Fprintln(f.IOStreams.ErrOut, "⚠️ "+strictModeRelaxLang(app).IdentityEscalationMessage)
	}

	scope := "profile"
	if global {
		scope = "global"
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "Strict mode set to %s (%s)\n", mode, scope)
	return nil
}

// strictModeRelaxLang picks the bind-message bundle whose language matches the
// active profile's Lang setting. Falls back to bindMsgZh when no profile is
// available (global mutation with no current app).
func strictModeRelaxLang(app *core.AppConfig) *bindMsg {
	if app != nil {
		return getBindMsg(app.Lang)
	}
	return getBindMsg("")
}

func resolveStrictModeStatus(multi *core.MultiAppConfig, app *core.AppConfig) (core.StrictMode, string) {
	if app != nil && app.StrictMode != nil {
		return *app.StrictMode, fmt.Sprintf("profile %q", app.ProfileName())
	}
	if multi.StrictMode.IsActive() {
		return multi.StrictMode, "global"
	}
	return core.StrictModeOff, "global (default)"
}
