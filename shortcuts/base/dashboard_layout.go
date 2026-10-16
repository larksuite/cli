// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/dashboardlayout"
	"github.com/larksuite/cli/shortcuts/common"
)

// BaseDashboardLayout 是仪表盘布局的收尾命令：调用方把盘上**全部**组件的 48 列坐标
// 一次交上来，命令只做四件事——
//
//  1. 读现状（组件列表，48 列坐标由 standard_position 换算）；
//  2. 校验覆盖：盘上每个组件都必须出现在 --layout 里，不能多、不能重复；
//  3. 只把坐标与现状不同的组件合成一次批量写入，全都没变就不发写请求；
//  4. **每次都**调一次服务端兜底整理（arrange + tidy_only），由它按前端布局库处理
//     空洞、重叠与越界，并返回被它挪动的组件。
//
// 命令本身不评判布局好坏，只在本地拒绝几何上不成立的坐标（负数、w/h<=0、x+w 超出 48 列）；
// 合法坐标之间的空洞与重叠交给第 4 步兜底。
var BaseDashboardLayout = common.Shortcut{
	Service:     "base",
	Command:     "+dashboard-layout",
	Description: "Commit every block's 48-col coordinates in one call, then let the server tidy holes and overlaps",
	Risk:        "write",
	Scopes:      []string{"base:dashboard:read", "base:dashboard:update"},
	AuthTypes:   authTypes(),
	HasFormat:   true,
	Flags: []common.Flag{
		baseTokenFlag(true),
		dashboardIDFlag(true),
		{
			Name:     "layout",
			Required: true,
			Input:    []string{common.File, common.Stdin},
			Desc: `coordinates of EVERY block on the dashboard (@file or - for stdin): a JSON array ` +
				`[{"block_id","x","y","w","h"}] on the 48-col grid, all five keys required, x/y/w/h integers. ` +
				`It must cover the whole board — existing blocks you did not move included, at the coordinates ` +
				`+dashboard-block-list reports — and must not name a block that is not on the dashboard or repeat one; ` +
				`any of these fails before anything is written and the error lists the block_ids. Coordinates must ` +
				`satisfy x>=0, y>=0, w>=1, h>=1 and x+w<=48, otherwise the command fails before any request. Only blocks whose coordinates differ from the current ones are written, in one ` +
				`batch; afterwards the server always runs a tidy pass that fixes holes, overlaps and out-of-bounds ` +
				`blocks, and stdout reports the final coordinates. Objects such as {"rows":...} or {"columns":...} are rejected`,
		},
	},
	Tips: []string{
		`lark-cli base +dashboard-layout --base-token <base_token> --dashboard-id <dashboard_id> --layout '[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":8},{"block_id":"blk_b","x":24,"y":0,"w":24,"h":8},{"block_id":"blk_c","x":0,"y":8,"w":48,"h":16}]'`,
		"lark-cli base +dashboard-layout --base-token <base_token> --dashboard-id <dashboard_id> --layout @layout.json",
		"Workflow: read the existing blocks with +dashboard-block-list (a new dashboard has none) → plan the coordinates of ALL blocks for the request (on a new dashboard plan the whole board; when appending or editing an existing one, keep the blocks the user did not ask to move where they are) → create new blocks with +dashboard-block-create --position → finish by calling this command with every block's coordinates.",
		"Always finish a layout change (creating, moving or resizing blocks) with this command, even when every block was created with --position and nothing seems to need moving: it is the step that runs the server's tidy pass.",
		"Do not call this command after only deleting blocks: the tidy pass always fills holes, so it would move the remaining blocks. Leave the rest of the board where it is unless the user asked to rearrange it.",
		"The tidy pass may move or resize blocks you did not touch, including filling holes that were already on the board. Its result is final: read tidied_block_ids and the final layout, and do not resubmit just to undo it.",
		"--layout must list every block on the dashboard. Leaving one out, naming one that does not exist, or repeating one fails with the offending block_ids and writes nothing.",
		"The grid is 48 columns wide. Common widths: statistics 8-24, column/bar/line/area 12-36, pie/ring 12-24; heights 6-12 for statistics and 12-24 for charts. Read lark-base-dashboard-visual-standards.md before planning coordinates.",
		"stdout: written_block_ids are the blocks whose submitted coordinates were written, tidied_block_ids are the blocks the tidy pass moved afterwards (stderr says from where to where), and layout is the final 48-col layout of the whole board.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := parseDashboardLayoutItems(newParseCtx(runtime), runtime.Str("layout"))
		return err
	},
	DryRun: dryRunDashboardLayout,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeDashboardLayout(runtime)
	},
}

