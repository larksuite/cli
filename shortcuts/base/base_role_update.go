// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseRoleUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+role-update",
	Description: "Update a role config (delta merge, only changed fields needed)",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:role:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "role-id", Desc: "role ID (e.g. rolxxxxxx4)", Required: true},
		{Name: "json", Desc: "delta role config JSON; read lark-base-role-guide.md and role-config.md before changing permissions", Required: true},
	},
	Tips: []string{
		baseHighRiskYesTip,
		"Requires advanced permissions to be enabled and the caller to be a Base admin.",
		"Update is a delta merge: only changed fields are updated, others remain unchanged.",
		"Use lark-base-role-guide.md as the entry guide and role-config.md as the role permission JSON SSOT.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return common.FlagErrorf("--base-token must not be blank")
		}
		if strings.TrimSpace(runtime.Str("role-id")) == "" {
			return common.FlagErrorf("--role-id must not be blank")
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(runtime.Str("json")), &body); err != nil {
			return common.FlagErrorf("--json must be valid JSON: %v", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		var body map[string]any
		json.Unmarshal([]byte(runtime.Str("json")), &body)
		return common.NewDryRunAPI().
			Desc("Delta merge: only changed fields are updated, others remain unchanged").
			PUT("/open-apis/base/v3/bases/:base_token/roles/:role_id").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("role_id", runtime.Str("role-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		roleId := runtime.Str("role-id")
		var body map[string]any
		json.Unmarshal([]byte(runtime.Str("json")), &body)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPut,
			ApiPath:    fmt.Sprintf("/open-apis/base/v3/bases/%s/roles/%s", validate.EncodePathSegment(baseToken), validate.EncodePathSegment(roleId)),
			Body:       body,
		})
		if err != nil {
			return err
		}

		return handleRoleResponse(runtime, apiResp.RawBody, "update role failed")
	},
}
