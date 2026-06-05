// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-update",
	Description: "Update a dashboard",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "name", Desc: "new dashboard name"},
		{Name: "theme-style", Desc: "theme style, leave empty to keep current theme"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{}
		if name := runtime.Str("name"); name != "" {
			body["name"] = name
		}
		if themeStyle := runtime.Str("theme-style"); themeStyle != "" {
			body["theme"] = map[string]interface{}{"theme_style": themeStyle}
		}
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id").
			Body(body).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardUpdate(runtime)
	},
}
