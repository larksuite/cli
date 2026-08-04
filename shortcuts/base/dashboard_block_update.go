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
		{Name: "position", Desc: `optional. component position+size in 12-col grid, JSON {"x","y","w","h"}; advisory bounds x/y>=0, 1<=w<=12 and x+w<=12, h>=1 (NOT validated locally or server-side, out-of-range passes through); omit to leave layout unchanged`},
		{Name: "user-id-type", Desc: "user ID type for user fields in filters: open_id / union_id / user_id"},
		{Name: "no-validate", Type: "bool", Desc: "skip local data_config validation and normalization; send data_config as-is"},
	},
	Tips: []string{
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --name "Total Sales"`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"series":[{"field_name":"Amount","rollup":"SUM"}]}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --data-config '{"number_format":{"precision":0}}'`,
		`lark-cli base +dashboard-block-update --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --position '{"x":6,"y":0,"w":6,"h":4}'`,
		"Read lark-base-dashboard-block-config.md as the SSOT for data_config templates, filters, metric rules, and type-specific fields; do not invent data_config from natural language.",
		"Use +dashboard-block-get first to inspect the current data_config before replacing nested values.",
		"Block type cannot be changed; delete and recreate the block to change chart type.",
		"data_config update merges top-level keys, but each provided key is replaced as a whole.",
		"--position is optional precise layout in a 12-col grid; omit it to leave the current layout unchanged. Coordinate values are not validated locally and overlaps are not server-checked. To re-tidy an existing dashboard use +dashboard-arrange instead.",
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
		if runtime.Bool("no-validate") {
			return nil
		}
		norm := normalizeDataConfig(cfg)
		if rawNumberFormat, hasNumberFormat := norm["number_format"]; hasNumberFormat {
			if problems := validateNumberFormat(rawNumberFormat); len(problems) > 0 {
				return formatDataConfigErrors(problems)
			}
		}
		b, _ := json.Marshal(norm)
		_ = runtime.Cmd.Flags().Set("data-config", string(b))
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		pc := newParseCtx(runtime)
		body, _ := buildDashboardBlockBody(pc, runtime, false)
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
