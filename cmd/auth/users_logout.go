// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// UsersLogoutOptions holds inputs for auth users logout.
type UsersLogoutOptions struct {
	Factory *cmdutil.Factory
	Target  string
}

// NewCmdAuthUsersLogout creates the auth users logout command.
func NewCmdAuthUsersLogout(f *cmdutil.Factory, runF func(*UsersLogoutOptions) error) *cobra.Command {
	opts := &UsersLogoutOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "logout <open_id|user_name>",
		Short: "Remove one logged-in user from the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			if runF != nil {
				return runF(opts)
			}
			return authUsersLogoutRun(opts)
		},
	}
	cmdutil.SetTips(cmd, []string{"AI agents: Do NOT log out users unless the user explicitly asks."})
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func authUsersLogoutRun(opts *UsersLogoutOptions) error {
	f := opts.Factory
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "user selector cannot be empty").WithParam("<open_id|user_name>")
	}

	var removed core.AppUser
	var active *core.AppUser
	var appID string
	err := withAuthConfigLock(func() error {
		multi, err := core.LoadOrNotConfigured()
		if err != nil {
			return err
		}
		app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
		if err != nil {
			return err
		}
		user, err := findCommandUser(app, target)
		if err != nil {
			return err
		}

		removed = *user
		appID = app.AppId
		users := make([]core.AppUser, 0, len(app.Users)-1)
		for _, candidate := range app.Users {
			if candidate.UserOpenId != removed.UserOpenId {
				users = append(users, candidate)
			}
		}
		app.Users = users
		if app.CurrentUser == removed.UserOpenId {
			app.CurrentUser = ""
			if len(app.Users) > 0 {
				app.CurrentUser = app.Users[0].UserOpenId
			}
		}
		if err := core.SaveMultiAppConfig(multi); err != nil {
			return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
		}
		if app.CurrentUser != "" {
			resolved, activeErr := app.ActiveUser()
			if activeErr == nil && resolved != nil {
				copy := *resolved
				active = &copy
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Delete the token only after the config commit so a failed write cannot
	// leave a configured user with no local credential.
	if err := larkauth.RemoveStoredToken(appID, removed.UserOpenId); err != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "Warning: failed to remove stored token: %v\n", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Logged out: %s (%s)", removed.UserName, removed.UserOpenId))
	if active != nil {
		fmt.Fprintf(f.IOStreams.ErrOut, "Active user: %s (%s)\n", active.UserName, active.UserOpenId)
	}
	return nil
}
