// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/errs"
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
		Short: "Show saved config",
		Long:  "Shows saved config. To see the app/profile lark-cli is using now, run `lark-cli whoami --json`.",
		// Override parent's RequireBuiltinCredentialProvider check: this
		// command reads the SAVED config only (its own help promises "saved
		// config, not current usage"), so the currently effective credential
		// source — external or otherwise — must not gate it.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			c.SilenceUsage = true
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return configShowRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func configShowRun(opts *ConfigShowOptions) error {
	f := opts.Factory

	config, err := core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return core.NotConfiguredError()
		}
		return errs.NewConfigError(errs.SubtypeInvalidConfig, "failed to load config: %v", err).WithCause(err)
	}
	if config == nil || len(config.Apps) == 0 {
		return core.NotConfiguredError()
	}
	// Saved config only: the session profile (--profile / LARKSUITE_CLI_PROFILE)
	// must not change what this command shows — the help and skill routing
	// promise "saved config, not current usage" (use whoami for that).
	app := config.CurrentAppConfig("")
	if app == nil {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "no active profile").WithHint("run: lark-cli profile list")
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
		"workspace": core.CurrentWorkspace().Display(),
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
