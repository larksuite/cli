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
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size, range 1-100"},
		{Name: "page-token", Desc: "pagination token"},
	},
	Tips: []string{
		"Use returned dashboard_id values for +dashboard-get, +dashboard-block-list, and +dashboard-block-create.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{}
		params["page_size"] = runtime.Int("page-size")
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
