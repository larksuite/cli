// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/spf13/cobra"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
)

// NewCmdAuth creates the auth command with subcommands.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "OAuth credentials and authorization management",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// cobra stops at the first PersistentPreRun[E] walking up; root's SilenceUsage=true is skipped without this.
			cmd.SilenceUsage = true
			// Pass "auth" literally; cmd.Name() is the subcommand.
			return f.RequireBuiltinCredentialProvider(cmd.Context(), "auth")
		},
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(NewCmdAuthLogin(f, nil))
	cmd.AddCommand(NewCmdAuthLogout(f, nil))
	cmd.AddCommand(NewCmdAuthStatus(f, nil))
	cmd.AddCommand(NewCmdAuthScopes(f, nil))
	cmd.AddCommand(NewCmdAuthList(f, nil))
	cmd.AddCommand(NewCmdAuthCheck(f, nil))
	cmd.AddCommand(NewCmdAuthQRCode(f, nil))
	cmd.AddCommand(NewCmdAuthUsers(f))
	return cmd
}

// userInfoResponse is the API response for /open-apis/authen/v1/user_info.
type userInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OpenID  string `json:"open_id"`
		UnionID string `json:"union_id"`
		Name    string `json:"name"`
	} `json:"data"`
}

// getUserInfoFn is the test seam for getUserInfo (mirrors getAppInfoFn).
var getUserInfoFn = getUserInfo

// getUserInfo fetches the current user's OpenID, UnionID, and name.
// UnionID is captured for cross-app reconciliation in per-user state files;
// absence is non-fatal.
func getUserInfo(ctx context.Context, sdk *lark.Client, accessToken string) (openId, unionId, name string, err error) {
	apiResp, err := sdk.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   larkauth.PathUserInfoV1,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}, larkcore.WithUserAccessToken(accessToken))
	if err != nil {
		return "", "", "", err
	}

	var resp userInfoResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return "", "", "", fmt.Errorf("failed to parse user info: %w", err)
	}
	if resp.Code != 0 {
		return "", "", "", fmt.Errorf("failed to get user info [%d]: %s", resp.Code, resp.Msg)
	}
	if resp.Data.OpenID == "" {
		return "", "", "", fmt.Errorf("failed to get user info: missing open_id in response")
	}

	name = resp.Data.Name
	if name == "" {
		name = "(unknown)"
	}
	return resp.Data.OpenID, resp.Data.UnionID, name, nil
}

// appInfo contains application information (owner, scopes).
type appInfo struct {
	OwnerOpenId string
	UserScopes  []string
}

// appInfoResponse is the API response for /open-apis/application/v6/applications/:app_id.
type appInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		App struct {
			Owner struct {
				OwnerID string `json:"owner_id"`
			} `json:"owner"`
			CreatorID string `json:"creator_id"`
			Scopes    []struct {
				Scope      string   `json:"scope"`
				TokenTypes []string `json:"token_types"`
			} `json:"scopes"`
		} `json:"app"`
	} `json:"data"`
}

// getAppInfoFn is the test seam for getAppInfo.
var getAppInfoFn = getAppInfo

// getAppInfo queries app info from the Lark API.
func getAppInfo(ctx context.Context, f *cmdutil.Factory, appId string) (*appInfo, error) {
	ac, err := f.NewAPIClient()
	if err != nil {
		return nil, err
	}

	queryParams := make(larkcore.QueryParams)
	queryParams.Set("lang", "zh_cn")

	apiResp, err := ac.DoSDKRequest(ctx, &larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     larkauth.ApplicationInfoPath(appId),
		QueryParams: queryParams,
	}, core.AsBot)
	if err != nil {
		return nil, err
	}

	var resp appInfoResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if resp.Code != 0 {
		return nil, classifyAppInfoErr(apiResp.RawBody, resp.Code, resp.Msg, f, appId)
	}

	app := resp.Data.App
	ownerOpenId := app.Owner.OwnerID
	if ownerOpenId == "" {
		ownerOpenId = app.CreatorID
	}

	var userScopes []string
	for _, s := range app.Scopes {
		if s.Scope == "" || !slices.Contains(s.TokenTypes, "user") {
			continue
		}
		userScopes = append(userScopes, s.Scope)
	}

	return &appInfo{OwnerOpenId: ownerOpenId, UserScopes: userScopes}, nil
}

// classifyAppInfoErr re-decodes the raw body so BuildAPIError sees the
// upstream `error` block — the typed appInfoResponse shape drops it.
func classifyAppInfoErr(rawBody []byte, code int, msg string, f *cmdutil.Factory, appId string) error {
	var raw map[string]any
	_ = json.Unmarshal(rawBody, &raw)
	if raw == nil {
		raw = map[string]any{}
	}
	raw["code"] = code
	raw["msg"] = msg
	cc := errclass.ClassifyContext{Identity: string(core.AsBot)}
	if cfg, _ := f.Config(); cfg != nil {
		cc.Brand = string(cfg.Brand)
		cc.AppID = appId
	}
	return errclass.BuildAPIError(raw, cc)
}