// dashboardLayoutItem 是 --layout 数组里的一项。
type dashboardLayoutItem struct {
	BlockID string
	X       int
	Y       int
	W       int
	H       int
}

// layoutItemCoordKeys 是每一项必须齐全且为整数的四个坐标键。
var layoutItemCoordKeys = []string{"x", "y", "w", "h"}

// tidyOnlyKey 是 arrange 端点的兜底整理开关：带上它时服务端不按 positions 重排，
// 只对库里现有布局跑前端布局库的空洞整理，并只写回有变化的组件。
const tidyOnlyKey = "tidy_only"

// tidiedBlockIDsKey 是兜底整理返回里「被挪动的组件」列表的字段名，已规整时为空数组。
const tidiedBlockIDsKey = "tidied_block_ids"

// tidyResponseBlocksKey 是兜底整理返回里整盘组件列表的字段名。最终坐标一律从每项的
// standard_position 换算（与读接口同一套换算），不用同项里的 layout：它按请求的栅格取整，
// 口径随请求变化，像素才是唯一不失真的来源。拿不到像素就再读一次组件列表。
const tidyResponseBlocksKey = "blocks"

// parseDashboardLayoutItems 把 --layout 投影成类型化的坐标列表。
// 校验形状（数组、每项是对象、block_id 非空且不重复、四个坐标齐全且为整数）和单项坐标
// 范围（见 checkLayoutItemBounds）；组件之间的空洞、重叠属于整盘关系，交给写入后的兜底整理。
func parseDashboardLayoutItems(pc *parseCtx, raw string) ([]dashboardLayoutItem, error) {
	resolved, err := loadJSONInput(pc, raw, "layout")
	if err != nil {
		return nil, err
	}
	resolved = strings.TrimSpace(resolved)
	if strings.HasPrefix(resolved, "{") {
		return nil, baseFlagErrorf(
			`--layout 只接受坐标数组 [{"block_id","x","y","w","h"}]，盘上每个组件一项；` +
				`不接受 {"rows":...}、{"columns":...} 等对象形态`)
	}
	arr, err := parseJSONArray(pc, resolved, "layout")
	if err != nil {
		return nil, err
	}
	if len(arr) == 0 {
		return nil, baseFlagErrorf("--layout 不能是空数组；需要给出盘上每个组件的 block_id 与 x/y/w/h")
	}
	items := make([]dashboardLayoutItem, 0, len(arr))
	seen := make(map[string]int, len(arr))
	for i, entry := range arr {
		obj, ok := entry.(map[string]interface{})
		if !ok {
			return nil, baseFlagErrorf("--layout 第 %d 项必须是 JSON 对象", i+1)
		}
		blockID, _ := obj["block_id"].(string)
		blockID = strings.TrimSpace(blockID)
		if blockID == "" {
			return nil, baseFlagErrorf("--layout 第 %d 项缺少 block_id；block_id 来自 +dashboard-block-list", i+1)
		}
		if prev, dup := seen[blockID]; dup {
			return nil, baseFlagErrorf("--layout 第 %d 项与第 %d 项的 block_id 都是 %s；同一个组件只能出现一次", i+1, prev+1, blockID)
		}
		seen[blockID] = i

		coords := make(map[string]int, len(layoutItemCoordKeys))
		var missing []string
		for _, key := range layoutItemCoordKeys {
			n, ok := toIntStrict(obj[key])
			if !ok {
				missing = append(missing, key)
				continue
			}
			coords[key] = n
		}
		if len(missing) > 0 {
			return nil, baseFlagErrorf(
				"--layout 第 %d 项（%s）的 %s 缺失或不是整数；x/y/w/h 必须同时提供"+
					"（position 按整体提交，不做逐字段合并，残缺对象无法表达一个完整位置）",
				i+1, blockID, strings.Join(missing, "/"))
		}
		item := dashboardLayoutItem{
			BlockID: blockID,
			X:       coords["x"],
			Y:       coords["y"],
			W:       coords["w"],
			H:       coords["h"],
		}
		if err := checkLayoutItemBounds(i, item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// checkLayoutItemBounds 拒绝单项坐标在 48 列栅格上不成立的情况。这些坐标交给服务端
// 只会被兜底整理改成别的样子，调用方读不出自己哪里写错了；在本地直接报出具体项更可修。
// 所有违规项一次列全，免得改一处再撞下一处。
func checkLayoutItemBounds(index int, item dashboardLayoutItem) error {
	var problems []string
	if item.X < 0 {
		problems = append(problems, fmt.Sprintf("x=%d 不能为负", item.X))
	}
	if item.Y < 0 {
		problems = append(problems, fmt.Sprintf("y=%d 不能为负", item.Y))
	}
	if item.W <= 0 {
		problems = append(problems, fmt.Sprintf("w=%d 必须 >=1", item.W))
	}
	if item.H <= 0 {
		problems = append(problems, fmt.Sprintf("h=%d 必须 >=1", item.H))
	}
	if item.X >= 0 && item.W > 0 && item.X+item.W > dashboardlayout.GridCols {
		problems = append(problems, fmt.Sprintf("x+w=%d 超出 %d 列", item.X+item.W, dashboardlayout.GridCols))
	}
	if len(problems) == 0 {
		return nil
	}
	return baseFlagErrorf("--layout 第 %d 项（%s）坐标越界：%s；48 列栅格要求 x>=0、y>=0、w>=1、h>=1 且 x+w<=%d",
		index+1, item.BlockID, strings.Join(problems, "，"), dashboardlayout.GridCols)
}

// dryRunDashboardLayout 预览整条链路的三个请求：读现状、批量写、兜底整理。
// 批量写只会带坐标与现状不同的组件，要读完组件列表才知道是哪些，所以预览里列出
// 全部提交的组件，并在说明里交代实际请求的裁剪规则。
func dryRunDashboardLayout(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	items, err := parseDashboardLayoutItems(newParseCtx(runtime), runtime.Str("layout"))
	if err != nil {
		// Validate 已经先跑过同一份解析，这里不可达；返回纯描述而不是半个预览。
		return common.NewDryRunAPI().Desc("invalid --layout: " + err.Error())
	}
	api := dryRunDashboardBase(runtime).Desc(fmt.Sprintf(
		"3-step: list blocks and check that --layout covers every block on the dashboard (%d submitted) → "+
			"write, in one batch, only the blocks whose %d-col coordinates differ from the current ones "+
			"(the preview lists every submitted block; the request is skipped when none differ) → "+
			"always run the server's tidy pass, which fixes holes, overlaps and out-of-bounds blocks.",
		len(items), dashboardlayout.GridCols))
	api.GET("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/blocks").
		Params(map[string]interface{}{"page_size": dashboardBlockListPageSize})
	comps := make([]dashboardlayout.Component, 0, len(items))
	for _, item := range items {
		comps = append(comps, item.component())
	}
	api.POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/arrange").
		Body(dashboardPositionsBody(comps))
	api.POST("/open-apis/base/v3/bases/:base_token/dashboards/:dashboard_id/arrange").
		Body(dashboardTidyBody())
	return api
}

func (item dashboardLayoutItem) component() dashboardlayout.Component {
	return dashboardlayout.Component{ID: item.BlockID, X: item.X, Y: item.Y, W: item.W, H: item.H}
}

// layoutPositionBody 构造 position 对象，与 +dashboard-block-update 的
// position 字段同形（x/y/w/h 四个整数，整体提交）。
func layoutPositionBody(x, y, w, h int) map[string]interface{} {
	return map[string]interface{}{"x": x, "y": y, "w": w, "h": h}
}

// dashboardTidyBody 是兜底整理的请求体，预览与真实请求共用它。
//
// 只能带 tidy_only 与 fine_grid：服务端在 tidy_only 下拒绝 positions / preserve_order /
// reset_sizes（传 false 也不行）。fine_grid 让服务端按 48 列整理，与本命令写入的口径一致。
func dashboardTidyBody() map[string]interface{} {
	return withFineGrid(map[string]interface{}{tidyOnlyKey: true})
}

// executeDashboardLayout 落地整盘布局：读现状 → 校验覆盖 → 只写变化 → 总是兜底整理。
func executeDashboardLayout(runtime *common.RuntimeContext) error {
	items, err := parseDashboardLayoutItems(newParseCtx(runtime), runtime.Str("layout"))
	if err != nil {
		return err
	}
	blocks, err := listAllDashboardBlocks(runtime)
	if err != nil {
		return err
	}
	if err := checkDashboardLayoutCoverage(items, blocks); err != nil {
		return err
	}

	// 只写坐标与现状不同的组件：OT 里不掺同值 op，written_block_ids 也与实际写入
	// 严格对应。读不到现有坐标的组件一律视为有变化。
	current := make(map[string]dashboardlayout.Component, len(blocks))
	for _, block := range blocks {
		if block.HasPosition {
			current[block.ID] = block.Position
		}
	}
	var changed []dashboardlayout.Component
	writtenIDs := []string{}
	for _, item := range items {
		comp := item.component()
		if pos, ok := current[item.BlockID]; ok && samePlacement(pos, comp) {
			continue
		}
		changed = append(changed, comp)
		writtenIDs = append(writtenIDs, item.BlockID)
	}
	// 一次提交：整批要么全部生效、要么全部不生效；没有变化时不发请求。
	if err := submitDashboardPositions(runtime, changed); err != nil {
		return err
	}

	// 兜底整理**无条件**执行，包括上一步没写任何东西的情况：盘上可能本来就有
	// 空洞或重叠（比如建组件时带的坐标就与已有组件冲突），模型交上来的坐标与现状
	// 一致不代表现状是整齐的。
	tidyData, err := baseV3Call(runtime, "POST", baseV3Path("bases", runtime.Str("base-token"), "dashboards", runtime.Str("dashboard-id"), "arrange"), nil, dashboardTidyBody())
	if err != nil {
		fmt.Fprintln(runtime.IO().ErrOut, layoutTidyFailureNotice(writtenIDs))
		return err
	}
	tidiedIDs := readStringList(tidyData[tidiedBlockIDsKey])

	final, ok := finalPositionsFromTidy(tidyData)
	if !ok {
		refreshed, err := listAllDashboardBlocks(runtime)
		if err != nil {
			return err
		}
		final = make(map[string]dashboardlayout.Component, len(refreshed))
		for _, block := range refreshed {
			if block.HasPosition {
				final[block.ID] = block.Position
			}
		}
	}

	if notice := layoutTidyNotice(items, tidiedIDs, final); notice != "" {
		fmt.Fprintln(runtime.IO().ErrOut, notice)
	}

	// layout 按 --layout 的顺序给出整理后的最终坐标；最终坐标缺失的组件（理论上不会
	// 出现）回落到提交的坐标，免得 stdout 少掉一个组件。
	layout := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		comp, ok := final[item.BlockID]
		if !ok {
			comp = item.component()
		}
		layout = append(layout, map[string]interface{}{
			"block_id": item.BlockID,
			"x":        comp.X,
			"y":        comp.Y,
			"w":        comp.W,
			"h":        comp.H,
		})
	}
	runtime.Out(map[string]interface{}{
		"dashboard_id": strings.TrimSpace(runtime.Str("dashboard-id")),
		// grid_cols 是给调用方看的坐标口径说明（layout 里的数是 48 列），不是请求参数；
		// 写请求上的栅格声明是 fine_grid，见 withFineGrid。
		"grid_cols":         dashboardlayout.GridCols,
		"written_block_ids": writtenIDs,
		"tidied_block_ids":  tidiedIDs,
		"layout":            layout,
	}, nil)
	return nil
}

// layoutTidyFailureNotice 是兜底整理失败时写到 stderr 的状态说明。批量写已经落盘、
// 不会回滚，调用方若以为「整条命令失败 = 什么都没写」，会按旧坐标重新规划，把已写入的
// 组件又挪回去。原错误（分类、hint、log_id）原样返回，不改写；状态和恢复做法单独说明。
func layoutTidyFailureNotice(writtenIDs []string) string {
	var state string
	if len(writtenIDs) > 0 {
		state = fmt.Sprintf("前面的批量写入已经生效（%s 的坐标已按 --layout 写入），只有兜底整理没有完成", strings.Join(writtenIDs, ", "))
	} else {
		state = "本次没有需要写入的坐标，盘面未被改动，只有兜底整理没有完成"
	}
	return state + "。不要回滚已写入的坐标；用 +dashboard-block-list 读取当前坐标确认后，" +
		"用同一份 --layout 重新调用 +dashboard-layout 即可补跑兜底整理（坐标已一致的组件不会重复写入）。"
}

// checkDashboardLayoutCoverage 要求 --layout 与盘上的组件一一对应。重复 block_id
// 已在解析期拦下，这里只剩两类：盘上没有的（多交）与盘上有但没交的（漏交）。
func checkDashboardLayoutCoverage(items []dashboardLayoutItem, blocks []dashboardBlockSnapshot) error {
	onBoard := make(map[string]bool, len(blocks))
	for _, block := range blocks {
		onBoard[block.ID] = true
	}
	submitted := make(map[string]bool, len(items))
	var unknown []string
	for _, item := range items {
		submitted[item.BlockID] = true
		if !onBoard[item.BlockID] {
			unknown = append(unknown, item.BlockID)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return baseFlagErrorf(
			"--layout 里的 block_id 不在该仪表盘上：%s；用 +dashboard-block-list 获取真实 block_id",
			strings.Join(unknown, ", "))
	}
	var missing []string
	for _, block := range blocks {
		if !submitted[block.ID] {
			missing = append(missing, block.ID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return baseFlagErrorf(
			"仪表盘上的组件 %s 不在 --layout 里；--layout 必须覆盖盘上全部组件，"+
				"没有移动的现有组件也要按 +dashboard-block-list 读到的坐标一并写进来",
			strings.Join(missing, ", "))
	}
	return nil
}

func samePlacement(a, b dashboardlayout.Component) bool {
	return a.X == b.X && a.Y == b.Y && a.W == b.W && a.H == b.H
}

// finalPositionsFromTidy 从兜底整理的返回里取整盘最终坐标（由 standard_position 换算）。
// 返回里没有组件列表、或有组件缺像素坐标时 ok=false，调用方再读一次组件列表。
func finalPositionsFromTidy(data map[string]interface{}) (map[string]dashboardlayout.Component, bool) {
	list, _ := data[tidyResponseBlocksKey].([]interface{})
	if len(list) == 0 {
		return nil, false
	}
	out := make(map[string]dashboardlayout.Component, len(list))
	for _, entry := range list {
		obj, ok := entry.(map[string]interface{})
		if !ok {
			return nil, false
		}
		id, _ := obj["block_id"].(string)
		id = strings.TrimSpace(id)
		px, hasPixel := readFloatRect(obj[standardPositionKey])
		if id == "" || !hasPixel {
			return nil, false
		}
		x, y, w, h := dashboardlayout.GridFromPixels(px[0], px[1], px[2], px[3])
		out[id] = dashboardlayout.Component{ID: id, X: x, Y: y, W: w, H: h}
	}
	return out, true
}

// readStringList 读出一个字符串数组，非字符串项与空串跳过；字段缺失时返回空切片，
// 保证 stdout 里恒为数组而不是 null。
func readStringList(v interface{}) []string {
	out := []string{}
	list, _ := v.([]interface{})
	for _, entry := range list {
		if s, ok := entry.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

// layoutTidyNoticePrefix 是「兜底整理挪动了组件」提示的固定开头。
const layoutTidyNoticePrefix = "兜底整理调整了组件位置"

// layoutTidyNotice 逐个说明兜底整理把组件从哪挪到了哪，没有组件被挪动时返回空串。
// 陈述句：整理结果已经落盘，stdout 的 layout 就是最终坐标，调用方不需要再做什么。
func layoutTidyNotice(items []dashboardLayoutItem, tidiedIDs []string, final map[string]dashboardlayout.Component) string {
	if len(tidiedIDs) == 0 {
		return ""
	}
	planned := make(map[string]dashboardlayout.Component, len(items))
	for _, item := range items {
		planned[item.BlockID] = item.component()
	}
	moves := make([]string, 0, len(tidiedIDs))
	for _, id := range tidiedIDs {
		from, hasFrom := planned[id]
		to, hasTo := final[id]
		if !hasFrom || !hasTo {
			moves = append(moves, fmt.Sprintf("%s 的位置已调整", id))
			continue
		}
		moves = append(moves, fmt.Sprintf("%s 从 %s 移到 %s", id, formatPlacement(from), formatPlacement(to)))
	}
	return fmt.Sprintf("%s（处理空洞、重叠或越界）：%s。stdout 的 layout 是整理后的最终坐标。",
		layoutTidyNoticePrefix, strings.Join(moves, "；"))
}

func formatPlacement(c dashboardlayout.Component) string {
	return fmt.Sprintf("(x=%d,y=%d,w=%d,h=%d)", c.X, c.Y, c.W, c.H)
}
