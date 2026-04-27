// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// NewCmdConfig creates the config command with subcommands.
func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Global CLI configuration management",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Replicate rootCmd's PersistentPreRun behaviour: cobra stops at the first
			// PersistentPreRun[E] found walking up the chain, so the root-level
			// SilenceUsage=true would be skipped without this line.
			cmd.SilenceUsage = true
			// Pass "config" as a literal — cmd.Name() would return the subcommand name.
			return f.RequireBuiltinCredentialProvider(cmd.Context(), "config")
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(NewCmdConfigInit(f, nil))
	cmd.AddCommand(NewCmdConfigBind(f, nil))
	cmd.AddCommand(NewCmdConfigRemove(f, nil))
	cmd.AddCommand(NewCmdConfigShow(f, nil))
	cmd.AddCommand(NewCmdConfigDefaultAs(f))
	cmd.AddCommand(NewCmdConfigStrictMode(f))
	return cmd
}

func parseBrand(value string) core.LarkBrand {
	return core.ParseBrand(value)
}
