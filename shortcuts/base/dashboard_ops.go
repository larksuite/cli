// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/dashboardlayout"
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

// fineGridKey is the backward-compatibility switch the dashboard write API added
// for the grid change: omitting it means the legacy 12-col grid, i.e.
// byte-for-byte the old behavior every already-released CLI depends on. Only an
// explicit JSON boolean true selects the fine-grained 48-col grid. The switch
// used to be a numeric grid_cols=48; the server now rejects a body that still
// carries grid_cols with InvalidParam, so that key must never be sent again.
//
// This repository is 48-col end to end — internal/dashboardlayout's GridCols and
// the size tables in the skill docs are all calibrated for 48 —
// so every request that writes coordinates declares the fine grid explicitly and
// never relies on the server default. That includes creates WITHOUT --position:
// the server auto-packs those, and auto-packing on a 12-col grid lands pixels
// that do not line up with anything else this CLI computes.
//
// Reads carry no grid parameter at all: the read endpoints accept none, and the
// 48-col coordinates are converted from standard_position on the CLI side (see
// dashboard_block_position.go). TestDashboardFineGridContract pins the wire shape.
const fineGridKey = "fine_grid"

// withFineGrid stamps the fine-grid declaration onto a write request's body.
// Call it at every coordinate-carrying call site rather than burying it in one
// transport helper: DryRun and Execute build their maps separately, and a preview
// that omits a parameter the real request sends is worse than no preview.
func withFineGrid(fields map[string]interface{}) map[string]interface{} {
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields[fineGridKey] = true
	return fields
}

// includeBlockType / omitBlockType name buildDashboardBlockBody's bool at the
// call sites: create sends the block type, update must not (the API rejects a
// type change).
const (
	includeBlockType = true
	omitBlockType    = false
)

