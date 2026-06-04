// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// ListOptions holds all inputs for auth list.
type ListOptions struct {
	Factory *cmdutil.Factory
}

// NewCmdAuthList creates the auth list subcommand.
func NewCmdAuthList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all logged-in users",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authListRun(opts)
		},
	}
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func authListRun(opts *ListOptions) error {
	f := opts.Factory

	multi, err := core.LoadMultiAppConfig()
	if err != nil {
		// auth list is a read-only probe — when the file is simply
		// missing we keep exit 0 with a workspace-aware stderr hint.
		// Anything else (R2 forward-incompat, parse error) must still
		// surface with its envelope intact: a *core.ConfigError must
		// reach the dispatcher with its upgrade Hint preserved.
		if errors.Is(err, os.ErrNotExist) {
			printNotConfiguredHint(f.IOStreams.ErrOut)
			return nil
		}
		return core.PassThroughOrNotConfigured(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		// Configured but empty Apps[] — read-only probe, exit 0 with hint.
		printNotConfiguredHint(f.IOStreams.ErrOut)
		return nil
	}

	app := multi.CurrentAppConfig(f.Invocation.Profile)
	if app == nil || len(app.Users) == 0 {
		fmt.Fprintln(f.IOStreams.ErrOut, "No logged-in users. Run `lark-cli auth login` to log in.")
		return nil
	}

	var items []map[string]interface{}
	for _, u := range app.Users {
		stored := larkauth.GetStoredToken(app.AppId, u.UserOpenId)
		status := "no_token"
		if stored != nil {
			status = larkauth.TokenStatus(stored)
		}
		items = append(items, map[string]interface{}{
			"userName":    u.UserName,
			"userOpenId":  u.UserOpenId,
			"appId":       app.AppId,
			"tokenStatus": status,
		})
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}

// printNotConfiguredHint emits the workspace-aware "not configured /
// no users" hint to stderr without failing the read-only probe.
// Pulls Message+Hint from core.NotConfiguredError() so a single source
// of truth governs both the error path and the soft-success path.
func printNotConfiguredHint(errOut io.Writer) {
	var cfgErr *core.ConfigError
	if errors.As(core.NotConfiguredError(), &cfgErr) {
		fmt.Fprintln(errOut, cfgErr.Message)
		if cfgErr.Hint != "" {
			fmt.Fprintln(errOut, "  hint: "+cfgErr.Hint)
		}
	}
}
