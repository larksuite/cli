// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// UsersUseOptions holds inputs for auth users use.
type UsersUseOptions struct {
	Factory *cmdutil.Factory
	Target  string
}

// NewCmdAuthUsersUse creates the auth users use command.
func NewCmdAuthUsersUse(f *cmdutil.Factory, runF func(*UsersUseOptions) error) *cobra.Command {
	opts := &UsersUseOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "use <open_id|user_name>",
		Short: "Switch the active user in the current profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Target = args[0]
			if runF != nil {
				return runF(opts)
			}
			return authUsersUseRun(opts)
		},
	}
	cmdutil.SetTips(cmd, []string{"AI agents: Do NOT switch users unless the user explicitly asks."})
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func authUsersUseRun(opts *UsersUseOptions) error {
	f := opts.Factory
	target := strings.TrimSpace(opts.Target)
	if target == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "user selector cannot be empty").WithParam("<open_id|user_name>")
	}

	return withAuthConfigLock(func() error {
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
		if app.CurrentUser == user.UserOpenId {
			fmt.Fprintf(f.IOStreams.ErrOut, "Already using %s (%s)\n", user.UserName, user.UserOpenId)
			return nil
		}

		app.CurrentUser = user.UserOpenId
		if err := core.SaveMultiAppConfig(multi); err != nil {
			return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
		}
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Active user: %s (%s)", user.UserName, user.UserOpenId))
		return nil
	})
}

func findCommandUser(app *core.AppConfig, target string) (*core.AppUser, error) {
	for i := range app.Users {
		if app.Users[i].UserOpenId == target {
			return &app.Users[i], nil
		}
	}

	match := -1
	for i := range app.Users {
		if app.Users[i].UserName != target {
			continue
		}
		if match >= 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"user name %q is ambiguous in profile %q", target, app.ProfileName()).
				WithHint("use the user's open_id from `lark-cli auth users list`")
		}
		match = i
	}
	if match >= 0 {
		return &app.Users[match], nil
	}
	return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
		"user %q not found in profile %q", target, app.ProfileName()).
		WithHint("run `lark-cli auth users list` to see available users")
}
