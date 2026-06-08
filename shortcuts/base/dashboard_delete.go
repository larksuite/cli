// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardDelete = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-delete",
	Description: "Delete a dashboard",
	Risk:        "high-risk-write",
	Scopes:      []string{"base:dashboard:delete"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
	},
	Tips: []string{
		"lark-cli base +dashboard-delete --base-token <base_token> --dashboard-id <dashboard_id> --yes",
		"Deleting a dashboard also deletes its blocks and cannot be recovered.",
		baseHighRiskYesTip,
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id").
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardDelete(runtime)
	},
}