// validateDashboardBlockWriteConfig rejects unsupported top-level write fields.
// This applies to both commands because update has no block type. Callers keep
// it inside semantic validation so --no-validate retains its passthrough contract.
func validateDashboardBlockWriteConfig(cfg map[string]interface{}) error {
	var problems []string
	for _, key := range []string{"timebar", "card"} {
		if _, exists := cfg[key]; exists {
			problems = append(problems, "data_config."+key+" 不支持创建或更新")
		}
	}
	return formatDataConfigErrors(problems)
}

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
	// create 一律声明 fine_grid：不带 position 时服务端会自动装箱，装箱也按这个栅格算。
	// update 只在带 position 时声明：不传坐标的 update 不触发装箱，栅格声明无意义，
	// 不带上可保持纯配置更新（name / data_config / linkage）的请求体与主干一致。
	body := map[string]interface{}{}
	if includeType || strings.TrimSpace(runtime.Str("position")) != "" {
		body = withFineGrid(body)
	}
	if name := strings.TrimSpace(runtime.Str("name")); name != "" {
		body["name"] = name
	}
	if includeType {
		if blockType := normalizeDashboardBlockType(runtime.Str("type")); blockType != "" {
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
	// Linkage is an update-only top-level sibling, never part of data_config.
	if !includeType && runtime.Cmd.Flags().Changed("linkage") {
		linkage, err := parseDashboardChartLinkage(pc, runtime.Str("linkage"), runtime.Str("block-id"))
		if err != nil {
			return nil, err
		}
		body["linkage"] = linkage
	}
	return body, nil
}

// 仪表盘布局只有一条流程：读现有组件 → 规划全部组件坐标 → 新建时带 --position →
// 最后用 +dashboard-layout 提交盘上全部组件的坐标（它只写变化的组件，并总是调服务端
// 兜底整理空洞、重叠与越界）。help 里的 Tips 是静态的，调用方不一定每次读，所以每次
// 成功的 create/update 之后都走 stderr 把「最后一定要调 +dashboard-layout」再说一遍。
//
// 按"调用方当前在走哪条路"分两条：
//   - create 没传 --position：组件由服务端自动放置，提示下一个组件起带上规划好的坐标；
//   - create/update 传了 --position：这是推荐做法，只提示收尾那一步。
//
// 两条互斥，每次调用最多一条，不会把 stderr 刷成两行。
const (
	dashboardBlockNoPositionHint = "Tip: the block was created without coordinates, so the server placed it. Plan the coordinates of every block first and pass them to +dashboard-block-create with --position. With or without --position, finish by calling base +dashboard-layout with the coordinates of every block on the dashboard, existing blocks included; it writes only the blocks that changed and then runs the server's tidy pass for holes, overlaps and out-of-bounds blocks."
	dashboardWithPositionHint    = "Tip: --position placed this block exactly as given. After the last block is created or moved, finish by calling base +dashboard-layout with the coordinates of every block on the dashboard, existing blocks included; it writes only the blocks that changed and then runs the server's tidy pass for holes, overlaps and out-of-bounds blocks."
)

// emitDashboardPositionHint 在一次成功的 create/update 之后把对应提示写到 stderr。
// 必须在 runtime.Out 之前调用，且只走 stderr——stdout 的 JSON 是契约，不能掺提示文本。
func emitDashboardPositionHint(runtime *common.RuntimeContext, createdWithoutPosition bool) {
	hint := dashboardWithPositionHint
	if createdWithoutPosition {
		hint = dashboardBlockNoPositionHint
	}
	fmt.Fprintln(runtime.IO().ErrOut, hint)
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
	// 读接口不带任何栅格参数：网关只透传分页参数，48 列坐标由 CLI 从 standard_position 换算。
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

// dashboardBlockListPageSize is the page size +dashboard-layout uses to walk the
// block list. It matches the documented maximum so a whole dashboard normally
// arrives in one page.
const dashboardBlockListPageSize = 100

// fetchDashboardBlockPage retrieves one page of a dashboard's blocks. It is the
// single fetch path behind both +dashboard-block-list and the whole-dashboard
// walk in +dashboard-layout, so the two cannot drift apart.
//
// Every item's coordinates come back already rewritten to the 48-col grid (see
// dashboard_block_position.go); lossy lists the blocks that had no standard_position
// and fell back to the 12-col rounded value, for the caller to report.
func fetchDashboardBlockPage(runtime *common.RuntimeContext, params map[string]interface{}) (data map[string]interface{}, lossy []string, err error) {
	data, err = baseV3Call(runtime, "GET", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "blocks"), params, nil)
	if err != nil {
		return nil, nil, err
	}
	return data, normalizeDashboardBlockItems(data), nil
}

// executeDashboardBlockList lists all blocks in a dashboard.
func executeDashboardBlockList(runtime *common.RuntimeContext) error {
	params := map[string]interface{}{}
	params["page_size"] = runtime.Int("page-size")
	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}
	data, lossy, err := fetchDashboardBlockPage(runtime, params)
	if err != nil {
		return err
	}
	emitLegacyPositionNotice(runtime, lossy)
	runtime.Out(data, nil)
	return nil
}

// dashboardBlockSnapshot 是从组件列表投影出来的类型化组件快照，
// 布局计算只消费这几项，不把松散 map 带到下游。
type dashboardBlockSnapshot struct {
	ID          string
	ChartType   string
	HasPosition bool
	Position    dashboardlayout.Component
}

// listAllDashboardBlocks 分页读完一个仪表盘的全部组件。
// +dashboard-layout 要校验 --layout 覆盖了盘上全部组件，所以必须收齐，不能只看第一页。
func listAllDashboardBlocks(runtime *common.RuntimeContext) ([]dashboardBlockSnapshot, error) {
	var blocks []dashboardBlockSnapshot
	var lossy []string
	pageToken := ""
	for {
		params := map[string]interface{}{"page_size": dashboardBlockListPageSize}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, pageLossy, err := fetchDashboardBlockPage(runtime, params)
		if err != nil {
			return nil, err
		}
		lossy = append(lossy, pageLossy...)
		items, _ := data["items"].([]interface{})
		for _, item := range items {
			obj, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if snapshot, ok := projectDashboardBlockSnapshot(obj); ok {
				blocks = append(blocks, snapshot)
			}
		}
		hasMore, _ := data["has_more"].(bool)
		nextToken, _ := data["page_token"].(string)
		if !hasMore || strings.TrimSpace(nextToken) == "" {
			// 收齐后只陈述一次，不按页各报一行。
			emitLegacyPositionNotice(runtime, lossy)
			return blocks, nil
		}
		pageToken = nextToken
	}
}

