// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
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

// includeBlockType / omitBlockType name buildDashboardBlockBody's bool at the
// call sites: create sends the block type, update must not (the API rejects a
// type change).
const (
	includeBlockType = true
	omitBlockType    = false
)

// buildDashboardBlockBody assembles the request body shared by the dashboard
// block create and update commands. It is the single source of truth for body
// shape so DryRun and Execute stay isomorphic across all call sites.
//
//   - includeType controls whether the block type is emitted (create only).
//   - The optional top-level position object is parsed as JSON and passed
//     through verbatim as a sibling of name/type/data_config; coordinate values
//     are NOT validated (aligns with dws grid semantics).
//   - data-config and position are parsed as JSON objects. Validate performs
//     this same parse before either DryRun or Execute, so malformed input cannot
//     disappear from a preview while failing only on the live request.
func buildDashboardBlockBody(pc *parseCtx, runtime *common.RuntimeContext, includeType bool) (map[string]interface{}, error) {
	body := map[string]interface{}{}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if includeType {
		if blockType := strings.TrimSpace(runtime.Str("type")); blockType != "" {
			body["type"] = blockType
		}
	}
	if raw := strings.TrimSpace(runtime.Str("data-config")); raw != "" {
		parsed, err := parseJSONObject(pc, raw, "data-config")
		if err != nil {
			return nil, err
		}
		body["data_config"] = parsed
	}
	if raw := strings.TrimSpace(runtime.Str("position")); raw != "" {
		parsed, err := parseJSONObject(pc, raw, "position")
		if err != nil {
			return nil, err
		}
		body["position"] = parsed
	}
	return body, nil
}

// positionKeys are the four grid coordinates a position object must carry.
// A partial object is rejected because a missing coordinate is not "leave this
// one alone" — an update meant to move a block sideways would arrive without a
// size. An explicit null is rejected for the same reason a missing key is:
// `{"x":6,"y":null}` carries exactly as little information as `{"x":6}`, so
// presence alone is not enough, the value has to actually be a number.
var positionKeys = []string{"x", "y", "w", "h"}

// isJSONNumber reports whether v came out of the JSON decoder as a number.
// float64 is what the production path yields (parseJSONObject uses a plain
// json.Unmarshal); json.Number is accepted so the check keeps working if that
// decoder ever switches to UseNumber.
func isJSONNumber(v interface{}) bool {
	switch v.(type) {
	case float64, json.Number:
		return true
	default:
		return false
	}
}

// validateDashboardBlockPosition parses the optional --position flag as a JSON
// object to fail fast on malformed input. Coordinate *values* (x/y/w/h) are NOT
// validated — out-of-range, negative and overlapping coordinates pass through
// verbatim, aligning with dws grid semantics and leaving overlap handling to
// the server. Only the object's shape is enforced.
//
// The JSON parse always runs, including with --no-validate, because it is
// required for DryRun/Execute request-shape parity. The key-completeness check
// is semantic, so --no-validate skips it like the rest of the semantic layer.
func validateDashboardBlockPosition(pc *parseCtx, runtime *common.RuntimeContext) error {
	raw := strings.TrimSpace(runtime.Str("position"))
	if raw == "" {
		return nil
	}
	pos, err := parseJSONObject(pc, raw, "position")
	if err != nil {
		return err
	}
	if !runtime.Bool("no-validate") {
		var missing []string
		for _, key := range positionKeys {
			if v, ok := pos[key]; !ok || !isJSONNumber(v) {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--position 的 %s 缺失或不是数字；x/y/w/h 必须同时提供且为数值"+
					"（position 按整体提交，不做逐字段合并，残缺对象无法表达一个完整位置）",
				strings.Join(missing, "/")).WithParam("--position")
		}
	}
	// Fold an @file input into inline JSON so DryRun and Execute do not each
	// re-open the file — the preview and the request are then guaranteed to
	// describe the same bytes even if the file changes underneath us.
	b, _ := json.Marshal(pos)
	_ = runtime.Cmd.Flags().Set("position", string(b))
	return nil
}

// dryRunDashboardBase returns a base DryRunAPI carrying the dashboard
// identifiers this command actually takes. The test is whether the command
// declares the flag, not whether the value is non-empty: Set doubles as the
// substitution source for :param placeholders in the URL, so skipping a
// declared-but-empty identifier would leave the raw template in the preview and
// hide the fact that the argument was empty. A create command simply has no
// block-id flag, and that is the case worth omitting.
func dryRunDashboardBase(runtime *common.RuntimeContext) *common.DryRunAPI {
	api := common.NewDryRunAPI()
	for key, flag := range map[string]string{
		"base_token":   "base-token",
		"dashboard_id": "dashboard-id",
		"block_id":     "block-id",
	} {
		if runtime.Cmd.Flags().Lookup(flag) != nil {
			api.Set(key, strings.TrimSpace(runtime.Str(flag)))
		}
	}
	return api
}

// dryRunDashboardList returns a DryRunAPI for listing dashboards.
func dryRunDashboardList(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	params := map[string]interface{}{}
	params["page_size"] = runtime.Int("page-size")
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
	params["page_size"] = runtime.Int("page-size")
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

// dryRunDashboardBlockGetData returns a DryRunAPI for getting computed data for a dashboard block.
func dryRunDashboardBlockGetData(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return common.NewDryRunAPI().
		GET("/open-apis/base/v3/bases/:base_token/dashboards/blocks/:block_id/data").
		Set("base_token", runtime.Str("base-token")).
		Set("block_id", runtime.Str("block-id"))
}

// dryRunDashboardBlockCreate returns a DryRunAPI for creating a dashboard block.
func dryRunDashboardBlockCreate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	pc := newParseCtx(runtime)
	body, err := buildDashboardBlockBody(pc, runtime, includeBlockType)
	if err != nil {
		// Unreachable while Validate parses the same flags first. Returning nil
		// makes the runner fail loudly instead of previewing a body that is
		// silently missing a field the real request would have carried.
		return nil
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
	body, err := buildDashboardBlockBody(pc, runtime, omitBlockType)
	if err != nil {
		// See dryRunDashboardBlockCreate: fail loudly rather than preview a
		// body that diverges from what Execute would send.
		return nil
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
	params["page_size"] = runtime.Int("page-size")
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
	params["page_size"] = runtime.Int("page-size")
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

// executeDashboardBlockGetData retrieves computed data for a dashboard chart block.
func executeDashboardBlockGetData(runtime *common.RuntimeContext) error {
	data, err := baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", "blocks", runtime.Str("block-id"), "data"), nil, nil)
	if err != nil {
		return err
	}
	runtime.Out(data, nil)
	return nil
}

// executeDashboardBlockCreate creates a new dashboard block.
func executeDashboardBlockCreate(runtime *common.RuntimeContext) error {
	pc := newParseCtx(runtime)
	body, err := buildDashboardBlockBody(pc, runtime, includeBlockType)
	if err != nil {
		return err
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
	body, err := buildDashboardBlockBody(pc, runtime, omitBlockType)
	if err != nil {
		return err
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
