// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

type UsersListOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthUsersList creates the `auth users list` subcommand. Unlike
// `auth list`, output includes the active-user marker plus FirstAuthAt,
// LastUsed, and LastScopes metadata.
func NewCmdAuthUsersList(f *cmdutil.Factory, runF func(*UsersListOptions) error) *cobra.Command {
	opts := &UsersListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all users in the active profile",
		Long:  `list shows every user in the active profile with their token freshness, FirstAuthAt, LastUsed, and the active marker.`,
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

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		// users list is also a read-only probe — keep exit 0 with a
		// stderr hint when the file is simply missing. Anything else
		// (R2 forward-incompat, parse error) must surface with envelope
		// intact: a *core.ConfigError must reach the dispatcher with
		// its upgrade Hint preserved.
		if errors.Is(err, os.ErrNotExist) {
			printNotConfiguredHint(f.IOStreams.ErrOut)
			return nil
		}
		return core.PassThroughOrNotConfigured(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		printNotConfiguredHint(f.IOStreams.ErrOut)
		return nil
	}
	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil || len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No users in this profile. Run `lark-cli auth login` to add one.")
		return nil
	}

	// Active user marker; mirrors ResolveConfigFromMulti precedence
	// (invocation > config > Users[0]).
	active := resolveActiveUserOpenId(f, app)

	items := make([]map[string]interface{}, 0, len(app.Users))
	for _, u := range app.Users {
		stored := larkauth.GetStoredToken(app.AppId, u.UserOpenId)
		status := "no_token"
		if stored != nil {
			status = larkauth.TokenStatus(stored)
		}
		row := map[string]interface{}{
			"userName":    u.UserName,
			"userOpenId":  u.UserOpenId,
			"unionId":     u.UnionId,
			"appId":       app.AppId,
			"tokenStatus": status,
			"active":      u.UserOpenId == active,
			"lastScopes":  u.LastScopes,
		}
		if u.FirstAuthAt != nil {
			row["firstAuthAt"] = u.FirstAuthAt.UTC()
		}
		if u.LastUsed != nil {
			row["lastUsed"] = u.LastUsed.UTC()
		}
		items = append(items, row)
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}

// resolveActiveUserOpenId picks the active AppUser for marking. Precedence
// matches ResolveConfigFromMulti: invocation override, then CurrentUser,
// then Users[0]. Stale references fall through.
func resolveActiveUserOpenId(f *cmdutil.Factory, app *core.AppConfig) string {
	if u := f.Invocation.UserOpenId; u != "" {
		if hit := app.FindUser(u); hit != nil {
			return hit.UserOpenId
		}
	}
	if app.CurrentUser != "" {
		if hit := app.FindUser(app.CurrentUser); hit != nil {
			return hit.UserOpenId
		}
	}
	if len(app.Users) > 0 {
		return app.Users[0].UserOpenId
	}
	return ""
}
