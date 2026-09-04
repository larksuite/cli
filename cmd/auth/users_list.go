// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// UsersListOptions holds inputs for auth users list.
type UsersListOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthUsersList creates the auth users list command.
func NewCmdAuthUsersList(f *cmdutil.Factory, runF func(*UsersListOptions) error) *cobra.Command {
	opts := &UsersListOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List users in the current profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authUsersListRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func authUsersListRun(opts *UsersListOptions) error {
	f := opts.Factory
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}
	app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
	if err != nil {
		return err
	}
	if len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No logged-in users. Run `lark-cli auth login` to add one.")
		return nil
	}

	active, err := app.ActiveUser()
	if err != nil {
		return err
	}
	items := make([]map[string]interface{}, 0, len(app.Users))
	for _, user := range app.Users {
		status := "no_token"
		if token := larkauth.GetStoredToken(app.AppId, user.UserOpenId); token != nil {
			status = larkauth.TokenStatus(token)
		}
		items = append(items, map[string]interface{}{
			"userName":    user.UserName,
			"userOpenId":  user.UserOpenId,
			"tokenStatus": status,
			"active":      active != nil && user.UserOpenId == active.UserOpenId,
		})
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}
