// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

func dashboardIDFlag(required bool) common.Flag {
	return common.Flag{Name: "dashboard-id", Desc: "dashboard ID", Required: required}
}

func blockIDFlag(required bool) common.Flag {
	return common.Flag{Name: "block-id", Desc: "dashboard block ID", Required: required}
}

func dryRunDashboardBase(runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		Set("base_token", runtime.Str("base-token")).
		Set("dashboard_id", runtime.Str("dashboard-id")).
		Set("block_id", runtime.Str("block-id"))
}

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

func dryRunDashboardGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id")
}

func dryRunDashboardCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := map[string]interface{}{"name": runtime.Str("name")}
	if themeStyle := strings.TrimSpace(runtime.Str("theme-style")); themeStyle != "" {
		body["theme"] = map[string]interface{}{"theme_style": themeStyle}
	}
	return dryRunDashboardBase(runtime).
		POST("/open-apis/base/v3/bases/:base_token/dashboards").
		Body(body)
}

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

func dryRunDashboardDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id")
}

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

func dryRunDashboardBlockGet(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	if userIDType := strings.TrimSpace(runtime.Str("user-id-type")); userIDType != "" {
		params["user_id_type"] = userIDType
	}
	return dryRunDashboardBase(runtime).
		GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id").
		Params(params)
}

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

func dryRunDashboardBlockDelete(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return dryRunDashboardBase(runtime).
		DELETE("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks/:block_id")
}

// ── Dashboard CRUD ──────────────────────────────────────────────────

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

func executeDashboardGet(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"dashboard": data}, nil)
	return nil
}

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

func executeDashboardDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "dashboard_id": runtime.Str("dashboard-id")}, nil)
	return nil
}

// ── Dashboard Block CRUD ────────────────────────────────────────────

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

func executeDashboardBlockDelete(runtime *common.RuntimeContext) error {
	_, err := baseV3Call(runtime, "DELETE", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks", runtime.Str("block-id")), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{"deleted": true, "block_id": runtime.Str("block-id")}, nil)
	return nil
}
