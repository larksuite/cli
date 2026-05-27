// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/identitydiag"
	"github.com/larksuite/cli/internal/output"
)

// StatusOptions holds all inputs for auth status.
type StatusOptions struct {
	Factory *cmdutil.Factory
	Verify  bool
}

// NewCmdAuthStatus creates the auth status subcommand.
func NewCmdAuthStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "View current auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authStatusRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Verify, "verify", false, "verify token against server (requires network)")
	cmdutil.SetRisk(cmd, "read")

	return cmd
}

func authStatusRun(opts *StatusOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	defaultAs := config.DefaultAs
	if defaultAs == "" {
		defaultAs = "auto"
	}
	result := map[string]interface{}{
		"appId":     config.AppID,
		"brand":     config.Brand,
		"defaultAs": defaultAs,
	}

	diagnostics := identitydiag.Diagnose(context.Background(), f, config, opts.Verify)
	result["identities"] = diagnostics
	result["identity"] = effectiveIdentity(diagnostics)
	addEffectiveVerification(result, diagnostics)
	addStatusNote(result, diagnostics)

	output.PrintJson(f.IOStreams.Out, result)
	return nil
}

const (
	identityUser = "user"
	identityBot  = "bot"
	identityNone = "none"
)

func effectiveIdentity(d identitydiag.Result) string {
	switch {
	case d.User.Available:
		return identityUser
	case d.Bot.Available:
		return identityBot
	default:
		return identityNone
	}
}

func addEffectiveVerification(result map[string]interface{}, d identitydiag.Result) {
	switch result["identity"] {
	case identityUser:
		if d.User.Verified != nil {
			result["verified"] = *d.User.Verified
			if !*d.User.Verified {
				result["verifyError"] = d.User.Message
			}
		}
	case identityBot:
		if d.Bot.Verified != nil {
			result["verified"] = *d.Bot.Verified
			if !*d.Bot.Verified {
				result["verifyError"] = d.Bot.Message
			}
		}
	}
}

func addStatusNote(result map[string]interface{}, d identitydiag.Result) {
	switch {
	case !d.User.Available && d.Bot.Available:
		result["note"] = "User identity is " + identitydiag.StatusMessage(d.User.Status) + "; bot identity is ready for bot/tenant API calls. Run `lark-cli auth login` to enable user identity."
	case d.User.Status == identitydiag.StatusNeedsRefresh:
		result["note"] = "User identity needs refresh and will be refreshed automatically on the next user API call."
	case !d.User.Available && !d.Bot.Available:
		result["note"] = "No usable identity is available. Configure bot credentials or run `lark-cli auth login`."
	}
}
