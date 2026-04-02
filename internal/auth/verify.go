// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

// VerifyUserToken calls /authen/v1/user_info to confirm the token is accepted server-side.
// Returns nil on success or an error describing why the server rejected the token.
func VerifyUserToken(ctx context.Context, sdk *lark.Client, accessToken string) error {
	apiResp, err := sdk.Do(ctx, &larkcore.ApiReq{
		HttpMethod:                http.MethodGet,
		ApiPath:                   PathUserInfoV1,
		SupportedAccessTokenTypes: []larkcore.AccessTokenType{larkcore.AccessTokenTypeUser},
	}, larkcore.WithUserAccessToken(accessToken))
	if err != nil {
		return err
	}
	logSDKResponse(PathUserInfoV1, apiResp)

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
		return fmt.Errorf("failed to parse response: %v", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf("[%d] %s", resp.Code, resp.Msg)
	}
	return nil
}
