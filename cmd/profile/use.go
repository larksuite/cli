// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"fmt"
	"github.com/larksuite/cli/brand"
	"github.com/larksuite/cli/internal/envvars"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
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
	cmdutil.SetRisk(cmd, "write")
	return cmd
}

func profileUseRun(f *cmdutil.Factory, name string) error {
	multi, err := configpkg.LoadOrNotConfigured()
	if err != nil {
		return err
	}

	// Handle "-" for toggle-back
	if name == "-" {
		if multi.PreviousApp == "" {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "no previous profile to switch back to").
				WithHint("switch to a profile by name first: lark-cli profile use <name>")
		}
		name = multi.PreviousApp
	}

	app := multi.FindApp(name)
	if app == nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "profile %q not found, available profiles: %s", name, strings.Join(multi.ProfileNames(), ", "))
	}

	targetName := app.ProfileName()

	// Short-circuit if already on the target profile
	currentApp := multi.CurrentAppConfig("")
	if currentApp != nil && currentApp.ProfileName() == targetName {
		fmt.Fprintf(f.IOStreams.ErrOut, "Already on profile %q\n", targetName)
		warnShadowedByEnvironment(f, targetName)
		return nil
	}

	// Update previous and current
	if currentApp != nil {
		multi.PreviousApp = currentApp.ProfileName()
	}
	multi.CurrentApp = targetName

	if err := configpkg.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Switched to profile %q (%s, %s)", targetName, app.AppId, app.Brand))
	warnShadowedByEnvironment(f, targetName)
	return nil
}

// warnShadowedByEnvironment tells the user when the persistent default they
// just set (or confirmed) is not what this shell will actually use: use
// writes the persisted state, but a set LARKSUITE_CLI_PROFILE keeps
// overriding every subsequent command. Only the environment source warns —
// a --profile flag ends with this invocation and shadows nothing after it.
func warnShadowedByEnvironment(f *cmdutil.Factory, targetName string) {
	if f.Invocation.ProfileSource != brand.ProfileFromEnvironment ||
		f.Invocation.Profile == targetName {
		return
	}
	fmt.Fprintf(f.IOStreams.ErrOut,
		"Warning: %s=%q is set — commands in this shell keep using %q until you unset it\n",
		envvars.CliProfile, f.Invocation.Profile, f.Invocation.Profile)
}
