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

var BaseRoleDelete = common.Shortcut{
	Service:     "base",
	Command:     "+role-delete",
	Description: "Delete a custom role (system roles cannot be deleted)",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:role:delete"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "role-id", Desc: "role ID (e.g. rolxxxxxx4)", Required: true},
	},
	Tips: []string{
		baseHighRiskYesTip,
		"Requires advanced permissions to be enabled and the caller to be a Base admin.",
		"Only custom roles can be deleted; system roles cannot be deleted.",
		"Use +role-get first if the role target is ambiguous, then pass --yes to confirm deletion.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("role-id")) == "" {
			return common.FlagErrorf("--role-id must not be blank")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/roles/:role_id").
			Set("base_token", runtime.Str("base-token")).
			Set("role_id", runtime.Str("role-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		roleId := runtime.Str("role-id")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodDelete,
			ApiPath:    fmt.Sprintf("/open-apis/base/v3/bases/%s/roles/%s", validate.EncodePathSegment(baseToken), validate.EncodePathSegment(roleId)),
			Body:       map[string]any{},
		})
		if err != nil {
			return err
		}

		return handleRoleResponse(runtime, apiResp.RawBody, "delete role failed")
	},
}
