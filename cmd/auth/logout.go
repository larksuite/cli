// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// LogoutOptions holds all inputs for auth logout.
type LogoutOptions struct {
	Factory *cmdutil.Factory
	Ctx     context.Context
	JSON    bool
}

// NewCmdAuthLogout creates the auth logout subcommand.
func NewCmdAuthLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out (clear token)",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLogoutRun(opts)
		},
	}
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")
	cmdutil.SetRisk(cmd, "write")

	return cmd
}

func authLogoutRun(opts *LogoutOptions) error {
	f := opts.Factory
	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

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
				"ok":        true,
				"loggedOut": false,
				"reason":    "not_configured",
			})
			return nil
		}
		fmt.Fprintln(f.IOStreams.ErrOut, "No configuration found.")
		return nil
	}

	// A selector that matches no profile is an input error, not a logged-out
	// state: "not_logged_in" (exit 0) would hide a stale --profile or
	// LARKSUITE_CLI_PROFILE value from the caller.
	app, err := multi.RequireAppConfig(f.Invocation.Profile, f.Invocation.ProfileSource)
	if err != nil {
		return err
	}
	if len(app.Users) == 0 {
		if opts.JSON {
			output.PrintJson(f.IOStreams.Out, map[string]interface{}{
				"ok":        true,
				"loggedOut": false,
				"reason":    "not_logged_in",
			})
			return nil
		}
		fmt.Fprintln(f.IOStreams.ErrOut, "Not logged in.")
		return nil
	}

	httpClient, httpErr := f.HttpClient()
	cfg, authErr := core.ResolveConfigFromMulti(multi, f.Keychain, f.Invocation.Profile, f.Invocation.ProfileSource)
	var clientAuth larkauth.ClientAuth
	if authErr == nil {
		clientAuth, authErr = resolveClientAuth(ctx, f, cfg)
	}

	for _, user := range app.Users {
		if httpErr == nil && authErr == nil {
			token, _ := larkauth.GetStoredToken(app.AppId, user.UserOpenId)
			if token != nil {
				revokeToken := token.RefreshToken
				tokenTypeHint := "refresh_token"
				if revokeToken == "" {
					revokeToken = token.AccessToken
					tokenTypeHint = "access_token"
				}
				if revokeToken != "" {
					_ = larkauth.RevokeToken(ctx, httpClient, clientAuth, app.Brand, revokeToken, tokenTypeHint)
				}
			}
		}
		if err := larkauth.RemoveStoredToken(app.AppId, user.UserOpenId); err != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "Warning: failed to remove token for %s: %v\n", user.UserOpenId, err)
		}
	}

	app.Users = []core.AppUser{}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save config: %v", err).WithCause(err)
	}
	if opts.JSON {
		output.PrintJson(f.IOStreams.Out, map[string]interface{}{
			"ok":        true,
			"loggedOut": true,
		})
		return nil
	}
	output.PrintSuccess(f.IOStreams.ErrOut, "Logged out")
	return nil
}
