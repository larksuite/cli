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

// ListOptions holds all inputs for auth list.
type ListOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthList creates the auth list subcommand.
func NewCmdAuthList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all logged-in users",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authListRun(opts)
		},
	}

	return cmd
}

func authListRun(opts *ListOptions) error {
	f := opts.Factory

	multi, _ := core.LoadMultiAppConfig()
	if multi == nil || len(multi.Apps) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "Not configured yet. Run `lark-cli config init` to initialize.")
		return nil
	}

	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil || len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No logged-in users. Run `lark-cli auth login` to log in.")
		return nil
	}

	var items []map[string]interface{}
	for _, u := range app.Users {
		stored := larkauth.GetStoredToken(app.AppId, u.UserOpenId)
		status := "no_token"
		if stored != nil {
			status = larkauth.TokenStatus(stored)
		}
		items = append(items, map[string]interface{}{
			"userName":    u.UserName,
			"userOpenId":  u.UserOpenId,
			"appId":       app.AppId,
			"tokenStatus": status,
		})
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}
