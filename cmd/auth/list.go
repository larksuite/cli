// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
)

// ListOptions holds all inputs for auth list.
type ListOptions struct {
	Factory *cmdutil.Factory
	JSON    bool
}

// NewCmdAuthList creates the auth list subcommand.
func NewCmdAuthList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	return newCmdAuthList(f, runF, nil)
}

func newCmdAuthList(
	f *cmdutil.Factory,
	runF func(*ListOptions) error,
	projector *recovery.Projector,
) *cobra.Command {
	opts := &ListOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all logged-in users",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authListRunWithRecovery(opts, projector)
		},
	}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func authListRun(opts *ListOptions) error {
	return authListRunWithRecovery(opts, nil)
}

func authListRunWithRecovery(opts *ListOptions, projector *recovery.Projector) error {
	f := opts.Factory

	multi, err := core.LoadMultiAppConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		subtype := errs.SubtypeNotConfigured
		if errors.Is(err, core.ErrMalformedConfig) {
			subtype = errs.SubtypeInvalidConfig
		}
		return errs.NewConfigError(subtype, "failed to load config: %v", err).WithCause(err)
	}
	if multi == nil || len(multi.Apps) == 0 {
		if opts.JSON {
			output.PrintJson(f.IOStreams.Out, map[string]interface{}{
				"ok":     true,
				"users":  []map[string]interface{}{},
				"reason": "not_configured",
			})
			return nil
		}
		// auth list is a read-only probe; the "configured but no users"
		// branch below already returns exit 0 with a stderr hint, so we
		// keep the same contract here. We still want the hint to be
		// workspace-aware, so we pull the message+hint out of
		// NotConfiguredError() instead of hard-coding it.
		var cfgErr *errs.ConfigError
		if errors.As(projector.Render(core.NotConfiguredError()), &cfgErr) {
			fmt.Fprintln(f.IOStreams.ErrOut, cfgErr.Message)
			if cfgErr.Hint != "" {
				fmt.Fprintln(f.IOStreams.ErrOut, "  hint: "+cfgErr.Hint)
			}
		}
		return nil
	}

	// A selector that matches no profile is an input error, not an empty
	// account: reporting it as not_logged_in (exit 0) would steer the caller
	// into auth login against a profile that does not exist.
	app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
	if err != nil {
		return err
	}
	if len(app.Users) == 0 {
		if opts.JSON {
			output.PrintJson(f.IOStreams.Out, map[string]interface{}{
				"ok":     true,
				"users":  []map[string]interface{}{},
				"reason": "not_logged_in",
			})
			return nil
		}
		fmt.Fprint(f.IOStreams.ErrOut, "No logged-in users.")
		if projector.CanReference(recovery.TargetAuthLogin) {
			fmt.Fprint(f.IOStreams.ErrOut, " Run `lark-cli auth login` to log in.")
		}
		fmt.Fprintln(f.IOStreams.ErrOut)
		return nil
	}

	var items []map[string]interface{}
	for _, u := range app.Users {
		stored, readErr := larkauth.GetStoredToken(app.AppId, u.UserOpenId)
		status := "no_token"
		if readErr != nil {
			status = identitydiag.StatusError
		} else if stored != nil {
			status = larkauth.TokenStatus(stored)
		}
		item := map[string]interface{}{
			"userName":    u.UserName,
			"userOpenId":  u.UserOpenId,
			"appId":       app.AppId,
			"tokenStatus": status,
		}
		if readErr != nil {
			presented := f.PresentError(readErr, cmdutil.ErrorPresentationOptions{
				Projector: projector,
				Identity:  core.AsUser,
			})
			if problem, ok := errs.ProblemOf(presented); ok {
				item["error"] = problem
			}
		}
		items = append(items, item)
	}
	output.PrintJson(f.IOStreams.Out, items)
	return nil
}
