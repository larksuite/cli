// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardArrange = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-arrange",
	Description: "Auto-arrange dashboard blocks layout (server-side smart layout)",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "user-id-type", Desc: "user ID type: open_id / union_id / user_id"},
	},
	DryRun: dryRunDashboardArrange,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardArrange(runtime)
	},
}
