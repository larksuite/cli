// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ConfigRemoveOptions holds all inputs for config remove.
type ConfigRemoveOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdConfigRemove creates the config remove subcommand.
func NewCmdConfigRemove(f *cmdutil.Factory, runF func(*ConfigRemoveOptions) error) *cobra.Command {
	opts := &ConfigRemoveOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove app configuration (clears all tokens and config)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return configRemoveRun(opts)
		},
	}

	return cmd
}

func configRemoveRun(opts *ConfigRemoveOptions) error {
	f := opts.Factory

	config, err := core.LoadMultiAppConfig()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return output.ErrValidation("not configured yet")
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Clean up keychain entries for all apps
	for _, app := range config.Apps {
		core.RemoveSecretStore(app.AppSecret, f.Keychain)
		for _, user := range app.Users {
			auth.RemoveStoredToken(app.AppId, user.UserOpenId)
		}
	}

	// Save empty config
	empty := &core.MultiAppConfig{Apps: []core.AppConfig{}}
	if err := core.SaveMultiAppConfig(empty); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}
	output.PrintSuccess(f.IOStreams.ErrOut, "Configuration removed")
	userCount := 0
	for _, app := range config.Apps {
		userCount += len(app.Users)
	}
	if userCount > 0 {
		fmt.Fprintf(f.IOStreams.ErrOut, "Cleared tokens for %d users\n", userCount)
	}
	return nil
}
