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
		{Name: "type", Desc: "block type: column(柱状图)|bar(条形图)|line(折线图)|pie(饼图)|ring(环形图)|area(面积图)|combo(组合图)|scatter(散点图)|funnel(漏斗图)|wordCloud(词云)|radar(雷达图)|statistics(指标卡)|text(文本). Read lark-base-dashboard-block-config.md before creating.", Required: true},
		{Name: "data-config", Desc: "data_config JSON object; read lark-base-dashboard-block-config.md for the SSOT"},
		{Name: "position", Desc: `optional. component position+size in 12-col grid, JSON {"x","y","w","h"}; all four keys required and numeric (position is submitted whole, so a partial object cannot express a complete placement). Advisory bounds x/y>=0, 1<=w<=12 and x+w<=12, h>=1 — coordinate VALUES are not validated locally and pass through as given; the server auto-arranges out-of-range or overlapping positions. Omit for server auto-layout`},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local SEMANTIC validation: data_config checks + normalization, and the --position x/y/w/h completeness check. JSON syntax is still parsed (a malformed value never silently vanishes from the preview). Sends data_config and position as-is"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Order Count" --type statistics --data-config '{"table_name":"Orders","count_all":true}'`,
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Revenue" --type statistics --data-config '{"table_name":"Orders","series":[{"field_name":"Amount","rollup":"SUM"}],"number_format":{"formatName":"dollar_rounded","precision":2}}'`,
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Dashboard Note" --type text --data-config '{"text":"# Sales Dashboard"}'`,
		`lark-cli base +dashboard-block-create --base-token <base_token> --dashboard-id <dashboard_id> --name "Order Count" --type statistics --data-config '{"table_name":"Orders","count_all":true}' --position '{"x":0,"y":0,"w":6,"h":4}'`,
		"Before creating data-backed blocks, use +table-list and +field-list to confirm real table and field names.",
		"data_config uses table and field names, not table_id or field_id.",
		"Read lark-base-dashboard-block-config.md as the SSOT for chart templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"--position is optional precise layout in a 12-col grid; omit it to let the server auto-layout. Coordinate values are not validated locally; the server auto-arranges out-of-range or overlapping positions. To re-tidy an existing dashboard use +dashboard-arrange instead.",
		"For funnel/stage charts backed by ordered helper data, set the intended group_by.sort in the initial create request; do not create first and then issue a second update just to fix sorting.",
		"Record the returned block_id; block update/delete/get-data commands need it.",
		"Create dashboard blocks sequentially; do not parallelize multiple block creates for the same dashboard.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		if err := validateDashboardBlockPosition(pc, runtime); err != nil {
			return err
		}
		raw := strings.TrimSpace(runtime.Str("data-config"))
		if raw == "" {
			// text 类型必须提供 data-config（含 text 内容）
			if !runtime.Bool("no-validate") && strings.EqualFold(strings.TrimSpace(runtime.Str("type")), "text") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "text 类型组件必须提供 data-config，包含必填字段 text").WithParam("--data-config")
			}
			return nil
		}
		cfg, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		effective := cfg
		if !runtime.Bool("no-validate") {
			effective = normalizeDataConfig(cfg)
			if errs := validateBlockDataConfig(runtime.Str("type"), effective); len(errs) > 0 {
				return formatDataConfigErrors(errs)
			}
		}
		// Fold @file input into inline JSON after the first successful parse.
		// DryRun/Execute must not reopen a file that may have changed.
		b, _ := json.Marshal(effective)
		_ = runtime.Cmd.Flags().Set("data-config", string(b))
		return nil
	},
	DryRun: dryRunDashboardBlockCreate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockCreate(runtime)
	},
}
