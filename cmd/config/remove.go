// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
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
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func configRemoveRun(opts *ConfigRemoveOptions) error {
	f := opts.Factory

	root := auth.NewLocalRoot(core.GetConfigDir())
	flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lk, err := root.Locks(auth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "config remove: acquire flock: %v", err).WithCause(err)
	}
	defer lk.Release()

	config, err := core.LoadMultiAppConfig()
	if err != nil {
		return core.PassThroughOrNotConfigured(err)
	}
	if config == nil || len(config.Apps) == 0 {
		return errs.NewConfigError(errs.SubtypeNotConfigured, "not configured yet")
	}

	// Save empty config first. If this fails, keep secrets and tokens intact so the
	// existing config can still be retried instead of ending up half-removed.
	empty := &core.MultiAppConfig{Apps: []core.AppConfig{}}
	if err := core.SaveMultiAppConfig(empty); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}

	// Clean up keychain entries for all apps after config is cleared.
	for _, app := range config.Apps {
		core.RemoveSecretStore(app.AppSecret, f.Keychain)
		for _, user := range app.Users {
			// Sweep all three on-host artifacts: keychain UAT,
			// sidecar profile JSON, and user_index.json row. A
			// stale sidecar / index makes a "removed" user
			// resurface in `auth users list` and mis-attribute a
			// future re-login under the same open_id.
			_ = auth.PurgeUserArtifacts(root, app.AppId, user.UserOpenId)
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
