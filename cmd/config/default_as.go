// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// NewCmdConfigDefaultAs creates the "config default-as" subcommand.
func NewCmdConfigDefaultAs(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default-as [user|bot|auto]",
		Short: "View or set default identity type",
		Long:  "Without arguments, shows the current default identity. Pass user, bot, or auto to set a new default.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			multi, err := core.LoadMultiAppConfig()
			if err != nil {
				return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
			}

			app := multi.CurrentAppConfig(f.Invocation.Profile)
			if app == nil {
				return output.ErrWithHint(output.ExitValidation, "config", "no active profile", "run: lark-cli config init")
			}

			if len(args) == 0 {
				current := app.DefaultAs
				if current == "" {
					current = "auto"
				}
				fmt.Fprintf(f.IOStreams.Out, "default-as: %s\n", current)
				return nil
			}

			value := args[0]
			if value != "user" && value != "bot" && value != "auto" {
				return output.ErrValidation("invalid identity type %q, valid values: user | bot | auto", value)
			}

			app.DefaultAs = core.Identity(value)
			if err := core.SaveMultiAppConfig(multi); err != nil {
				return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "Default identity set to: %s\n", value)
			return nil
		},
	}
	return cmd
}
