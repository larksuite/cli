// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// LogoutOptions holds all inputs for auth logout.
type LogoutOptions struct {
	Factory *cmdutil.Factory
	JSON    bool
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
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func authLogoutRun(opts *LogoutOptions) error {
	f := opts.Factory

	multi, _ := core.LoadMultiAppConfig()
	if multi == nil || len(multi.Apps) == 0 {
		if opts.JSON {
			output.PrintJson(f.IOStreams.Out, map[string]interface{}{
				"ok":        true,
				"loggedOut": false,
				"reason":    "not_configured",
			})
			return nil
		}
		fmt.Fprintln(f.IOStreams.ErrOut, "No configuration found.")
		return nil
	}

	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil || len(app.Users) == 0 {
		if opts.JSON {
			output.PrintJson(f.IOStreams.Out, map[string]interface{}{
				"ok":        true,
				"loggedOut": false,
				"reason":    "not_logged_in",
			})
			return nil
		}
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}

	for _, user := range app.Users {
		if err := larkauth.RemoveStoredToken(app.AppId, user.UserOpenId); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "Warning: failed to remove token for %s: %v\n", user.UserOpenId, err)
		}
	}
	app.Users = []core.AppUser{}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}
	if opts.JSON {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{
			"ok":        true,
			"loggedOut": true,
		})
		return nil
	}
	output.PrintSuccess(f.IOStreams.ErrOut, "Logged out")
	return nil
}
