// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdProfile creates the profile command with subcommands.
func NewCmdProfile(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage configuration profiles",
	}
	cmdutil.DisableAuthCheck(cmd)
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT switch or remove profiles unless the user explicitly asks.",
	})

	cmd.AddCommand(NewCmdProfileList(f))
	cmd.AddCommand(NewCmdProfileUse(f))
	cmd.AddCommand(NewCmdProfileAdd(f))
	cmd.AddCommand(NewCmdProfileRemove(f))
	cmd.AddCommand(NewCmdProfileRename(f))
	return cmd
}
