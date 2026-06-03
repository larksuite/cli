// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseDashboardCreate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-create",
	Description: "Create a dashboard in a base",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:create"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		{Name: "name", Desc: "dashboard name", Required: true},
		{Name: "theme-style", Desc: "theme style, defaults to platform default when omitted"},
	},
	Tips: []string{
		"Record the returned dashboard_id; dashboard block create/get/update/delete/arrange commands need it.",
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
			POST("/open-apis/base/v3/bases/:base_token/dashboards").
			Body(body).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardCreate(runtime)
	},
}
