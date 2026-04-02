// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
)

// ScopesOptions holds all inputs for auth scopes.
type ScopesOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	Format  string
}

// NewCmdAuthScopes creates the auth scopes subcommand.
func NewCmdAuthScopes(f *cmdutil.Factory, runF func(*ScopesOptions) error) *cobra.Command {
	opts := &ScopesOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "scopes",
		Short: "Query scopes enabled for the app",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authScopesRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.Format, "format", "json", "output format: json (default) | pretty")

	cmdutil.NoFileCompletion(cmd)
	return cmd
}

func authScopesRun(opts *ScopesOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}
	fmt.Fprintf(f.IOStreams.ErrOut, "Querying app scopes...\n\n")
	appInfo, err := getAppInfo(opts.Ctx, f, config.AppID)
	if err != nil {
		return output.ErrWithHint(output.ExitAPI, "permission",
			fmt.Sprintf("failed to get app scope info: %v", err),
			"ensure the app has enabled the application:application:self_manage scope.")
	}
	if opts.Format == "pretty" {
		fmt.Fprintf(f.IOStreams.ErrOut, "App ID: %s\n", config.AppID)
		fmt.Fprintf(f.IOStreams.ErrOut, "Enabled scopes (%d):\n\n", len(appInfo.UserScopes))
		for _, s := range appInfo.UserScopes {
			fmt.Fprintf(f.IOStreams.ErrOut, "  • %s\n", s)
		}
	} else {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{
			"appId":      config.AppID,
			"brand":      config.Brand,
			"tokenType":  "user",
			"userScopes": appInfo.UserScopes,
			"count":      len(appInfo.UserScopes),
		})
	}
	return nil
}
