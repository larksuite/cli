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

var BaseRoleCreate = common.Shortcut{
	Service:     "base",
	Command:     "+role-create",
	Description: "Create a custom role in a Base",
	Risk:        "write",
	Scopes:      []string{"base:role:create"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "json", Desc: "role config JSON; read lark-base-role-guide.md and role-config.md before constructing permissions", Required: true},
	},
	Tips: []string{
		"Requires advanced permissions to be enabled and the caller to be a Base admin.",
		"Use lark-base-role-guide.md as the entry guide and role-config.md as the role permission JSON SSOT.",
		"Create supports custom_role only.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(runtime.Str("json")), &body); err != nil {
			return baseFlagErrorf("--json must be valid JSON: %v", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		var body map[string]any
		json.Unmarshal([]byte(runtime.Str("json")), &body)
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/roles").
			Body(body).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		baseToken := runtime.Str("base-token")
		var body map[string]any
		json.Unmarshal([]byte(runtime.Str("json")), &body)

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod: http.MethodPost,
			ApiPath:    fmt.Sprintf("/open-apis/base/v3/bases/%s/roles", validate.EncodePathSegment(baseToken)),
			Body:       body,
		})
		if err != nil {
			return err
		}

		return handleRoleAPIResponse(runtime, apiResp, "create role failed")
	},
}
