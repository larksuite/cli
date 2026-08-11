// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var dashboardShareUpdateFlagNames = []string{
	"enabled",
	"access-scope",
	"show-source",
	"enable-auto-analysis",
}

var BaseDashboardShareGet = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-share-get",
	Description: "Get dashboard share status and settings",
	Risk:        "read",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/share").
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "GET", baseV3Path(
			"bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "share",
		), nil, nil)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseDashboardShareUpdate = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-share-update",
	Description: "Update dashboard share status and settings",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:update"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{Name: "enabled", Type: "bool", Desc: "enable or disable dashboard sharing"},
		{Name: "access-scope", Desc: "share access scope", Enum: shareAccessScopeEnums},
		{Name: "show-source", Type: "bool", Desc: "show the entry back to the source Base"},
		{Name: "enable-auto-analysis", Type: "bool", Desc: "enable intelligent analysis on the shared dashboard"},
	},
	Tips: []string{
		"Boolean settings use PATCH semantics: pass --show-source=false or --enable-auto-analysis=false to explicitly turn them off.",
		"Update exactly one field per invocation; run separate commands to change multiple share fields.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateSingleShareUpdate(runtime, dashboardShareUpdateFlagNames...)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/share").
			Body(buildDashboardShareUpdateBody(runtime)).
			Set("base_token", runtime.Str("base-token")).
			Set("dashboard_id", runtime.Str("dashboard-id"))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := baseV3Call(runtime, "PATCH", baseV3Path(
			"bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "share",
		), nil, buildDashboardShareUpdateBody(runtime))
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func buildDashboardShareUpdateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	addCommonShareUpdateFields(runtime, body)

	settings := map[string]interface{}{}
	if runtime.Changed("show-source") {
		settings["show_source"] = runtime.Bool("show-source")
	}
	if runtime.Changed("enable-auto-analysis") {
		settings["enable_auto_analysis"] = runtime.Bool("enable-auto-analysis")
	}
	if len(settings) > 0 {
		body["settings"] = settings
	}
	return body
}
