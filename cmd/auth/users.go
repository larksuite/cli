// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdAuthUsers creates the auth users command group.
func NewCmdAuthUsers(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "List, switch, or remove logged-in users",
	}
	cmdutil.SetRisk(cmd, "read")
	cmd.AddCommand(NewCmdAuthUsersList(f, nil))
	cmd.AddCommand(NewCmdAuthUsersUse(f, nil))
	cmd.AddCommand(NewCmdAuthUsersLogout(f, nil))
	return cmd
}
