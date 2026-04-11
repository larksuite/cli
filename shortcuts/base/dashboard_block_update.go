// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-update",
	Description: "Update a dashboard block",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		blockIDFlag(true),
		{Name: "name", Desc: "new block name"},
		{Name: "data-config", Desc: "data config JSON. For chart types: table_name, series|count_all, group_by, filter. For text type: text (markdown supported). See dashboard-block-data-config.md for details."},
		{Name: "user-id-type", Desc: "user ID type: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config validation"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		if runtime.Bool("no-validate") {
			return nil
		}
		raw := runtime.Str("data-config")
		if strings.TrimSpace(raw) == "" {
			return nil
		}
		cfg, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		norm := normalizeDataConfig(cfg)
		// update 时不做强类型校验（不传 type），让后端验证具体字段
		b, _ := json.Marshal(norm)
		_ = runtime.Cmd.Flags().Set("data-config", string(b))
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		pc := newParseCtx(runtime)
		body := map[string]interface{}{}
		if name := runtime.Str("name"); name != "" {
			body["name"] = name
		}
		if raw := runtime.Str("data-config"); raw != "" {
			if parsed, err := parseJSONObject(pc, raw, "data-config"); err == nil {
				body["data_config"] = parsed
			}
		}
		params := map[string]interface{}{}
		if uid := runtime.Str("user-id-type"); uid != "" {
			params["user_id_type"] = uid
		}
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
			Params(params).
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id")).
			Set("block_id", runtime.Str("block-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockUpdate(runtime)
	},
}
