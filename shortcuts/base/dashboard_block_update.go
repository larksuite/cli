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
		{Name: "data-config", Desc: "data_config JSON object; read lark-base-dashboard-block-config.md for the SSOT"},
		{Name: "position", Desc: `optional. component position+size in 12-col grid, JSON {"x","y","w","h"}; all four keys required and numeric (position is submitted whole, so a partial object cannot express a complete placement). Advisory bounds x/y>=0, 1<=w<=12 and x+w<=12, h>=1 — coordinate VALUES are not validated locally and pass through as given; the server auto-arranges out-of-range or overlapping positions. Omit to leave layout unchanged`},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local SEMANTIC validation: data_config checks + normalization, and the --position x/y/w/h completeness check. JSON syntax is still parsed (a malformed value never silently vanishes from the preview). Sends data_config and position as-is"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --name "Total Sales"`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"series":[{"field_name":"Amount","rollup":"SUM"}]}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"number_format":{"formatName":"dollar_rounded","precision":0}}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --position '{"x":6,"y":0,"w":6,"h":4}'`,
		"Read lark-base-dashboard-block-config.md as the SSOT for data_config templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"Use +dashboard-block-get first to inspect the current data_config before replacing nested values.",
		"Block type cannot be changed; delete and recreate the block to change chart type.",
		"data_config update merges top-level keys; each provided key is normally replaced as a whole, except number_format, whose subfields merge server-side.",
		"--position is optional precise layout in a 12-col grid; omit it to leave the current layout unchanged. Coordinate values are not validated locally; the server auto-arranges out-of-range or overlapping positions. To re-tidy an existing dashboard use +dashboard-arrange instead.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		if err := validateDashboardBlockPosition(pc, runtime); err != nil {
			return err
		}
		raw := strings.TrimSpace(runtime.Str("data-config"))
		if raw == "" {
			return nil
		}
		cfg, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		effective := cfg
		if !runtime.Bool("no-validate") {
			effective = normalizeDataConfig(cfg)
			// update 不传 type，其余字段交给后端按组件现有类型校验。
			// number_format 是例外：它必须和 create 一样在本地拦截，否则同一份
			// 非法取值在 create 报错、在 update 却要等一次网络往返才失败。这里
			// 只复用 number_format 子校验，不走 validateBlockDataConfig 全量分支
			// ——后者会误报 table_name/series 缺失，破坏“只改 number_format”的用法。
			if rawNumberFormat, hasNumberFormat := effective["number_format"]; hasNumberFormat {
				if problems := validateNumberFormat(rawNumberFormat); len(problems) > 0 {
					return formatDataConfigErrors(problems)
				}
			}
		}
		// Fold @file input into inline JSON after the first successful parse.
		// DryRun/Execute must not reopen a file that may have changed.
		b, _ := json.Marshal(effective)
		_ = runtime.Cmd.Flags().Set("data-config", string(b))
		return nil
	},
	DryRun: dryRunDashboardBlockUpdate,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockUpdate(runtime)
	},
}
