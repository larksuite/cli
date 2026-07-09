// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

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
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 20, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{}
		params["page_size"] = runtime.Int("page-size")
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
			Params(params).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockList(runtime)
	},
}
