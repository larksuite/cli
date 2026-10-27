// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockGet = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-get",
	Description: "Get a dashboard block by ID",
	Risk:        "read",
	Scopes:      []string{"base:dashboard:read"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		blockIDFlag(true),
		{Name: "user-id-type", Desc: "user ID type: open_id / union_id / user_id"},
	},
	Tips: []string{
		"lark-cli base +dashboard-block-get --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id>",
		"Use this command for block metadata such as name, type, layout, and data_config.",
		"Single-source and multi-source blocks can return series_descriptors with stable keys and storage_paths. Use the returned descriptors for targeted styling; do not guess keys or convert sources when a target is missing.",
		"The returned position is on the 48-col grid (converted by the CLI from the server's pixel coordinates), the same grid +dashboard-layout takes.",
		"linkage contains this block's outgoing chart linkage when configured; it does not list incoming sources. Read lark-base-dashboard-linkage.md before replacing targets.",
		"Countdown data_config returns the complete active fixed or field configuration with camelCase keys; create and update return the same full block shape.",
		"Progress blocks return the complete active data_config.progress. Relative dates remain relative; cleared filters and hidden sources are omitted. This endpoint does not compute the progress percentage.",
		"View blocks return tableId, viewId, and viewType references in data_config; viewType is a string such as grid, gantt, or gallery. Do not assume GET echoes the create/update fields view_type, filter, group_by, sort_by, or visible_fields; their omission does not indicate a failed save or justify resending the configuration.",
		"To read persisted view configuration, pass the returned tableId/viewId as --table-id/--view-id to the +view-get-* commands: +view-get-filter, +view-get-sort, +view-get-group, or +view-get-visible-fields. Use +record-list to read records.",
		"Text block content is stored in data_config.text; include it when the user asks for all dashboard content.",
		"Use +dashboard-block-get-data when you need the computed chart result instead of metadata.",
	},
	// 预览走 dashboard_ops.go 的共享构造，与 Execute 同源，预览不会漏掉真实请求带的参数。
	// 读请求不带栅格参数，返回的 position 由 CLI 从 standard_position 换算成 48 列。
	DryRun: dryRunDashboardBlockGet,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockGet(runtime)
	},
}
