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
	Risk:        "read",
	Scopes:      []string{"base:dashboard:read"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		blockIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +dashboard-block-get-data --base-token <base_token> --block-id <block_id>",
		"Use +dashboard-block-get first when you need block metadata like name, type, or data_config.",
		"This command returns computed chart protocol JSON directly, not wrapped block metadata.",
		"Text blocks do not have computed chart data; this shortcut is for chart/statistics blocks.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunDashboardBlockGetData(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockGetData(runtime)
	},
}
