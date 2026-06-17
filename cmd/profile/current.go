// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

type currentProfileOutput struct {
	Profile string         `json:"profile"`
	Source  string         `json:"source"`
	Config  string         `json:"config"`
	AppID   string         `json:"appId"`
	Brand   core.LarkBrand `json:"brand"`
}

// NewCmdProfileCurrent creates the profile current subcommand.
func NewCmdProfileCurrent(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Show the effective profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return profileCurrentRun(f)
		},
	}
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func profileCurrentRun(f *cmdutil.Factory) error {
	multi, err := core.LoadOrNotConfigured()
	if err != nil {
		return err
	}
	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		if f.Invocation.Profile != "" && f.Invocation.ProfileSource == core.ProfileSourceProject {
			return core.ProjectProfileNotFoundError(f.Invocation.Profile, f.Invocation.ProfileConfigPath, multi.ProfileNames())
		}
		return errs.NewConfigError(errs.SubtypeNotConfigured, "no active profile").WithHint("run: lark-cli profile list")
	}

	source := f.Invocation.ProfileSource
	if source == "" {
		source = core.ProfileSourceGlobal
	}
	configPath := core.GetConfigPath()
	if source == core.ProfileSourceProject {
		configPath = f.Invocation.ProfileConfigPath
	}

	output.PrintJson(f.IOStreams.Out, currentProfileOutput{
		Profile: app.ProfileName(),
		Source:  string(source),
		Config:  configPath,
		AppID:   app.AppId,
		Brand:   app.Brand,
	})
	return nil
}
