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
		{Name: "linkage", Desc: `same-datasource chart linkage JSON or @file: {"target_block_ids":["<block_id>"]}. Replaces the source's complete target list; [] clears it; omit to preserve. field_mappings is not supported. Datasource equality is validated by the API`},
		{Name: "position", Desc: `optional new placement for this block. Moving or resizing blocks is part of the dashboard layout: plan every block's coordinates and finish with +dashboard-layout carrying all of them (it writes the changed blocks and runs the server's tidy pass), so you normally put the new coordinates there instead of here. Component position+size in the 48-col grid, JSON {"x","y","w","h"}; all four keys required and numeric (position is submitted whole, so a partial object cannot express a complete placement). Advisory bounds x/y>=0, 1<=w<=48 and x+w<=48, h>=1 — coordinate VALUES are not validated locally and pass through as given. Omit to leave layout unchanged`},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local SEMANTIC validation: data_config checks + normalization, and the --position x/y/w/h completeness check. JSON syntax and linkage constraints are always checked. Sends data_config and position as-is"},
		{Name: "yes", Type: "bool", Hidden: true, Desc: "accepted for compatibility; has no effect"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --name "Total Sales"`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"series":[{"field_name":"Amount","rollup":"SUM"}]}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"number_format":{"formatName":"dollar_rounded","precision":0}}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --position '{"x":12,"y":0,"w":36,"h":12}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <ranking_block_id> --data-config '{"limit_size":20}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <slicer_block_id> --data-config '{"order":"desc"}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <slicer_block_id> --data-config '{"filter":null}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <countdown_block_id> --data-config '{"targetTime":1798732800000}'`,
		"Read lark-base-dashboard-block-config.md as the SSOT for data_config templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"For non-view blocks, use +dashboard-block-get first to inspect the current data_config before replacing nested values. For view blocks, GET returns tableId/viewId/viewType references; use the view configuration getters described below.",
		"For chart linkage, read lark-base-dashboard-linkage.md first. Update the source block with the complete target list; adding one target also requires retaining the existing targets. Targets must use the same datasource; dry-run checks the request shape only.",
		"Block type cannot be changed; countdown data_config.type can switch between fixed and field with a complete target-mode configuration.",
		"Ordinary chart data_config updates merge top-level keys; arrays replace whole. number_format and supported viewModel/overrideViewModel fields merge by leaf, preserving omitted fields.",
		"Slicer updates preserve omitted fields and never add create defaults. On the same table, omit filter to preserve it or send filter:null to clear it. Changing tables requires a new group_by and clears the old filter unless a new filter is supplied.",
		"Countdown patches keep omitted fields and merge the sole additionalDatasources entry by field. groupBy and series replace whole arrays; same-mode/same-table filter omission keeps it, an object replaces it, and null clears it. Changing tableName requires a complete new datasource. Read the countdown section of lark-base-dashboard-block-config.md.",
		`For progress, {"progress":{"target":{"value":200}}} updates an existing custom target. current/target subfields merge server-side and arrays replace whole. Omitted fields are preserved; field-source filter:null clears a filter. Source or mode changes require complete target configuration. See lark-base-dashboard-progress.md.`,
		"For view blocks, pass the returned tableId/viewId as --table-id/--view-id to +view-get-filter, +view-get-sort, +view-get-group, or +view-get-visible-fields to inspect persisted configuration. Submit only the changed data_config fields, such as sort_by or visible_fields; do not resend the full configuration because block GET omits the write fields. The server validates the patch against the existing component. Read lark-base-dashboard-view.md for update and readback guidance.",
		"Dashboard block create/update reject top-level data_config.timebar and data_config.card, including null, unless --no-validate is set. This does not restrict view_type or change reads of existing blocks.",
		"--position is optional (48-col grid, 1<=w<=48, x+w<=48); omit it to leave the current layout unchanged. Whether or not you pass it, a layout change ends with +dashboard-layout carrying the coordinates of every block on the dashboard.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		pc := newParseCtx(runtime)
		if err := validateDashboardChartLinkage(pc, runtime); err != nil {
			return err
		}
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
			if err := validateDashboardBlockWriteConfig(cfg); err != nil {
				return err
			}
			effective = normalizePivotTableDataConfig(normalizeDataConfig(cfg), false)
			if isMultiSourceDataConfig(cfg) {
				effective = normalizeMultiSourceDataConfig(cfg)
			}
			if problems := validateNativeChartDataConfig(effective); len(problems) > 0 {
				return formatDataConfigErrors(problems)
			}
			// Update 不传 type，因此只执行与组件类型无关的局部校验；全量校验
			// 会误报局部 patch 未提交的 table_name/series 等字段。
			problems := validateBlockFilter(effective, "filter", true)
			if rawNumberFormat, hasNumberFormat := effective["number_format"]; hasNumberFormat {
				problems = append(problems, validateNumberFormat(rawNumberFormat)...)
			}
			if len(problems) > 0 {
				return formatDataConfigErrors(problems)
			}
			if isPivotTableDataConfig(effective) {
				if problems := validatePivotTableDataConfig(chartDataWithoutNativeStyle(effective), false); len(problems) > 0 {
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
