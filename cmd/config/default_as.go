// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"context"
	"fmt"
	"time"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/spf13/cobra"
)

// NewCmdConfigDefaultAs creates the "config default-as" subcommand.
func NewCmdConfigDefaultAs(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "default-as [user|bot|auto]",
		Short: "View or set default identity type",
		Long:  "Without arguments, shows the current default identity. Pass user, bot, or auto to set a new default.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Read-only show path: skip the flock so a peer holding it across
			// a TUI prompt (e.g. `config bind`) doesn't turn an instant status
			// query into a 30s timeout. The set arm below saves and MUST
			// take the lock.
			if len(args) == 0 {
				multi, err := core.LoadOrNotConfigured()
				if err != nil {
					return err
				}
				app := multi.CurrentAppConfig(f.Invocation.Profile)
				if app == nil {
					return core.NoActiveProfileError()
				}
				current := app.DefaultAs
				if current == "" {
					current = "auto"
				}
				fmt.Fprintf(f.IOStreams.Out, "default-as: %s\n", current)
				return nil
			}

			root := larkauth.NewLocalRoot(core.GetConfigDir())
			flockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", 30*time.Second)
			if err != nil {
				return errs.NewInternalError(errs.SubtypeStorage, "default-as: acquire flock: %v", err).WithCause(err)
			}
			defer lk.Release()

			multi, err := core.LoadOrNotConfigured()
			if err != nil {
				return err
			}

			app := multi.CurrentAppConfig(f.Invocation.Profile)
			if app == nil {
				return core.NoActiveProfileError()
			}

			value := args[0]
			if value != "user" && value != "bot" && value != "auto" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid identity type %q, valid values: user | bot | auto", value)
			}

			app.DefaultAs = core.Identity(value)
			if err := core.SaveMultiAppConfig(multi); err != nil {
				return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "Default identity set to: %s\n", value)
			return nil
		},
	}
	cmdutil.SetRisk(cmd, "write")
	return cmd
}
