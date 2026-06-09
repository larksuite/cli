// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockCreate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-create",
	Description: "Create a block in a dashboard",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:create"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "name", Desc: "block name", Required: true},
		{Name: "type", Desc: "block type: column(柱状图)|bar(条形图)|line(折线图)|pie(饼图)|ring(环形图)|area(面积图)|combo(组合图)|scatter(散点图)|funnel(漏斗图)|wordCloud(词云)|radar(雷达图)|statistics(指标卡)|text(文本). Read dashboard-block-data-config.md before creating.", Required: true},
		{Name: "data-config", Desc: "data_config JSON object; read dashboard-block-data-config.md for the SSOT"},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config validation"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Order Count" --type statistics --data-config '{"table_name":"Orders","count_all":true}'`,
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Dashboard Note" --type text --data-config '{"text":"# Sales Dashboard"}'`,
		"Before creating data-backed blocks, use +table-list and +field-list to confirm real table and field names.",
		"data_config uses table and field names, not table_id or field_id.",
		"Read dashboard-block-data-config.md as the SSOT for chart templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"Record the returned block_id; block update/delete/get-data commands need it.",
		"Create dashboard blocks sequentially; do not parallelize multiple block creates for the same dashboard.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		if runtime.Bool("no-validate") {
			return nil
		}
		raw := runtime.Str("data-config")
		if strings.TrimSpace(raw) == "" {
			// text 类型必须提供 data-config（含 text 内容）
			if strings.ToLower(runtime.Str("type")) == "text" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "text 类型组件必须提供 data-config，包含必填字段 text").WithParam("--data-config")
			}
			return nil
		}
		cfg, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		norm := normalizeDataConfig(cfg)
		if errs := validateBlockDataConfig(runtime.Str("type"), norm); len(errs) > 0 {
			return formatDataConfigErrors(errs)
		}
		// 用规范化后的 JSON 覆写 flag，确保后续透传一致
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
		if t := runtime.Str("type"); t != "" {
			body["type"] = t
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
			POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
			Params(params).
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockCreate(runtime)
	},
}
