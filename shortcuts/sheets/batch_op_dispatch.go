// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── +batch-update sub-op dispatch ─────────────────────────────────────
//
// 用户传给 +batch-update --operations 的形态是 CLI 视角的 {shortcut, input}：
//
//     [{"shortcut": "+dim-insert", "input": {...}}, ...]
//
// 而底层 MCP batch_update tool 的契约是 {tool_name, input} —— input 里某些
// MCP tool 还需要 operation 字段区分动作（如 modify_sheet_structure
// 的 insert / delete / hide / unhide / freeze / group / ungroup）。
//
// translateBatchOp 做这层翻译：查表 shortcut → mcpToolName + 可选 operation，
// 然后把 operation 注入 input.operation。dispatch 表只列**可纳入 atomic
// batch 的 write shortcut**——读操作、fan-out wrapper（包括 +batch-update
// 自身）、走 legacy v2 endpoint 的 shortcut（如 +dim-move）、需要多步副作用
// 的 shortcut（如 +cells-set-image / +workbook-create）一律不放进表里，
// 用户传到 +batch-update 里会被 translator 拒绝。

type batchOpMapping struct {
	// mcpToolName 是底层 MCP batch_update 接受的 tool_name。
	mcpToolName string
	// operationField 注入到 input.operation 的值；空 = 不注入（MCP tool 没有
	// operation 字段，如 set_cell_range / clear_cell_range / replace_data /
	// set_range_from_csv / resize_range）。
	operationField string
}

