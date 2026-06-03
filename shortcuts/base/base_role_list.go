// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleList = common.Shortcut{
	Service:     "base",
	Command:     "+role-list",
	Description: "List all roles in a Base",
	Risk:        "read",
	Scopes:      []string{"base:role:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
	},
	Tips: []string{
		"Requires advanced permissions to be enabled and the caller to be a Base admin.",
		"Returns role summaries; use +role-get for the full permission config.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/roles").
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/base/v3/bases/%s/roles", validate.EncodePathSegment(baseToken)),
		})
		if err != nil {
			return err
		}

		return handleRoleResponse(runtime, apiResp.RawBody, "list roles failed")
	},
}
