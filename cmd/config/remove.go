// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ConfigRemoveOptions holds all inputs for config remove.
type ConfigRemoveOptions struct {
	Factory           *cmdutil.Factory
	LoadConfig        func() (*core.MultiAppConfig, error)
	SaveConfig        func(*core.MultiAppConfig) error
	RemoveSecret      func(core.SecretInput, keychain.KeychainAccess)
	RemoveStoredToken func(string, string) error
}

// NewCmdConfigRemove creates the config remove subcommand.
func NewCmdConfigRemove(f *cmdutil.Factory, runF func(*ConfigRemoveOptions) error) *cobra.Command {
	opts := &ConfigRemoveOptions{
		Factory:           f,
		LoadConfig:        core.LoadMultiAppConfig,
		SaveConfig:        core.SaveMultiAppConfig,
		RemoveSecret:      core.RemoveSecretStore,
		RemoveStoredToken: auth.RemoveStoredToken,
	}

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
	if opts == nil || opts.Factory == nil ||
		opts.LoadConfig == nil || opts.SaveConfig == nil ||
		opts.RemoveSecret == nil || opts.RemoveStoredToken == nil {
		return output.Errorf(output.ExitInternal, "internal", "config remove options not initialized")
	}

	f := opts.Factory

	config, err := opts.LoadConfig()
	if err != nil || config == nil || len(config.Apps) == 0 {
		return output.ErrValidation("not configured yet")
	}

	// Save empty config first. If this fails, keep secrets and tokens intact so the
	// existing config can still be retried instead of ending up half-removed.
	empty := &core.MultiAppConfig{Apps: []core.AppConfig{}}
	if err := opts.SaveConfig(empty); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save config: %v", err)
	}

	// Clean up keychain entries for all apps after config is cleared.
	for _, app := range config.Apps {
		opts.RemoveSecret(app.AppSecret, f.Keychain)
		for _, user := range app.Users {
			if err := opts.RemoveStoredToken(app.AppId, user.UserOpenId); err != nil {
				fmt.Fprintf(f.IOStreams.ErrOut, "warning: failed to remove stored token for app %q user %q: %v\n", app.AppId, user.UserOpenId, err)
			}
		}
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
