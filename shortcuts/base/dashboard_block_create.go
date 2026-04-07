// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

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
		{Name: "type", Desc: "block type: column(柱状图)|bar(条形图)|line(折线图)|pie(饼图)|ring(环形图)|area(面积图)|combo(组合图)|scatter(散点图)|funnel(漏斗图)|wordCloud(词云)|radar(雷达图)|statistics(指标卡). Read dashboard-block-data-config.md before creating.", Required: true},
		{Name: "data-config", Desc: "data config JSON object (table_name, series, count_all, group_by, filter, etc.)"},
		{Name: "user-id-type", Desc: "user ID type: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config validation"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("no-validate") {
			return nil
		}
		raw := runtime.Str("data-config")
		if strings.TrimSpace(raw) == "" {
			return nil // 允许无 data_config 的创建（某些类型可先创建后配置）
		}
		cfg, err := parseJSONObject(raw, "data-config")
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
		body := map[string]interface{}{}
		if name := runtime.Str("name"); name != "" {
			body["name"] = name
		}
		if t := runtime.Str("type"); t != "" {
			body["type"] = t
		}
		if raw := runtime.Str("data-config"); raw != "" {
			if parsed, err := parseJSONObject(raw, "data-config"); err == nil {
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
