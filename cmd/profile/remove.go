// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdProfileRemove creates the profile remove subcommand.
func NewCmdProfileRemove(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileRemoveRun(f, args[0])
		},
	}
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT remove profiles unless the user explicitly asks. This is destructive and clears all associated credentials.",
	})
	return cmd
}

func profileRemoveRun(f *cmdutil.Factory, name string) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
	}

	idx := multi.FindAppIndex(name)
	if idx < 0 {
		return output.ErrValidation("profile %q not found, available profiles: %s", name, strings.Join(multi.ProfileNames(), ", "))
	}

	if len(multi.Apps) == 1 {
		return output.ErrValidation("cannot remove the only profile")
	}

	app := &multi.Apps[idx]
	removedName := app.ProfileName()
	appId := app.AppId
	appSecret := app.AppSecret
	users := app.Users

	// Remove from slice
	multi.Apps = append(multi.Apps[:idx], multi.Apps[idx+1:]...)

	// Fix currentApp / previousApp references
	if multi.CurrentApp == removedName {
		multi.CurrentApp = multi.Apps[0].ProfileName()
	}
	if multi.PreviousApp == removedName {
		multi.PreviousApp = ""
	}

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	// Best-effort credential cleanup after config commit
	core.RemoveSecretStore(appSecret, f.Keychain)
	for _, user := range users {
		larkauth.RemoveStoredToken(appId, user.UserOpenId)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Profile %q removed", removedName))
	return nil
}
