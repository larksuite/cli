// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// StoreTokenOptions holds all inputs for auth store-token.
type StoreTokenOptions struct {
	AuthProtocol     string
	Factory          *cmdutil.Factory
	UserOpenID       string
	UserName         string
	AccessToken      string
	RefreshToken     string
	ExpiresIn        int64
	RefreshExpiresIn int64
	Scope            string
}

// NewCmdAuthStoreToken creates the auth store-token subcommand.
func NewCmdAuthStoreToken(f *cmdutil.Factory, runF func(*StoreTokenOptions) error) *cobra.Command {
	opts := &StoreTokenOptions{Factory: f}

	cmd := &cobra.Command{
		Use:   "store-token",
		Short: "Store a user access token into local auth state",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return authStoreTokenRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.UserOpenID, "user-id", "", "user OpenID to bind this token to")
	cmd.Flags().StringVar(&opts.UserOpenID, "user-open-id", "", "user OpenID to bind this token to")
	cmd.Flags().StringVar(&opts.UserName, "user-name", "", "user display name")
	cmd.Flags().StringVar(&opts.AccessToken, "access-token", "", "user access token")
	cmd.Flags().StringVar(&opts.RefreshToken, "refresh-token", "", "user refresh token")
	cmd.Flags().Int64Var(&opts.ExpiresIn, "expires-in", 0, "access token lifetime in seconds")
	cmd.Flags().Int64Var(&opts.RefreshExpiresIn, "refresh-expires-in", 0, "refresh token lifetime in seconds")
	cmd.Flags().StringVar(&opts.AuthProtocol, "auth-protocol", "", "explicit credential protocol: legacy_oidc or oauth_v2")
	cmd.Flags().StringVar(&opts.Scope, "scope", "", "granted scopes")

	_ = cmd.MarkFlagRequired("user-name")
	_ = cmd.MarkFlagRequired("access-token")
	_ = cmd.MarkFlagRequired("refresh-token")
	_ = cmd.MarkFlagRequired("expires-in")
	_ = cmd.MarkFlagRequired("refresh-expires-in")

	return cmd
}

func authStoreTokenRun(opts *StoreTokenOptions) error {
	if opts.UserOpenID == "" {
		return output.ErrValidation("please specify --user-id or --user-open-id")
	}
	if opts.ExpiresIn <= 0 {
		return output.ErrValidation("--expires-in must be greater than 0")
	}
	if opts.RefreshExpiresIn <= 0 {
		return output.ErrValidation("--refresh-expires-in must be greater than 0")
	}

	config, err := opts.Factory.Config()
	if err != nil {
		return err
	}

	protocol := opts.AuthProtocol
	if protocol == "" {
		protocol = config.AuthProtocol
	}
	if protocol == "" {
		if config.Brand == core.BrandFeishu || config.Brand == core.BrandLark || config.Brand == "" {
			protocol = "oauth_v2"
		} else {
			return output.ErrValidation("private credentials require explicit --auth-protocol")
		}
	}
	if protocol != "legacy_oidc" && protocol != "oauth_v2" {
		return output.ErrValidation("unsupported auth protocol")
	}
	endpoints, err := config.ResolveServiceEndpoints()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	storedToken := &larkauth.StoredUAToken{
		UserOpenId:   opts.UserOpenID,
		AppId:        config.AppID,
		AuthProtocol: protocol, Authority: endpoints.Open,
		AccessToken:      opts.AccessToken,
		RefreshToken:     opts.RefreshToken,
		ExpiresAt:        now + opts.ExpiresIn*1000,
		RefreshExpiresAt: now + opts.RefreshExpiresIn*1000,
		Scope:            opts.Scope,
		GrantedAt:        now,
	}
	if err := larkauth.SetStoredToken(storedToken); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to save token: %v", err)
	}

	if err := core.MergeAppUser(config.AppID, core.AppUser{UserOpenId: opts.UserOpenID, UserName: opts.UserName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	output.PrintSuccess(opts.Factory.IOStreams.ErrOut, fmt.Sprintf("Stored token for %s (%s)", opts.UserName, opts.UserOpenID))
	return nil
}