// batchOpDispatch 全表 41 项，覆盖 sheet skill 下所有可 batch 的 write shortcut。
// 增删请同步 canonical-spec/tool-schemas/cli-schemas.json 的 shortcut enum。
var batchOpDispatch = map[string]batchOpMapping{
	// ─── 单元格内容 ──────────────────────────────────────────────────
	"+cells-set":       {mcpToolName: "set_cell_range"},
	"+cells-set-style": {mcpToolName: "set_cell_range"},
	"+cells-clear":     {mcpToolName: "clear_cell_range"},
	"+cells-replace":   {mcpToolName: "replace_data"},
	"+csv-put":         {mcpToolName: "set_range_from_csv"},
	"+dropdown-set":    {mcpToolName: "set_cell_range"},

	// ─── 单元格合并 (merge_cells, operation 区分) ────────────────────
	"+cells-merge":   {mcpToolName: "merge_cells", operationField: "merge"},
	"+cells-unmerge": {mcpToolName: "merge_cells", operationField: "unmerge"},

	// ─── 行列结构 (modify_sheet_structure, operation 区分) ──────────
	// 注意：+dim-move 不在此 — 单 shortcut 走 legacy v2 dimension_range
	// endpoint，不经 MCP，无法 batch。
	// +dim-freeze 静态注入 operation="freeze"，单 shortcut 里基于 count==0
	// 切换 unfreeze 的路径在 batch 里不支持（用户要 unfreeze 用单 shortcut）。
	"+dim-insert":  {mcpToolName: "modify_sheet_structure", operationField: "insert"},
	"+dim-delete":  {mcpToolName: "modify_sheet_structure", operationField: "delete"},
	"+dim-hide":    {mcpToolName: "modify_sheet_structure", operationField: "hide"},
	"+dim-unhide":  {mcpToolName: "modify_sheet_structure", operationField: "unhide"},
	"+dim-freeze":  {mcpToolName: "modify_sheet_structure", operationField: "freeze"},
	"+dim-group":   {mcpToolName: "modify_sheet_structure", operationField: "group"},
	"+dim-ungroup": {mcpToolName: "modify_sheet_structure", operationField: "ungroup"},

	// ─── 行高列宽 (resize_range, 无 operation 字段) ─────────────────
	// row/column 通过 input.resize_height vs input.resize_width 顶层 key 表达。
	"+rows-resize": {mcpToolName: "resize_range"},
	"+cols-resize": {mcpToolName: "resize_range"},

	// ─── 区域操作 (transform_range, operation 区分) ─────────────────
	"+range-move": {mcpToolName: "transform_range", operationField: "move"},
	"+range-copy": {mcpToolName: "transform_range", operationField: "copy"},
	"+range-fill": {mcpToolName: "transform_range", operationField: "fill"},
	"+range-sort": {mcpToolName: "transform_range", operationField: "sort"},

	// ─── 工作簿 / 子表 (modify_workbook_structure, operation 区分) ──
	"+sheet-create":        {mcpToolName: "modify_workbook_structure", operationField: "create"},
	"+sheet-delete":        {mcpToolName: "modify_workbook_structure", operationField: "delete"},
	"+sheet-rename":        {mcpToolName: "modify_workbook_structure", operationField: "rename"},
	"+sheet-move":          {mcpToolName: "modify_workbook_structure", operationField: "move"},
	"+sheet-copy":          {mcpToolName: "modify_workbook_structure", operationField: "copy"},
	"+sheet-hide":          {mcpToolName: "modify_workbook_structure", operationField: "hide"},
	"+sheet-unhide":        {mcpToolName: "modify_workbook_structure", operationField: "unhide"},
	"+sheet-set-tab-color": {mcpToolName: "modify_workbook_structure", operationField: "set_tab_color"},

	// ─── 对象族 CRUD (manage_*_object, operation 区分) ─────────────
	"+chart-create": {mcpToolName: "manage_chart_object", operationField: "create"},
	"+chart-update": {mcpToolName: "manage_chart_object", operationField: "update"},
	"+chart-delete": {mcpToolName: "manage_chart_object", operationField: "delete"},

	"+pivot-create": {mcpToolName: "manage_pivot_table_object", operationField: "create"},
	"+pivot-update": {mcpToolName: "manage_pivot_table_object", operationField: "update"},
	"+pivot-delete": {mcpToolName: "manage_pivot_table_object", operationField: "delete"},

	"+cond-format-create": {mcpToolName: "manage_conditional_format_object", operationField: "create"},
	"+cond-format-update": {mcpToolName: "manage_conditional_format_object", operationField: "update"},
	"+cond-format-delete": {mcpToolName: "manage_conditional_format_object", operationField: "delete"},

	"+filter-create": {mcpToolName: "manage_filter_object", operationField: "create"},
	"+filter-update": {mcpToolName: "manage_filter_object", operationField: "update"},
	"+filter-delete": {mcpToolName: "manage_filter_object", operationField: "delete"},

	"+filter-view-create": {mcpToolName: "manage_filter_view_object", operationField: "create"},
	"+filter-view-update": {mcpToolName: "manage_filter_view_object", operationField: "update"},
	"+filter-view-delete": {mcpToolName: "manage_filter_view_object", operationField: "delete"},

	"+sparkline-create": {mcpToolName: "manage_sparkline_object", operationField: "create"},
	"+sparkline-update": {mcpToolName: "manage_sparkline_object", operationField: "update"},
	"+sparkline-delete": {mcpToolName: "manage_sparkline_object", operationField: "delete"},

	"+float-image-create": {mcpToolName: "manage_float_image_object", operationField: "create"},
	"+float-image-update": {mcpToolName: "manage_float_image_object", operationField: "update"},
	"+float-image-delete": {mcpToolName: "manage_float_image_object", operationField: "delete"},
}

// reservedSubOpKeys 是禁止用户在 sub-op input 里手填的 key —— 它们要么由
// shortcut 名隐含（operation），要么由 +batch-update 顶层 --url/--token
// 统一提供（excel_id / spreadsheet_token / url）。
var reservedSubOpKeys = []string{"excel_id", "spreadsheet_token", "url"}

