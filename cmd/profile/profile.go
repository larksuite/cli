// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/runtimeplan"
)

// NewCmdProfile creates the profile command with subcommands.
func NewCmdProfile(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			// A child PersistentPreRunE shadows root's PersistentPreRun, so retain
			// the invocation state used by structured error hints here.
			cmd.SilenceUsage = true
			f.CurrentCommand = cmd
			return f.RequireCommandRuntimeCapabilities(cmd.Context(), cmd)
		},
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetRuntimeCapabilities(cmd, runtimeplan.CapabilityLocalProfileMutation)
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT switch or remove profiles unless the user explicitly asks.",
	})

	list := NewCmdProfileList(f)
	// Listing profiles is read-only and remains useful for diagnostics under a
	// managed credential runtime. Every other profile subcommand mutates local
	// profile selection, config, or keychain state and inherits the parent gate.
	cmdutil.SetRuntimeCapabilities(list)
	cmd.AddCommand(list)
	cmd.AddCommand(NewCmdProfileUse(f))
	cmd.AddCommand(NewCmdProfileAdd(f))
	cmd.AddCommand(NewCmdProfileRemove(f))
	cmd.AddCommand(NewCmdProfileRename(f))
	return cmd
}
