// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ConfigShowOptions holds all inputs for config show.
type ConfigShowOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdConfigShow creates the config show subcommand.
func NewCmdConfigShow(f *cmdutil.Factory, runF func(*ConfigShowOptions) error) *cobra.Command {
	opts := &ConfigShowOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return configShowRun(opts)
		},
	}

	return cmd
}

func configShowRun(opts *ConfigShowOptions) error {
	f := opts.Factory

	config, err := core.LoadMultiAppConfig()
	if err != nil || config == nil || len(config.Apps) == 0 {
		fmt.Fprintf(f.IOStreams.ErrOut, "Not configured yet. Config file path: %s\n", core.GetConfigPath())
		fmt.Fprintln(f.IOStreams.ErrOut, "Run `lark-cli config init` to initialize.")
		return nil
	}
	app := config.CurrentAppConfig(f.Invocation.Profile)
	if app == nil {
		fmt.Fprintln(f.IOStreams.ErrOut, "No active profile found.")
		return nil
	}
	runtime := runtimeConfigSnapshot(f)
	profile := app.ProfileName()
	appID := app.AppId
	brand := string(app.Brand)
	users := formatStoredUsers(app.Users)

	if runtime != nil {
		if runtime.ProfileName != "" {
			profile = runtime.ProfileName
		}
		if runtime.AppID != "" {
			appID = runtime.AppID
		}
		if runtime.Brand != "" {
			brand = string(runtime.Brand)
		}
		if runtime.UserOpenId != "" {
			users = formatRuntimeUser(runtime.UserName, runtime.UserOpenId)
		}
	}

	output.PrintJson(f.IOStreams.Out, map[string]interface{}{
		"profile":   profile,
		"appId":     appID,
		"appSecret": "****",
		"brand":     brand,
		"lang":      app.Lang,
		"users":     users,
	})
	fmt.Fprintf(f.IOStreams.ErrOut, "\nConfig file path: %s\n", core.GetConfigPath())
	return nil
}

func runtimeConfigSnapshot(f *cmdutil.Factory) *core.CliConfig {
	if f == nil || f.Config == nil {
		return nil
	}
	cfg, err := f.Config()
	if err != nil {
		return nil
	}
	return cfg
}

func formatStoredUsers(users []core.AppUser) string {
	if len(users) == 0 {
		return "(no logged-in users)"
	}
	var userStrs []string
	for _, u := range users {
		userStrs = append(userStrs, formatRuntimeUser(u.UserName, u.UserOpenId))
	}
	return strings.Join(userStrs, ", ")
}

func formatRuntimeUser(name, openID string) string {
	if name == "" {
		return openID
	}
	return fmt.Sprintf("%s (%s)", name, openID)
}
