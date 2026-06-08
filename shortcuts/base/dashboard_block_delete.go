// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockDelete = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-delete",
	Description: "Delete a dashboard block",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:dashboard:delete"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		blockIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +dashboard-block-delete --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> --yes",
		baseHighRiskYesTip,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id")).
			Set("block_id", runtime.Str("block-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockDelete(runtime)
	},
}
