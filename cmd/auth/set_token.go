// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

const defaultRefreshExpiresIn = 30 * 24 * 3600 // 30 days

// SetTokenOptions holds all inputs for auth set-token.
type SetTokenOptions struct {
	Factory          *cmdutil.Factory
	Ctx              context.Context
	JSON             bool
	AccessToken      string
	RefreshToken     string
	OpenID           string
	UserName         string
	ExpiresIn        int
	RefreshExpiresIn int
	Scope            string
}

// NewCmdAuthSetToken creates the auth set-token subcommand.
//
// Set-token imports an already-obtained user OAuth token into the CLI's
// credential store, bypassing the interactive device-flow handshake. It is
// designed for CI/CD environments, agent harnesses, and automation pipelines
// that obtain tokens through an external OAuth flow (e.g. server-side
// authorization-code exchange) and want to make them available to lark-cli
// without user interaction.
//
// The command is purely local: it writes the supplied tokens to the
// keychain and updates the active profile's user list. It performs no
// network calls.
//
// Example:
//
//	lark-cli auth set-token \
//	  --access-token  u-xxxx \
//	  --refresh-token ur-yyyy \
//	  --open-id       ou-zzzz \
//	  --expires-in    7200
func NewCmdAuthSetToken(f *cmdutil.Factory, runF func(*SetTokenOptions) error) *cobra.Command {
	opts := &SetTokenOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "set-token",
		Short: "Import a user OAuth token into the credential store",
		Long: `Import a previously-obtained user OAuth token (access + refresh)
into the CLI's keychain, bypassing the interactive device-flow login.

This is intended for automation/CI and agent harnesses that obtain tokens
through a server-side OAuth exchange (e.g. authorization-code flow) and
need to provision lark-cli with the resulting credentials non-interactively.

The command performs no network calls; it writes the supplied tokens directly
into the local keychain and updates the active profile's user list. It
operates on the currently configured app/profile (use 'lark-cli profile use'
to switch first).

Required flags: --access-token, --refresh-token, --open-id, --expires-in.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode := f.ResolveStrictMode(cmd.Context()); mode == core.StrictModeBot {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"strict mode is %q, user login is disabled in this profile", mode).
					WithHint("if the user explicitly wants to switch to user identity, see `lark-cli config strict-mode --help` (confirm with the user before switching; switching does NOT require re-bind)")
			}
			opts.Ctx = cmd.Context()

			if opts.AccessToken == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--access-token is required").WithParam("--access-token")
			}
			if opts.RefreshToken == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--refresh-token is required").WithParam("--refresh-token")
			}
			if opts.OpenID == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--open-id is required").WithParam("--open-id")
			}
			if opts.ExpiresIn <= 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--expires-in must be a positive integer (seconds)").WithParam("--expires-in")
			}
			if cmd.Flags().Changed("refresh-expires-in") && opts.RefreshExpiresIn <= 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--refresh-expires-in must be a positive integer (seconds) when specified").WithParam("--refresh-expires-in")
			}
			if opts.UserName == "" {
				opts.UserName = "(imported)"
			}

			if runF != nil {
				return runF(opts)
			}
			return authSetTokenRun(opts)
		},
	}
	cmdutil.SetSupportedIdentities(cmd, []string{"user"})
	cmdutil.SetRisk(cmd, "write")

	cmd.Flags().StringVar(&opts.AccessToken, "access-token", "", "user access token (required)")
	cmd.Flags().StringVar(&opts.RefreshToken, "refresh-token", "", "user refresh token (required)")
	cmd.Flags().StringVar(&opts.OpenID, "open-id", "", "user open_id (required)")
	cmd.Flags().StringVar(&opts.UserName, "user-name", "", "display name for the user (default: \"(imported)\")")
	cmd.Flags().IntVar(&opts.ExpiresIn, "expires-in", 0, "access-token lifetime in seconds (required)")
	cmd.Flags().IntVar(&opts.RefreshExpiresIn, "refresh-expires-in", defaultRefreshExpiresIn,
		fmt.Sprintf("refresh-token lifetime in seconds (default: %d = 30 days)", defaultRefreshExpiresIn))
	cmd.Flags().StringVar(&opts.Scope, "scope", "", "space-separated list of granted scopes")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "structured JSON output")

	return cmd
}

// authSetTokenRun executes the set-token command logic.
func authSetTokenRun(opts *SetTokenOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	appID := config.AppID
	if appID == "" {
		return errs.NewConfigError(errs.SubtypeNotConfigured,
			"no app ID configured; run `lark-cli config init` first")
	}
	profileName := config.ProfileName

	// Snapshot any previously-stored token so we can roll back if profile
	// update fails (avoids leaving a half-imported state).
	previousToken := larkauth.GetStoredToken(appID, opts.OpenID)

	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:       opts.OpenID,
		AppId:            appID,
		AccessToken:      opts.AccessToken,
		RefreshToken:     opts.RefreshToken,
		ExpiresAt:        now + int64(opts.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(opts.RefreshExpiresIn)*1000,
		Scope:            opts.Scope,
		GrantedAt:        now,
	}

	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "failed to save token: %v", err).WithCause(err)
	}

	if err := syncLoginUserToProfile(profileName, appID, opts.OpenID, opts.UserName); err != nil {
		// Roll back: restore the previous token if one existed, otherwise
		// remove the half-imported one.
		if previousToken != nil {
			_ = larkauth.SetStoredToken(previousToken)
		} else {
			_ = larkauth.RemoveStoredToken(appID, opts.OpenID)
		}
		return errs.NewInternalError(errs.SubtypeStorage, "failed to update profile: %v", err).WithCause(err)
	}

	if opts.JSON {
		payload := map[string]interface{}{
			"event":        "token_imported",
			"user_open_id": opts.OpenID,
			"user_name":    opts.UserName,
			"app_id":       appID,
			"scope":        opts.Scope,
			"expires_at":   storedToken.ExpiresAt,
		}
		encoder := json.NewEncoder(f.IOStreams.Out)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(payload); err != nil {
			return errs.NewInternalError(errs.SubtypeSDKError, "failed to write JSON output: %v", err).WithCause(err)
		}
		return nil
	}

	fmt.Fprintln(f.IOStreams.ErrOut)
	output.PrintSuccess(f.IOStreams.ErrOut,
		fmt.Sprintf("Token imported for user %s (%s).", opts.UserName, opts.OpenID))
	fmt.Fprintf(f.IOStreams.ErrOut, "App ID: %s\n", appID)
	if opts.Scope != "" {
		fmt.Fprintf(f.IOStreams.ErrOut, "Scopes: %s\n", opts.Scope)
	}
	return nil
}
