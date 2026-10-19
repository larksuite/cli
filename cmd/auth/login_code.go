// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// LoginCodeOptions holds options for auth login-code command.
type LoginCodeOptions struct {
	Factory     *cmdutil.Factory
	Ctx         context.Context
	RedirectURI string
	Scope       string
	Timeout     int
}

// NewCmdAuthLoginCode creates the auth login-code subcommand.
func NewCmdAuthLoginCode(f *cmdutil.Factory, runF func(*LoginCodeOptions) error) *cobra.Command {
	opts := &LoginCodeOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "login-code",
		Short: "Authorization Code Flow login",
		Long:  `Login using Authorization Code Flow with local callback server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()
			if runF != nil {
				return runF(opts)
			}
			return authLoginCodeRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.RedirectURI, "redirect-uri", "http://localhost:3000/callback", "OAuth callback URI")
	cmd.Flags().StringVar(&opts.Scope, "scope", "", "OAuth scope (empty by default)")
	cmd.Flags().IntVar(&opts.Timeout, "timeout", 300, "timeout in seconds")

	return cmd
}

func authLoginCodeRun(opts *LoginCodeOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	httpClient, err := f.HttpClient()
	if err != nil {
		return err
	}

	// Get domain from Brand (which may be a custom domain URL)
	endpoints, err := config.ResolveServiceEndpoints()
	if err != nil {
		return err
	}

	flowOpts := &larkauth.AuthCodeFlowOptions{
		AppID:       config.AppID,
		AppSecret:   config.AppSecret,
		Domain:      endpoints.Open,
		RedirectURI: opts.RedirectURI,
		Scope:       opts.Scope,
		Timeout:     time.Duration(opts.Timeout) * time.Second,
	}

	result, err := larkauth.StartAuthCodeFlow(opts.Ctx, flowOpts, httpClient, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("authorization failed: %v", err)
	}

	// Store token
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:   result.OpenID,
		AppId:        config.AppID,
		AuthProtocol: "oauth_v2", Authority: endpoints.Open,
		AccessToken:      result.AccessToken,
		RefreshToken:     result.RefreshToken,
		ExpiresAt:        now + int64(result.ExpiresIn)*1000,
		RefreshExpiresAt: now + int64(result.RefreshExpiresIn)*1000,
		Scope:            result.Scope,
		GrantedAt:        now,
	}
	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	// Update config
	if err := core.MergeAppUser(config.AppID, core.AppUser{UserOpenId: result.OpenID, UserName: result.UserName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Login successful: %s (%s)", result.UserName, result.OpenID))
	return nil
}
