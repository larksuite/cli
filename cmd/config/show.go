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
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/workspace"
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
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func configShowRun(opts *ConfigShowOptions) error {
	f := opts.Factory

	config, err := configpkg.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configpkg.NotConfiguredError()
		}
		return errs.NewConfigError(errs.SubtypeInvalidConfig, "failed to load config: %v", err).WithCause(err)
	}
	if config == nil || len(config.Apps) == 0 {
		return configpkg.NotConfiguredError()
	}
	app, err := config.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
	if err != nil {
		return err
	}
	users := "(no logged-in users)"
	if len(app.Users) > 0 {
		var userStrs []string
		for _, u := range app.Users {
			userStrs = append(userStrs, fmt.Sprintf("%s (%s)", u.UserName, u.UserOpenId))
		}
		users = strings.Join(userStrs, ", ")
	}
	// profileSource says which channel picked this profile (config | flag |
	// environment) — with a session-level LARKSUITE_CLI_PROFILE in play, the
	// effective profile and the persisted default can legitimately differ.
	_, effectiveSource := config.EffectiveProfile(f.Invocation.Profile, f.Invocation.ProfileSource)
	output.PrintJson(f.IOStreams.Out, map[string]interface{}{
		"workspace":     workspace.CurrentWorkspace().Display(),
		"profile":       app.ProfileName(),
		"profileSource": effectiveSource.String(),
		"appId":         app.AppId,
		"appSecret":     "****",
		"brand":         app.Brand,
		"lang":          app.Lang,
		"users":         users,
	})
	fmt.Fprintf(f.IOStreams.ErrOut, "\nConfig file path: %s\n", workspace.GetConfigPath())
	return nil
}
