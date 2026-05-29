// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardList = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-list",
	Description: "List dashboards in a base",
	Risk:        "read",
	Scopes:      []string{"base:dashboard:read"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "page-size", Desc: "page size, max 100"},
		{Name: "page-token", Desc: "pagination token"},
	},
	Tips: []string{
		"Use returned dashboard_id values for +dashboard-get, +dashboard-block-list, and +dashboard-block-create.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{}
		if ps := strings.TrimSpace(runtime.Str("page-size")); ps != "" {
			params["page_size"] = ps
		}
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/dashboards").
			Params(params).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardList(runtime)
	},
}
