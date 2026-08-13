// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/runtimeplan"
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
			return f.RequireCommandRuntimeCapabilities(cmd.Context(), cmd)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityLocalCredentialManagement)

	initCmd := NewCmdConfigInit(f, nil)
	bind := newCmdConfigBind(f, nil, projector)
	remove := NewCmdConfigRemove(f, nil)
	show := NewCmdConfigShow(f, nil)
	defaultAs := NewCmdConfigDefaultAs(f)
	strictMode := NewCmdConfigStrictMode(f)
	riskControl := NewCmdConfigRiskControl(f)
	policy := NewCmdConfigPolicy(f)
	plugins := NewCmdConfigPlugins(f)
	keychainDowngrade := NewCmdConfigKeychainDowngrade(f)
	// Identity preferences live in the Profile, but external providers have
	// historically treated these config commands as credential management.
	// Check Profile ownership first so a managed runtime gives the actionable
	// deployment-managed Profile error, then retain the credential capability
	// so Standard external-provider behavior stays unchanged.
	for _, identitySetting := range []*cobra.Command{defaultAs, strictMode} {
		cmdutil.SetRuntimeCapabilities(
			identitySetting,
			runtimeplan.CapabilityLocalProfileMutation,
			runtimeplan.CapabilityLocalCredentialManagement,
		)
	}
	for _, sourceNeutral := range []*cobra.Command{show, riskControl, policy, plugins} {
		cmdutil.SetRuntimeCapabilities(sourceNeutral)
	}
	cmd.AddCommand(initCmd, bind, remove, show, defaultAs, strictMode, riskControl, policy, plugins, keychainDowngrade)
	return cmd
}

func parseBrand(value string) core.LarkBrand {
	return core.ParseBrand(value)
}
