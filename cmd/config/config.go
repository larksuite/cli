// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/spf13/cobra"
)

// NewCmdConfig creates the config command with subcommands.
func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	return newCmdConfig(f, nil)
}

// NewCmdConfigWithRecovery creates the config command with build-local
// recovery projection while preserving NewCmdConfig's established signature.
func NewCmdConfigWithRecovery(f *cmdutil.Factory, projector *recovery.Projector) *cobra.Command {
	return newCmdConfig(f, projector)
}

func newCmdConfig(f *cmdutil.Factory, projector *recovery.Projector) *cobra.Command {
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
	cmd.AddCommand(newCmdConfigBind(f, nil, projector))
	cmd.AddCommand(NewCmdConfigRemove(f, nil))
	cmd.AddCommand(NewCmdConfigShow(f, nil))
	cmd.AddCommand(NewCmdConfigDefaultAs(f))
	cmd.AddCommand(NewCmdConfigStrictMode(f))
	cmd.AddCommand(NewCmdConfigRiskControl(f))
	cmd.AddCommand(NewCmdConfigPolicy(f))
	cmd.AddCommand(NewCmdConfigPlugins(f))
	cmd.AddCommand(NewCmdConfigKeychainDowngrade(f))
	cmd.AddCommand(newCmdConfigTenantAccessToken(f))
	return cmd
}

func parseBrand(value string) core.LarkBrand {
	return core.ParseBrand(value)
}