// projectDashboardBlockSnapshot 把一条组件记录投影成 dashboardBlockSnapshot。
// 写入用的字段名是 position，而读回侧历史上也出现过 layout，两个名字都认，
// 谁都读不到就当作"没有可读坐标"，由调用方决定怎么处理。
// 入参必须已经过 normalizeDashboardBlockPosition：这里读到的坐标按 48 列使用。
func projectDashboardBlockSnapshot(obj map[string]interface{}) (dashboardBlockSnapshot, bool) {
	id, _ := obj["block_id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return dashboardBlockSnapshot{}, false
	}
	chartType, _ := obj["type"].(string)
	if strings.TrimSpace(chartType) == "" {
		chartType, _ = obj["block_type"].(string)
	}
	snapshot := dashboardBlockSnapshot{ID: id, ChartType: normalizeDashboardBlockType(chartType)}
	for _, key := range []string{"position", "layout"} {
		pos, ok := obj[key].(map[string]interface{})
		if !ok {
			continue
		}
		x, okX := toIntStrict(pos["x"])
		y, okY := toIntStrict(pos["y"])
		w, okW := toIntStrict(pos["w"])
		h, okH := toIntStrict(pos["h"])
		if okX && okY && okW && okH {
			snapshot.HasPosition = true
			snapshot.Position = dashboardlayout.Component{ID: id, ChartType: snapshot.ChartType, X: x, Y: y, W: w, H: h}
			break
		}
	}
	return snapshot, true
}

// submitDashboardPositions 把整批坐标一次提交到 arrange 端点。
//
// 相对逐个 PATCH 的关键差别不是快，是**原子**：服务端把整批变更合成一次 OT 提交，
// 要么全部生效要么全部不生效。逐个 PATCH 时中途失败会把盘面留在"前几个已经挪了、
// 后几个还在原位"的状态，且没有可回滚的位置。
//
// comps 为空时不发请求——空 positions 会被服务端按参数非法拒绝。
func submitDashboardPositions(runtime *common.RuntimeContext, comps []dashboardlayout.Component) error {
	if len(comps) == 0 {
		return nil
	}
	_, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "arrange"), nil, dashboardPositionsBody(comps))
	return err
}

// dashboardPositionsBody 是批量写坐标的请求体，预览与真实请求共用。
// fine_grid 必须显式声明：服务端默认 12 列，漏掉会把这批 48 列坐标按 12 列换算成像素。
func dashboardPositionsBody(comps []dashboardlayout.Component) map[string]interface{} {
	positions := make([]map[string]interface{}, 0, len(comps))
	for _, comp := range comps {
		positions = append(positions, map[string]interface{}{
			"block_id": comp.ID,
			"x":        comp.X,
			"y":        comp.Y,
			"w":        comp.W,
			"h":        comp.H,
		})
	}
	return withFineGrid(map[string]interface{}{"positions": positions})
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
	normalizeSingleBlockOutput(runtime, data, runtime.Str("block-id"))
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
	emitDashboardPositionHint(runtime, strings.TrimSpace(runtime.Str("position")) == "")
	normalizeSingleBlockOutput(runtime, data, "")
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
	// 不带 --position 的 update 只改 name/data_config，没碰布局，不提示。
	if strings.TrimSpace(runtime.Str("position")) != "" {
		emitDashboardPositionHint(runtime, false)
	}
	normalizeSingleBlockOutput(runtime, data, runtime.Str("block-id"))
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
