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
	users := "(no logged-in users)"
	if len(app.Users) > 0 {
		var userStrs []string
		for _, u := range app.Users {
			userStrs = append(userStrs, fmt.Sprintf("%s (%s)", u.UserName, u.UserOpenId))
		}
		users = strings.Join(userStrs, ", ")
	}
	output.PrintJson(f.IOStreams.Out, map[string]interface{}{
		"profile":   app.ProfileName(),
		"appId":     app.AppId,
		"appSecret": "****",
		"brand":     app.Brand,
		"lang":      app.Lang,
		"users":     users,
	})
	fmt.Fprintf(f.IOStreams.ErrOut, "\nConfig file path: %s\n", core.GetConfigPath())
	return nil
}
