// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdProfileUse creates the profile use subcommand.
func NewCmdProfileUse(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <name>",
		Short: "Switch to a profile (use '-' to toggle back)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileUseRun(f, args[0])
		},
	}
	cmdutil.SetTips(cmd, []string{
		"AI agents: Do NOT switch profiles unless the user explicitly asks.",
	})
	return cmd
}

func profileUseRun(f *cmdutil.Factory, name string) error {
	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		return output.ErrWithHint(output.ExitValidation, "config", "not configured", "run: lark-cli config init")
	}

	// Handle "-" for toggle-back
	if name == "-" {
		if multi.PreviousApp == "" {
			return output.ErrValidation("no previous profile to switch back to")
		}
		name = multi.PreviousApp
	}

	app := multi.FindApp(name)
	if app == nil {
		return output.ErrValidation("profile %q not found, available profiles: %s", name, strings.Join(multi.ProfileNames(), ", "))
	}

	targetName := app.ProfileName()

	// Short-circuit if already on the target profile
	currentApp := multi.CurrentAppConfig("")
	if currentApp != nil && currentApp.ProfileName() == targetName {
		fmt.Fprintf(f.IOStreams.ErrOut, "Already on profile %q\n", targetName)
		return nil
	}

	// Update previous and current
	if currentApp != nil {
		multi.PreviousApp = currentApp.ProfileName()
	}
	multi.CurrentApp = targetName

	if err := core.SaveMultiAppConfig(multi); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Switched to profile %q (%s, %s)", targetName, app.AppId, app.Brand))
	return nil
}
