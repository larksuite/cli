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

// OIDCLoginOptions OIDC 登录选项
type OIDCLoginOptions struct {
	AuthProtocol string
	Factory      *cmdutil.Factory
	Ctx          context.Context
	JSON         bool
	Scope        string
	RedirectURI  string
	Timeout      int
	Prompt       string // 登录提示参数
	AppID        string
	AppSecret    string
	Domain       string
}

// NewCmdAuthLoginOIDC 创建 OIDC 登录子命令
func NewCmdAuthLoginOIDC(f *cmdutil.Factory, runF func(*OIDCLoginOptions) error) *cobra.Command {
	opts := &OIDCLoginOptions{
		Factory: f,
	}

	cmd := &cobra.Command{
		Use:   "login-oidc",
		Short: "Authenticate using OIDC (OpenID Connect)",
		Long:  "Authenticate with the server using OpenID Connect (OIDC) protocol for enhanced user identity verification.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Ctx = cmd.Context()

			if runF != nil {
				return runF(opts)
			}
			return authLoginWithOIDCCommand(opts)
		},
	}

	cmd.Flags().StringVar(&opts.AuthProtocol, "auth-protocol", "", "per-invocation user protocol override: legacy_oidc")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print session data in JSON format to stdout")
	cmd.Flags().StringVar(&opts.Scope, "scope", "openid email profile", "Scopes to request during login")
	cmd.Flags().StringVar(&opts.RedirectURI, "redirect-uri", "http://localhost:3000/callback", "Callback URL for the OIDC flow")
	cmd.Flags().IntVar(&opts.Timeout, "timeout", 120, "Authentication flow timeout in seconds")
	cmd.Flags().StringVar(&opts.Prompt, "prompt", "", "Specify the prompt options sent to the OIDC provider")
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID for the OIDC provider")
	cmd.Flags().StringVar(&opts.AppSecret, "app-secret", "", "App secret for the OIDC provider")
	cmd.Flags().StringVar(&opts.Domain, "domain", "", "Domain for the OIDC provider")

	// 标记为必需的参数

	return cmd
}

// authLoginWithOIDCCommand executes the OIDC login flow
func authLoginWithOIDCCommand(opts *OIDCLoginOptions) error {
	f := opts.Factory

	config, err := f.Config()
	if err != nil {
		return err
	}

	oidcOpts, err := oidcLoginFlowOptions(opts, config)
	if err != nil {
		return err
	}
	appID := oidcOpts.AppID

	httpClient, err := f.HttpClient()
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %w", err)
	}

	ctx, cancel := context.WithTimeout(opts.Ctx, oidcOpts.Timeout)
	defer cancel()

	result, err := larkauth.StartOIDCFlow(ctx, oidcOpts, httpClient, f.IOStreams.ErrOut)
	if err != nil {
		return output.ErrAuth("OIDC authentication failed: %v", err)
	}

	// Store token using the UpdateFromOIDCResult helper
	storedToken := &larkauth.StoredUAToken{
		AppId:      appID,
		UserOpenId: result.OpenID,
	}
	storedToken.UpdateFromOIDCResult(result)

	err = larkauth.SetStoredToken(storedToken)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "failed to store token: %v", err)
	}

	// Update config
	if err := core.MergeAppUser(appID, core.AppUser{UserOpenId: result.OpenID, UserName: result.UserName}); err != nil {
		return output.Errorf(output.ExitInternal, "internal", "token stored but config merge failed: %v", err)
	}

	if opts.JSON {
		// 输出 JSON 格式的结果，包含 ID Token 相关信息
		outputData := map[string]interface{}{
			"status":     "success",
			"app_id":     appID,
			"user_id":    result.OpenID,
			"user_name":  result.UserName,
			"email":      result.Email,
			"scope":      result.Scope,
			"expires_in": result.ExpiresIn,
			"token_type": "Bearer",
		}

		output.PrintJson(f.IOStreams.Out, outputData)
	} else {
		output.PrintSuccess(f.IOStreams.ErrOut, fmt.Sprintf("Successfully authenticated using OIDC for app %s", appID))

		// 输出用户详细信息
		fmt.Fprintf(f.IOStreams.Out, "\nUser Information:\n")
		fmt.Fprintf(f.IOStreams.Out, "  User ID:   %s\n", result.OpenID)
		fmt.Fprintf(f.IOStreams.Out, "  Name:      %s\n", result.UserName)
		fmt.Fprintf(f.IOStreams.Out, "  Email:     %s\n", result.Email)

		// 输出 Token 状态
		fmt.Fprintf(f.IOStreams.Out, "\nToken Information:\n")
		fmt.Fprintf(f.IOStreams.Out, "  Access Token expires in:  %d seconds (%.1f minutes)\n",
			result.ExpiresIn, float64(result.ExpiresIn)/60.0)
		fmt.Fprintf(f.IOStreams.Out, "  Refresh Token expires in: %d seconds (%.1f days)\n",
			result.RefreshExpiresIn, float64(result.RefreshExpiresIn)/3600.0/24.0)

	}

	return nil
}

func oidcLoginFlowOptions(opts *OIDCLoginOptions, config *core.CliConfig) (*larkauth.OIDCFlowOptions, error) {
	endpointConfig := *config
	if opts.Domain != "" {
		endpointConfig.APIBaseURL = opts.Domain
		endpointConfig.AuthorizationBaseURL = opts.Domain
	}
	if opts.AuthProtocol != "" {
		if opts.AuthProtocol != "legacy_oidc" {
			return nil, output.ErrValidation("unsupported OIDC protocol override")
		}
		endpointConfig.AuthProtocol = opts.AuthProtocol
	}
	endpoints, err := endpointConfig.ResolveServiceEndpoints()
	if err != nil {
		return nil, err
	}
	appID, secret := opts.AppID, opts.AppSecret
	if appID == "" {
		appID = config.AppID
	}
	if secret == "" {
		secret = config.AppSecret
	}
	return &larkauth.OIDCFlowOptions{AuthProtocol: endpointConfig.AuthProtocol, AppID: appID, AppSecret: secret, Domain: endpoints.Accounts, OpenDomain: endpoints.Open, RedirectURI: opts.RedirectURI, Timeout: time.Duration(opts.Timeout) * time.Second}, nil
}
