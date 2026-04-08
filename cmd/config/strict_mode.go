// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmdConfigStrictMode creates the "config strict-mode" subcommand.
func NewCmdConfigStrictMode(f *cmdutil.Factory) *cobra.Command {
	var global bool
	var reset bool

	cmd := &cobra.Command{
		Use:   "strict-mode [bot|user|off]",
		Short: "View or set strict mode (identity restriction policy)",
		Long: `View or set strict mode (identity restriction policy).

Without arguments, shows the current strict mode status and its source.
Pass "bot", "user", or "off" to set strict mode.
Use --global to set at the global level.
Use --reset to clear the profile-level setting (inherit global).

Modes:
  bot   — only bot identity is allowed, user commands are hidden
  user  — only user identity is allowed, bot commands are hidden
  off   — no restriction (default)

WARNING: Strict mode is a security policy set by the administrator.
AI agents are strictly prohibited from modifying this setting.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
			}

			if reset {
				app := multi.CurrentAppConfig(f.Invocation.Profile)
				if app == nil {
					return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
				}
				return resetStrictMode(f, multi, app, global, args)
			}
			if len(args) == 0 {
				app := multi.CurrentAppConfig(f.Invocation.Profile)
				if app == nil {
					return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
				}
				return showStrictMode(cmd.Context(), f, multi, app)
			}
			app := multi.CurrentAppConfig(f.Invocation.Profile)
			if !global && app == nil {
				return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
			}
			return setStrictMode(f, multi, app, args[0], global)
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "set at global level (applies to all profiles)")
	cmd.Flags().BoolVar(&reset, "reset", false, "reset profile setting to inherit global")

	return cmd
}

func resetStrictMode(f *cmdutil.Factory, multi *core.MultiAppConfig, app *core.AppConfig, global bool, args []string) error {
	if global {
		return output.ErrValidation("--reset cannot be used with --global")
	}
	if len(args) > 0 {
		return output.ErrValidation("--reset cannot be used with a value argument")
	}
	app.StrictMode = nil
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
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
		return output.ErrValidation("invalid value %q, valid values: bot | user | off", value)
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
			return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
		}
		app.StrictMode = &mode
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}
	scope := "profile"
	if global {
		scope = "global"
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "Strict mode set to %s (%s)\n", mode, scope)
	return nil
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
