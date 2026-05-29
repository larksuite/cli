// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

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
		"Use +dashboard-block-get-data when you need the computed chart result instead of metadata.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{}
		if uid := strings.TrimSpace(runtime.Str("user-id-type")); uid != "" {
			params["user_id_type"] = uid
		}
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
			Params(params).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id")).
			Set("block_id", runtime.Str("block-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardBlockGet(runtime)
	},
}
