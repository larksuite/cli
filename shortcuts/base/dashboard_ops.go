// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// dashboardIDFlag returns a Flag for dashboard ID.
func dashboardIDFlag(required bool) common.Flag {
	return common.Flag{Name: "dashboard-id", Desc: "dashboard ID", Required: required}
}

// blockIDFlag returns a Flag for dashboard block ID.
func blockIDFlag(required bool) common.Flag {
	return common.Flag{Name: "block-id", Desc: "dashboard block ID", Required: required}
}

// dryRunDashboardBase returns a base DryRunAPI with common dashboard parameters set.
func dryRunDashboardBase(runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		Set("base_token", runtime.Str("base-token")).
		Set("dashboard_id", runtime.Str("dashboard-id")).
		Set("block_id", runtime.Str("block-id"))
}

// dryRunDashboardList returns a DryRunAPI for listing dashboards.
func dryRunDashboardList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	if pageSize := strings.TrimSpace(runtime.Str("page-size")); pageSize != "" {
		params["page_size"] = pageSize
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards").
		Params(params)
}

// dryRunDashboardGet returns a DryRunAPI for getting a dashboard.
func dryRunDashboardGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id")
}

// dryRunDashboardCreate returns a DryRunAPI for creating a dashboard.
func dryRunDashboardCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	return dryRunDashboardBase(runtime).
		POST("/open-apis/base/v3/bases/:base_token/dashboards").
		Body(body)
}

// dryRunDashboardUpdate returns a DryRunAPI for updating a dashboard.
func dryRunDashboardUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	return dryRunDashboardBase(runtime).
		PATCH("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id").
		Body(body)
}

// dryRunDashboardDelete returns a DryRunAPI for deleting a dashboard.
func dryRunDashboardDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id")
}

// dryRunDashboardBlockList returns a DryRunAPI for listing dashboard blocks.
func dryRunDashboardBlockList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	if pageSize := strings.TrimSpace(runtime.Str("page-size")); pageSize != "" {
		params["page_size"] = pageSize
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
		Params(params)
}

// dryRunDashboardBlockGet returns a DryRunAPI for getting a dashboard block.
func dryRunDashboardBlockGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
		Params(params)
}

// dryRunDashboardBlockCreate returns a DryRunAPI for creating a dashboard block.
func dryRunDashboardBlockCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if blockType := strings.TrimSpace(runtime.Str("type")); blockType != "" {
		body["type"] = blockType
	}
	if raw := runtime.Str("data-config"); raw != "" {
		if parsed, err := parseJSONObject(pc, raw, "data-config"); err == nil {
			body["data_config"] = parsed
		}
	}

	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return dryRunDashboardBase(runtime).
		POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
		Params(params).
		Body(body)
}

// dryRunDashboardBlockUpdate returns a DryRunAPI for updating a dashboard block.
func dryRunDashboardBlockUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if raw := runtime.Str("data-config"); raw != "" {
		if parsed, err := parseJSONObject(pc, raw, "data-config"); err == nil {
			body["data_config"] = parsed
		}
	}
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return dryRunDashboardBase(runtime).
		PATCH("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
		Params(params).
		Body(body)
}

// dryRunDashboardBlockDelete returns a DryRunAPI for deleting a dashboard block.
func dryRunDashboardBlockDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id")
}

// ── Dashboard CRUD ──────────────────────────────────────────────────

// executeDashboardList lists all dashboards in a base.
func executeDashboardList(runtime *common.RuntimeContext) error {
	params := map[string]interface{}{}
	if pageSize := strings.TrimSpace(runtime.Str("page-size")); pageSize != "" {
		params["page_size"] = pageSize
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards"), params, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

// executeDashboardGet retrieves a dashboard by ID.
func executeDashboardGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"dashboard": data}, nil)
	return nil
}

// executeDashboardCreate creates a new dashboard.
func executeDashboardCreate(runtime *common.RuntimeContext) error {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards"), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"dashboard": data, "created": true}, nil)
	return nil
}

// executeDashboardUpdate updates an existing dashboard.
func executeDashboardUpdate(runtime *common.RuntimeContext) error {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id")), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"dashboard": data, "updated": true}, nil)
	return nil
}

// executeDashboardDelete deletes a dashboard by ID.
func executeDashboardDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "dashboard_id": runtime.Str("dashboard-id")}, nil)
	return nil
}

// ── Dashboard Block CRUD ────────────────────────────────────────────

// executeDashboardBlockList lists all blocks in a dashboard.
func executeDashboardBlockList(runtime *common.RuntimeContext) error {
	params := map[string]interface{}{}
	if pageSize := strings.TrimSpace(runtime.Str("page-size")); pageSize != "" {
		params["page_size"] = pageSize
	}
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks"), params, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

// executeDashboardBlockGet retrieves a dashboard block by ID.
func executeDashboardBlockGet(runtime *common.RuntimeContext) error {
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks", runtime.Str("block-id")), params, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data}, nil)
	return nil
}

// executeDashboardBlockCreate creates a new dashboard block.
func executeDashboardBlockCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if blockType := strings.TrimSpace(runtime.Str("type")); blockType != "" {
		body["type"] = blockType
	}
	if raw := runtime.Str("data-config"); raw != "" {
		parsed, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		body["data_config"] = parsed
	}

	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}

	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks"), params, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "created": true}, nil)
	return nil
}

// executeDashboardBlockUpdate updates an existing dashboard block.
func executeDashboardBlockUpdate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if raw := runtime.Str("data-config"); raw != "" {
		parsed, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return err
		}
		body["data_config"] = parsed
	}
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}

	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks", runtime.Str("block-id")), params, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"block": data, "updated": true}, nil)
	return nil
}

// executeDashboardBlockDelete deletes a dashboard block by ID.
func executeDashboardBlockDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks", runtime.Str("block-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "block_id": runtime.Str("block-id")}, nil)
	return nil
}

// ── Dashboard Arrange ────────────────────────────────────────────────

// dryRunDashboardArrange returns a DryRunAPI for the dashboard arrange endpoint.
func dryRunDashboardArrange(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return dryRunDashboardBase(runtime).
		POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/arrange").
		Params(params).
		Body(map[string]interface{}{})
}

// executeDashboardArrange sends a POST request to auto-arrange dashboard blocks layout.
func executeDashboardArrange(runtime *common.RuntimeContext) error {
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	// 请求体为空对象，由服务端智能重排
	data, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "arrange"), params, map[string]interface{}{})
	if err != nil {
		return err
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	data["arranged"] = true
	runtime.Out(data, nil)
	return nil
}
