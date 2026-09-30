// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"net/http"
	"strconv"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleMemberList = common.Shortcut{
	Service:     "base",
	Command:     "+role-member-list",
	Description: "List collaborators (members) of a custom role in a Base",
	Risk:        "read",
	Scopes:      []string{"base:collaborator:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "role-id", Desc: "custom role ID (e.g. rolxxxxxx)", Required: true},
		{Name: "limit", Aliases: []string{"page-size"}, Type: "int", Default: "100", Desc: "pagination size, range 1-100"},
		{Name: "page-token", Desc: "pagination token from the previous page's page_token"},
	},
	Tips: []string{
		"Requires advanced permissions to be enabled and the caller to be a Base admin/manager.",
		"Returns each member's open_id/user_id/chat_id/department_id, member_name and member_type (the collaborator category: user/chat/department), plus has_more, page_token and total.",
		"The Base owner is not listed as a role member: owners bypass role permissions, so adding an owner returns success but the owner never appears here.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, pageSize, _, err := readRoleMemberListSpec(runtime)
		if err != nil {
			return err
		}
		if pageSize < 0 || pageSize > 100 {
			return baseFlagErrorf("--limit must be in range 1-100, got %d", pageSize)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		baseToken, roleID, pageSize, pageToken, err := readRoleMemberListSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dr := common.NewDryRunAPI().
			GET("/open-apis/bitable/v1/apps/:base_token/roles/:role_id/members").
			Set("base_token", baseToken).
			Set("role_id", roleID)
		params := map[string]interface{}{}
		if pageSize > 0 {
			params["page_size"] = pageSize
		}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		if len(params) > 0 {
			dr.Params(params)
		}
		return dr
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken, roleID, pageSize, pageToken, err := readRoleMemberListSpec(runtime)
		if err != nil {
			return err
		}

		query := larkcore.QueryParams{}
		if pageSize > 0 {
			query.Set("page_size", strconv.Itoa(pageSize))
		}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodGet,
			ApiPath:     roleMemberAPIPath(baseToken, roleID, ""),
			QueryParams: query,
		})
		if err != nil {
			return err
		}
		return handleRoleAPIResponse(runtime, apiResp, "list role members failed")
	},
}
