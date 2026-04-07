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
)

// NewCmdAuth creates the auth command with subcommands.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "OAuth credentials and authorization management",
	}
	cmdutil.DisableAuthCheck(cmd)

	cmd.AddCommand(NewCmdAuthLogin(f, nil))
	cmd.AddCommand(NewCmdAuthLogout(f, nil))
	cmd.AddCommand(NewCmdAuthStatus(f, nil))
	cmd.AddCommand(NewCmdAuthScopes(f, nil))
	cmd.AddCommand(NewCmdAuthList(f, nil))
	cmd.AddCommand(NewCmdAuthCheck(f, nil))
	return cmd
}

// userInfoResponse is the API response for /open-apis/authen/v1/user_info.
type userInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OpenID string `json:"open_id"`
		Name   string `json:"name"`
	} `json:"data"`
}

// getUserInfo fetches the current user's OpenID and name using the given access token.
func getUserInfo(ctx context.Context, sdk *lark.Client, accessToken string) (openId, name string, err error) {
	apiResp, err := sdk.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   larkauth.PathUserInfoV1,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}, larkcore.WithUserAccessToken(accessToken))
	if err != nil {
		return "", "", err
	}

	var resp userInfoResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return "", "", fmt.Errorf("failed to parse user info: %v", err)
	}
	if resp.Code != 0 {
		return "", "", fmt.Errorf("failed to get user info [%d]: %s", resp.Code, resp.Msg)
	}
	if resp.Data.OpenID == "" {
		return "", "", fmt.Errorf("failed to get user info: missing open_id in response")
	}

	name = resp.Data.Name
	if name == "" {
		name = "(unknown)"
	}
	return resp.Data.OpenID, name, nil
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

// getAppInfo queries app info from the Lark API.
func getAppInfo(ctx context.Context, f *cmdutil.Factory, appId string) (*appInfo, error) {
	sdk, err := f.LarkClient()
	if err != nil {
		return nil, err
	}

	queryParams := make(larkcore.QueryParams)
	queryParams.Set("lang", "zh_cn")

	apiResp, err := sdk.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   larkauth.ApplicationInfoPath(appId),
		QueryParams:               queryParams,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeTenant},
	})
	if err != nil {
		return nil, err
	}

	var resp appInfoResponse
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("API error [%d]: %s", resp.Code, resp.Msg)
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
