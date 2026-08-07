// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"fmt"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	configpkg "github.com/larksuite/cli/internal/config"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/secret"
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
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func configRemoveRun(opts *ConfigRemoveOptions) error {
	f := opts.Factory

	config, err := configpkg.LoadMultiAppConfig()
	if err != nil || config == nil || len(config.Apps) == 0 {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "not configured yet")
	}

	// Save empty config first. If this fails, keep secrets and tokens intact so the
	// existing config can still be retried instead of ending up half-removed.
	empty := &configpkg.MultiAppConfig{Apps: []configpkg.AppConfig{}}
	if err := configpkg.SaveMultiAppConfig(empty); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	// Clean up keychain entries for all apps after config is cleared.
	for _, app := range config.Apps {
		secret.RemoveSecretStore(app.AppSecret, f.Keychain)
		for _, user := range app.Users {
			_ = auth.RemoveStoredToken(app.AppId, user.UserOpenId)
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
