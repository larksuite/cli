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

// LogoutOptions holds all inputs for auth logout.
type LogoutOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthLogout creates the auth logout subcommand.
func NewCmdAuthLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out (clear token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authLogoutRun(opts)
		},
	}

	return cmd
}

func authLogoutRun(opts *LogoutOptions) error {
	f := opts.Factory

	multi, _ := core.LoadMultiAppConfig()
	if multi == nil || len(multi.Apps) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No configuration found.")
		return nil
	}

	app := &multi.Apps[0]
	if len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}

	active := core.ResolveActiveUser(app.Users)
	if active == nil {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}

	if err := larkauth.RemoveStoredToken(app.AppId, active.UserOpenId); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "Warning: failed to remove token for %s: %v\n", active.UserOpenId, err)
	}
	app.Users = core.RemoveUser(app.Users, active.UserOpenId)
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}
	output.PrintSuccess(f.IOStreams.ErrOut, "Logged out")
	return nil
}