// translateBatchOp 把一个 CLI 视角的 {shortcut, input} 翻成底层 MCP
// batch_update 的 {tool_name, input(+operation)}。`index` 用于错误信息定位。
//
// 失败场景：
//   - shortcut 字段缺失 / 非 string
//   - shortcut 不在 dispatch 表（典型：拼写错；用户传了 read 操作；
//     用户嵌套 +batch-update / +cells-batch-set-style 之类的 fan-out wrapper）
//   - input 不是 object
//   - input 里手填了 operation（由 shortcut 名隐含，禁手填以防 mismatch）
//   - input 里手填了 excel_id / spreadsheet_token / url
func translateBatchOp(raw interface{}, index int) (map[string]interface{}, error) {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return nil, common.FlagErrorf("operations[%d] must be a JSON object", index)
	}
	scRaw, present := op["shortcut"]
	if !present {
		return nil, common.FlagErrorf("operations[%d]: 'shortcut' field is required", index)
	}
	sc, ok := scRaw.(string)
	if !ok || sc == "" {
		return nil, common.FlagErrorf("operations[%d]: 'shortcut' must be a non-empty string (got %T)", index, scRaw)
	}
	mapping, ok := batchOpDispatch[sc]
	if !ok {
		return nil, common.FlagErrorf(
			"operations[%d]: shortcut %q not allowed in +batch-update "+
				"(read ops / fan-out wrappers like +batch-update / +cells-batch-set-style / +dropdown-{update,delete} / +dim-move are excluded; "+
				"run `lark-cli sheets +batch-update --print-schema --flag-name operations` to see the full enum)",
			index, sc,
		)
	}
	inputRaw, hasInput := op["input"]
	var input map[string]interface{}
	if !hasInput || inputRaw == nil {
		input = map[string]interface{}{}
	} else {
		input, ok = inputRaw.(map[string]interface{})
		if !ok {
			return nil, common.FlagErrorf("operations[%d] (%s): 'input' must be a JSON object (got %T)", index, sc, inputRaw)
		}
	}
	// 禁手填 operation —— 由 shortcut 名表达，手填易与 shortcut 不一致。
	if _, has := input["operation"]; has {
		return nil, common.FlagErrorf(
			"operations[%d] (%s): do not pass input.operation manually — it is implied by the shortcut name",
			index, sc,
		)
	}
	// 禁在 sub-op 重复填 spreadsheet 定位 —— 由 +batch-update 顶层 --url/--token 统一提供。
	for _, k := range reservedSubOpKeys {
		if _, has := input[k]; has {
			return nil, common.FlagErrorf(
				"operations[%d] (%s): do not pass input.%s — it is already set from +batch-update top-level --url / --token",
				index, sc, k,
			)
		}
	}
	// 拒绝任何额外的 sub-op 顶层 key（防御未来 schema drift / 用户笔误）。
	for k := range op {
		if k != "shortcut" && k != "input" {
			return nil, common.FlagErrorf("operations[%d] (%s): unknown top-level key %q (expected only 'shortcut' and 'input')", index, sc, k)
		}
	}
	// 浅拷贝 input，注入 operation（如有），再补 excel_id 由调用方统一注入到顶层后，
	// translator 也把 excel_id 写进 sub-op input（MCP tool 要求每个 sub-tool 都带）。
	out := make(map[string]interface{}, len(input)+1)
	for k, v := range input {
		out[k] = v
	}
	if mapping.operationField != "" {
		out["operation"] = mapping.operationField
	}
	return map[string]interface{}{
		"tool_name": mapping.mcpToolName,
		"input":     out,
	}, nil
}

// translateBatchOperations 翻译整个 ops 数组；fail-fast，遇错立即返回。
// 翻译后会把 excel_id 注入每个 sub-op 的 input（MCP 契约要求）。
func translateBatchOperations(rawOps []interface{}, token string) ([]interface{}, error) {
	if len(rawOps) == 0 {
		return nil, common.FlagErrorf("--operations must be a non-empty JSON array")
	}
	out := make([]interface{}, 0, len(rawOps))
	for i, raw := range rawOps {
		translated, err := translateBatchOp(raw, i)
		if err != nil {
			return nil, err
		}
		// MCP batch_update 每个 sub-tool 的 input 都需要 excel_id（与单调用一致）。
		input := translated["input"].(map[string]interface{})
		input["excel_id"] = token
		out = append(out, translated)
	}
	return out, nil
}

// 仅供测试 / 调试：暴露已知 shortcut 列表，便于做 enum 漂移对账。
func batchOpDispatchKeys() []string {
	keys := make([]string, 0, len(batchOpDispatch))
	for k := range batchOpDispatch {
		keys = append(keys, k)
	}
	return keys
}
