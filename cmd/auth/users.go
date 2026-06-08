// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
)

// NewCmdAuthUsers creates the `auth users` subcommand group.
// `auth logout` clears the entire profile; `auth users logout <id>` is
// the per-user surgical version.
func NewCmdAuthUsers(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "users",
		Short: "Manage logged-in users within the active profile",
		Long: `users groups operator commands that act on the per-profile user
list maintained by lark-cli's multi-user auth surface (see
` + "`lark-cli auth login`" + `).

Subcommands:
  list     show all users in the active profile, marking the active one
  use      switch the active user (sets currentUser in config)
  logout   wipe one user's tokens, sidecar profile, and index row

The legacy ` + "`lark-cli auth logout`" + ` continues to clear the entire
profile (all users in one shot); ` + "`lark-cli auth users logout <id>`" + `
is the per-user surgical version.`,
	}
	cmdutil.SetRisk(cmd, "read")
	cmd.AddCommand(NewCmdAuthUsersList(f, nil))
	cmd.AddCommand(NewCmdAuthUsersUse(f, nil))
	cmd.AddCommand(NewCmdAuthUsersLogout(f, nil))
	return cmd
}
