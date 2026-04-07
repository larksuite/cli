// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// profileListItem is the JSON output for a single profile entry.
type profileListItem struct {
	Name        string         `json:"name"`
	AppID       string         `json:"appId"`
	Brand       core.LarkBrand `json:"brand"`
	Active      bool           `json:"active"`
	User        string         `json:"user,omitempty"`
	TokenStatus string         `json:"tokenStatus,omitempty"`
}

// NewCmdProfileList creates the profile list subcommand.
func NewCmdProfileList(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileListRun(f)
		},
	}
	return cmd
}

func profileListRun(f *cmdutil.Factory) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			output.PrintJson(f.IOStreams.Out, []profileListItem{})
			return nil
		}
		return output.Errorf(output.ExitValidation, "config", "failed to load config: %v", err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		output.PrintJson(f.IOStreams.Out, []profileListItem{})
		return nil
	}

	// Intentionally uses "" to show the persistent active profile, not the ephemeral --profile override.
	currentApp := multi.CurrentAppConfig("")
	currentName := ""
	if currentApp != nil {
		currentName = currentApp.ProfileName()
	}

	items := make([]profileListItem, 0, len(multi.Apps))
	for i := range multi.Apps {
		app := &multi.Apps[i]
		name := app.ProfileName()

		item := profileListItem{
			Name:   name,
			AppID:  app.AppId,
			Brand:  app.Brand,
			Active: name == currentName,
		}

		if len(app.Users) > 0 {
			item.User = app.Users[0].UserName
			stored := larkauth.GetStoredToken(app.AppId, app.Users[0].UserOpenId)
			if stored != nil {
				item.TokenStatus = larkauth.TokenStatus(stored)
			}
		}

		items = append(items, item)
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}
