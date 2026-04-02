// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"os"

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
		Use:   "strict-mode [on|off]",
		Short: "View or set strict mode (bot-only identity restriction)",
		Long: `View or set strict mode (bot-only identity restriction).

Without arguments, shows the current strict mode status and its source.
Pass "on" or "off" to set strict mode at the profile level.
Use --global to set at the global level.
Use --reset to clear the profile-level setting (inherit global).

WARNING: Strict mode is a security policy set by the administrator.
AI agents are strictly prohibited from modifying this setting.
Do not run this command to disable strict mode on behalf of automated workflows.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
			}

			app := multi.CurrentAppConfig(f.ProfileOverride)
			if app == nil {
				return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
			}

			// --reset: clear profile-level setting
			if reset {
				if len(args) > 0 {
					return output.ErrValidation("--reset cannot be used with a value argument")
				}
				app.StrictMode = nil
				if err := core.SaveMultiAppConfig(multi); err != nil {
					return fmt.Errorf("failed to save config: %w", err)
				}
				fmt.Fprintln(f.IOStreams.ErrOut, "Profile strict-mode reset (inherits global)")
				return nil
			}

			// No args: show current status
			if len(args) == 0 {
				effective, source := resolveStrictModeStatus(multi, app)
				status := "off"
				if effective {
					status = "on"
				}
				fmt.Fprintf(f.IOStreams.Out, "strict-mode: %s (source: %s)\n", status, source)
				return nil
			}

			// Set value
			value := args[0]
			if value != "on" && value != "off" {
				return output.ErrValidation("invalid value %q, valid values: on | off", value)
			}
			boolVal := value == "on"

			if global {
				multi.StrictMode = boolVal
				if boolVal {
					for _, a := range multi.Apps {
						if a.StrictMode != nil && !*a.StrictMode {
							fmt.Fprintf(f.IOStreams.ErrOut, "Warning: profile %q has strict-mode explicitly set to off, which overrides the global setting. Use `lark-cli config strict-mode --reset` in that profile to inherit global.\n", a.ProfileName())
						}
					}
				}
			} else {
				app.StrictMode = &boolVal
			}

			if err := core.SaveMultiAppConfig(multi); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}
			scope := "profile"
			if global {
				scope = "global"
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "Strict mode set to %s (%s)\n", value, scope)
			return nil
		},
	}

	cmd.Flags().BoolVar(&global, "global", false, "set at global level (applies to all profiles)")
	cmd.Flags().BoolVar(&reset, "reset", false, "reset profile setting to inherit global")

	return cmd
}

// resolveStrictModeStatus returns the effective strict mode value and its source.
func resolveStrictModeStatus(multi *core.MultiAppConfig, app *core.AppConfig) (bool, string) {
	if v := os.Getenv("LARKSUITE_CLI_STRICT_MODE"); v != "" {
		return v == "true" || v == "1", "env LARKSUITE_CLI_STRICT_MODE"
	}
	if app != nil && app.StrictMode != nil {
		return *app.StrictMode, fmt.Sprintf("profile %q", app.ProfileName())
	}
	return multi.StrictMode, "global"
}
