// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardBlockList = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-block-list",
	Description: "List blocks in a dashboard",
	Risk:        "read",
	Scopes:      []string{"base:dashboard:read"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size, range 1-100"},
		{Name: "page-token", Desc: "pagination token"},
	},
	Tips: []string{
		"lark-cli base +dashboard-block-list --base-token <base_token> --dashboard-id <dashboard_id>",
		"Use returned block_id and type values for +dashboard-block-get/update/delete/get-data.",
		"Each block's position is on the 48-col grid (converted by the CLI from the server's pixel coordinates), the same grid +dashboard-layout takes; plan new coordinates against these numbers directly.",
		"For a complete dashboard, use --page-size 100; while has_more=true, pass the returned page_token to --page-token and continue until has_more=false.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 20, 1, 100)
		return err
	},
	// 预览走 dashboard_ops.go 的共享构造，理由同 +dashboard-block-get。
	DryRun: dryRunDashboardBlockList,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockList(runtime)
	},
}
