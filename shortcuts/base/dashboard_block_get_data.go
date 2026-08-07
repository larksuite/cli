// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockGetData = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-get-data",
	Description: "Get computed data for a dashboard chart block",
	Risk:        common.RiskRead,
	Scopes:      []string{"base:dashboard:read"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		blockIDFlag(true),
		{Name: "dashboard-id", Desc: "hidden compatibility flag accepted by dashboard block commands; ignored by get-data", Hidden: true},
	},
	Tips: []string{
		"lark-cli base +dashboard-block-get-data --base-token <base_token> --block-id <block_id>",
		"This command does not need --dashboard-id.",
		"Use +dashboard-block-get first when you need block metadata like name, type, or data_config.",
		"This command returns computed chart protocol JSON directly, not wrapped block metadata.",
		"For a complete dashboard export, read text blocks with +dashboard-block-get; their content is in data_config.text.",
		"If a chart type does not support computed data, inspect its data_config with +dashboard-block-get, then use +data-query with the same real table, dimensions, measures, and filters; do not omit the block or guess values.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunDashboardBlockGetData(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockGetData(runtime)
	},
}
