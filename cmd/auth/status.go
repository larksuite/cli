// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
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

	cmdutil.NoFileCompletion(cmd)
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

	if config.UserOpenId == "" {
		result["identity"] = "bot"
		result["note"] = "No user logged in. Only bot (tenant) identity is available for API calls. Run `lark-cli auth login` to log in."
		output.PrintJson(f.IOStreams.Out, result)
		return nil
	}

	stored := larkauth.GetStoredToken(config.AppID, config.UserOpenId)
	if stored == nil {
		result["identity"] = "bot"
		result["userName"] = config.UserName
		result["userOpenId"] = config.UserOpenId
		result["note"] = "Token does not exist or has been cleared. Only bot (tenant) identity is available. Re-login: lark-cli auth login"
		output.PrintJson(f.IOStreams.Out, result)
		return nil
	}

	status := larkauth.TokenStatus(stored)
	if status == "expired" {
		result["identity"] = "bot"
		result["note"] = "User token has expired. Only bot (tenant) identity is available. Re-login: lark-cli auth login"
	} else {
		result["identity"] = "user"
	}
	result["userName"] = config.UserName
	result["userOpenId"] = config.UserOpenId
	result["tokenStatus"] = status
	result["scope"] = stored.Scope
	result["expiresAt"] = time.UnixMilli(stored.ExpiresAt).Format(time.RFC3339)
	result["refreshExpiresAt"] = time.UnixMilli(stored.RefreshExpiresAt).Format(time.RFC3339)
	result["grantedAt"] = time.UnixMilli(stored.GrantedAt).Format(time.RFC3339)

	// --verify: call the server to confirm token is actually usable.
	if opts.Verify && status != "expired" {
		verified, verifyErr := verifyTokenOnServer(f, config)
		result["verified"] = verified
		if verifyErr != "" {
			result["verifyError"] = verifyErr
		}
	}

	output.PrintJson(f.IOStreams.Out, result)
	return nil
}

// verifyTokenOnServer obtains a valid access token (refreshing if needed)
// and calls /authen/v1/user_info to confirm the server accepts it.
// Returns (true, "") on success or (false, reason) on failure.
func verifyTokenOnServer(f *cmdutil.Factory, config *core.CliConfig) (bool, string) {
	httpClient, err := f.HttpClient()
	if err != nil {
		return false, "failed to create HTTP client: " + err.Error()
	}

	token, err := larkauth.GetValidAccessToken(httpClient, larkauth.NewUATCallOptions(config, f.IOStreams.ErrOut))
	if err != nil {
		return false, "token unusable: " + err.Error()
	}

	sdk, err := f.LarkClient()
	if err != nil {
		return false, "failed to create SDK client: " + err.Error()
	}

	if err := larkauth.VerifyUserToken(context.Background(), sdk, token); err != nil {
		return false, "server rejected token: " + err.Error()
	}

	return true, ""
}
