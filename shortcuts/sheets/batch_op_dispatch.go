// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
)

// ─── +batch-update sub-op dispatch ─────────────────────────────────────
//
// 用户传给 +batch-update --operations 的形态是 CLI 视角的 {shortcut, input}：
//
//     [{"shortcut": "+range-copy", "input": {"sheet_id":"...","source-range":"A1:B2","target-range":"A10"}}, ...]
//
// input 里用的是该 shortcut 的 **CLI flag 名**（与 standalone 调用一致；连字符 /
// 下划线两种写法都接受）。底层 MCP batch_update tool 要的是
// {tool_name, input(MCP body)} —— body 的字段名往往与 CLI flag 名不同
// （如 +range-copy 的 source-range/target-range 要翻成 range/destination_range）。
//
// 关键：每个子操作复用 **standalone shortcut 同一套 flag→body translator**
// （那些 *Input 构建函数，现在统一接收 flagView 接口）。这样 batch 子操作
// 产出的 MCP body 与该 shortcut 单独调用产出的 body 完全一致（由
// batch-vs-standalone 契约测试保证）。dispatch 表只列**可纳入 atomic batch
// 的 write shortcut**——读操作、fan-out wrapper（+batch-update 自身、
// +cells-batch-set-style、+cells-batch-clear、+dropdown-{update,delete}）一律不放进表里，
// 用户传到 +batch-update 里会被 translator 拒绝。

// batchTranslateFn turns a sub-op's CLI-shape input (via flagView) into the MCP
// tool body for the underlying batch_update sub-tool. token is the
// +batch-update top-level spreadsheet token; sheetID/sheetName are the resolved
// sheet selector for this sub-op. The returned body already carries excel_id
// and (where the tool needs one) the operation discriminator — exactly as the
// standalone shortcut would emit.
type batchTranslateFn func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error)

type batchOpMapping struct {
	// mcpToolName 是底层 MCP batch_update 接受的 tool_name。
	mcpToolName string
	// translate 复用 standalone 的 *Input 构建逻辑，产出 MCP body。
	translate batchTranslateFn
}

// sheetSelectorFlagsForSubOp returns the (id, name) flag names a +batch-update
// sub-op uses to express its placement / context sheet. Defaults are
// `sheet-id` / `sheet-name`; +pivot-create deviates because its create
// shortcut renamed the placement selector to `target-sheet-id` /
// `target-sheet-name` (the data-source sheet is encoded in --source as
// `'SheetName'!Range`, not in a sheet selector flag). Update / delete on
// pivot still use the default names — only the create create-side
// shortcut was renamed.
func sheetSelectorFlagsForSubOp(shortcut string) (string, string) {
	if shortcut == "+pivot-create" {
		return "target-sheet-id", "target-sheet-name"
	}
	return "sheet-id", "sheet-name"
}

// objCreateTranslate / objUpdateTranslate / objDeleteTranslate bind an object
// CRUD spec to the shared object_crud builders.
func objCreateTranslate(spec objectCRUDSpec) batchTranslateFn {
	return func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
		return objectCreateInput(fv, token, sheetID, sheetName, spec)
	}
}

func objUpdateTranslate(spec objectCRUDSpec) batchTranslateFn {
	return func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
		return objectUpdateInput(fv, token, sheetID, sheetName, spec)
	}
}

func objDeleteTranslate(spec objectCRUDSpec) batchTranslateFn {
	return func(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
		return objectDeleteInput(fv, token, sheetID, sheetName, spec)
	}
}

// batchOpDispatch covers every write shortcut that can join an atomic batch.
// Each entry plugs the shortcut's standalone xxxInput builder into the
// batch translator path — so the body is byte-identical to the standalone
// invocation (locked by TestBatchOp_BodyMatchesStandalone) and the missing-
// flag error is identical too (locked by TestBatchOp_ErrorEquivalence).
var batchOpDispatch = map[string]batchOpMapping{
	// ─── 单元格内容 ──────────────────────────────────────────────────
	"+cells-set":       {"set_cell_range", cellsSetInput},
	"+cells-set-style": {"set_cell_range", cellsSetStyleInput},
	"+cells-clear":     {"clear_cell_range", cellsClearInput},
	"+cells-replace":   {"replace_data", replaceInput},
	"+csv-put":         {"set_range_from_csv", csvPutInput},
	"+dropdown-set":    {"set_cell_range", dropdownSetInput},

	// ─── 单元格合并 (merge_cells, operation 区分) ────────────────────
	"+cells-merge": {"merge_cells", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return mergeInput(fv, token, sid, sname, "merge", true)
	}},
	"+cells-unmerge": {"merge_cells", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return mergeInput(fv, token, sid, sname, "unmerge", false)
	}},

	// ─── 行列结构 (modify_sheet_structure, operation 区分) ──────────
	"+dim-insert": {"modify_sheet_structure", dimInsertInput},
	"+dim-delete": {"modify_sheet_structure", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return dimRangeOpInput(fv, token, sid, sname, "delete")
	}},
	"+dim-hide": {"modify_sheet_structure", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return dimRangeOpInput(fv, token, sid, sname, "hide")
	}},
	"+dim-unhide": {"modify_sheet_structure", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return dimRangeOpInput(fv, token, sid, sname, "unhide")
	}},
	"+dim-freeze": {"modify_sheet_structure", dimFreezeInput},
	"+dim-group": {"modify_sheet_structure", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return dimGroupInput(fv, token, sid, sname, "group")
	}},
	"+dim-ungroup": {"modify_sheet_structure", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return dimGroupInput(fv, token, sid, sname, "ungroup")
	}},

	// ─── 行高列宽 (resize_range, 无 operation 字段) ─────────────────
	"+rows-resize": {"resize_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return resizeInput(fv, token, sid, sname, "row")
	}},
	"+cols-resize": {"resize_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return resizeInput(fv, token, sid, sname, "column")
	}},

	// ─── 区域操作 (transform_range, operation 区分) ─────────────────
	"+range-move": {"transform_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return transformMoveCopyInput(fv, token, sid, sname, "move", false)
	}},
	"+range-copy": {"transform_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		return transformMoveCopyInput(fv, token, sid, sname, "copy", true)
	}},
	"+range-fill": {"transform_range", rangeFillInput},
	"+range-sort": {"transform_range", rangeSortInput},

	// ─── 工作簿 / 子表 (modify_workbook_structure, operation 区分) ──
	"+sheet-create": {"modify_workbook_structure", func(fv flagView, token, _, _ string) (map[string]interface{}, error) {
		return sheetCreateInput(fv, token)
	}},
	"+sheet-delete": {"modify_workbook_structure", sheetDeleteInput},
	"+sheet-rename": {"modify_workbook_structure", sheetRenameInput},
	"+sheet-move":   {"modify_workbook_structure", sheetMoveBatchInput},
	"+sheet-copy":   {"modify_workbook_structure", sheetCopyInput},
	"+sheet-hide": {"modify_workbook_structure", func(fv flagView, t, sid, sn string) (map[string]interface{}, error) {
		return sheetVisibilityInput(fv, t, sid, sn, "hide")
	}},
	"+sheet-unhide": {"modify_workbook_structure", func(fv flagView, t, sid, sn string) (map[string]interface{}, error) {
		return sheetVisibilityInput(fv, t, sid, sn, "unhide")
	}},
	"+sheet-set-tab-color": {"modify_workbook_structure", sheetSetTabColorInput},
	"+sheet-show-gridline": {"modify_workbook_structure", func(fv flagView, t, sid, sn string) (map[string]interface{}, error) {
		return sheetVisibilityInput(fv, t, sid, sn, "show_gridline")
	}},
	"+sheet-hide-gridline": {"modify_workbook_structure", func(fv flagView, t, sid, sn string) (map[string]interface{}, error) {
		return sheetVisibilityInput(fv, t, sid, sn, "hide_gridline")
	}},

	// ─── 对象族 CRUD (manage_*_object, operation 区分) ─────────────
	"+chart-create": {"manage_chart_object", objCreateTranslate(chartSpec)},
	"+chart-update": {"manage_chart_object", objUpdateTranslate(chartSpec)},
	"+chart-delete": {"manage_chart_object", objDeleteTranslate(chartSpec)},

	"+pivot-create": {"manage_pivot_table_object", objCreateTranslate(pivotSpec)},
	"+pivot-update": {"manage_pivot_table_object", objUpdateTranslate(pivotSpec)},
	"+pivot-delete": {"manage_pivot_table_object", objDeleteTranslate(pivotSpec)},

	"+cond-format-create": {"manage_conditional_format_object", objCreateTranslate(condFormatSpec)},
	"+cond-format-update": {"manage_conditional_format_object", objUpdateTranslate(condFormatSpec)},
	"+cond-format-delete": {"manage_conditional_format_object", objDeleteTranslate(condFormatSpec)},

	"+filter-create": {"manage_filter_object", filterCreateInput},
	"+filter-update": {"manage_filter_object", filterUpdateInput},
	"+filter-delete": {"manage_filter_object", filterDeleteInput},

	"+filter-view-create": {"manage_filter_view_object", objCreateTranslate(filterViewSpec)},
	"+filter-view-update": {"manage_filter_view_object", objUpdateTranslate(filterViewSpec)},
	"+filter-view-delete": {"manage_filter_view_object", objDeleteTranslate(filterViewSpec)},

	"+sparkline-create": {"manage_sparkline_object", objCreateTranslate(sparklineSpec)},
	"+sparkline-update": {"manage_sparkline_object", objUpdateTranslate(sparklineSpec)},
	"+sparkline-delete": {"manage_sparkline_object", objDeleteTranslate(sparklineSpec)},

	"+float-image-create": {"manage_float_image_object", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		if err := rejectLocalImageInBatch(fv); err != nil {
			return nil, err
		}
		return floatImageWriteInput(fv, token, sid, sname, "create", false, "")
	}},
	"+float-image-update": {"manage_float_image_object", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		if err := rejectLocalImageInBatch(fv); err != nil {
			return nil, err
		}
		return floatImageWriteInput(fv, token, sid, sname, "update", true, "")
	}},
	"+float-image-delete": {"manage_float_image_object", objDeleteTranslate(floatImageDeleteSpec)},
}

// rejectLocalImageInBatch blocks the local-file --image source inside
// +batch-update: a batch sub-op has no upload phase, so the file could not be
// turned into a file_token. Callers must pass --image-token / --image-uri.
func rejectLocalImageInBatch(fv flagView) error {
	if strings.TrimSpace(fv.Str("image")) != "" {
		return sheetsValidationForFlag("image", "--image (local upload) is not supported inside +batch-update; pass --image-token or --image-uri instead")
	}
	return nil
}

// sheetMoveBatchInput translates +sheet-move inside a batch. Unlike the
// standalone shortcut it cannot issue the get_workbook_structure read that
// auto-derives sheet_id / source_index, so both must be supplied explicitly.
func sheetMoveBatchInput(fv flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if sheetID == "" {
		return nil, sheetsValidationForFlag("sheet-id", "+sheet-move in +batch-update requires sheet_id (sheet_name needs a network lookup unavailable mid-batch)")
	}
	if !fv.Changed("source-index") {
		return nil, sheetsValidationForFlag("source-index", "+sheet-move in +batch-update requires source_index (auto-derive needs a network lookup unavailable mid-batch)")
	}
	if fv.Int("source-index") < 0 {
		return nil, sheetsValidationForFlag("source-index", "--source-index must be >= 0")
	}
	// Standalone +sheet-move requires --index (see SheetMove.Validate). A batch
	// sub-op skips that path, and mapFlagView falls back to the flag default (0),
	// which would silently move the sheet to the front. Require it explicitly so
	// the batch contract matches the standalone one.
	if !fv.Changed("index") {
		return nil, sheetsValidationForFlag("index", "+sheet-move in +batch-update requires index")
	}
	if fv.Int("index") < 0 {
		return nil, sheetsValidationForFlag("index", "--index must be >= 0")
	}
	return map[string]interface{}{
		"excel_id":     token,
		"operation":    "move",
		"sheet_id":     sheetID,
		"source_index": fv.Int("source-index"),
		"target_index": fv.Int("index"),
	}, nil
}

// reservedSubOpKeys 是禁止用户在 sub-op input 里手填的 key —— 它们由
// +batch-update 顶层 --url/--token 统一提供（excel_id / spreadsheet_token / url）。
var reservedSubOpKeys = []string{"excel_id", "spreadsheet_token", "url"}

// translateBatchOp 把一个 CLI 视角的 {shortcut, input} 翻成底层 MCP
// batch_update 的 {tool_name, input}。`index` 用于错误信息定位。input 用
// shortcut 的 CLI flag 名（连字符/下划线均可），经该 shortcut 的 standalone
// translator 翻成 MCP body。
//
// 失败场景：
//   - shortcut 字段缺失 / 非 string
//   - shortcut 不在 dispatch 表（拼写错；read 操作；嵌套 fan-out wrapper）
//   - input 不是 object
//   - input 里手填了 operation（由 shortcut 名隐含，禁手填以防 mismatch）
//   - input 里手填了 excel_id / spreadsheet_token / url
//   - 子操作的 translator 报错（如缺必填字段）
func translateBatchOp(raw interface{}, token string, index int) (map[string]interface{}, error) {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return nil, sheetsValidationForFlag("operations", "operations[%d] must be a JSON object", index)
	}
	scRaw, present := op["shortcut"]
	if !present {
		return nil, sheetsValidationForFlag("operations", "operations[%d]: 'shortcut' field is required", index)
	}
	sc, ok := scRaw.(string)
	if !ok || sc == "" {
		return nil, sheetsValidationForFlag("operations", "operations[%d]: 'shortcut' must be a non-empty string (got %T)", index, scRaw)
	}
	mapping, ok := batchOpDispatch[sc]
	if !ok {
		return nil, sheetsValidationForFlag(
			"operations",
			"operations[%d]: shortcut %q not allowed in +batch-update "+
				"(read ops / fan-out wrappers like +batch-update / +cells-batch-set-style / +cells-batch-clear / +dropdown-{update,delete} are excluded; "+
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
			return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): 'input' must be a JSON object (got %T)", index, sc, inputRaw)
		}
	}
	// 禁手填 operation —— 由 shortcut 名表达，手填易与 shortcut 不一致。
	if _, has := input["operation"]; has {
		return nil, sheetsValidationForFlag(
			"operations",
			"operations[%d] (%s): do not pass input.operation manually — it is implied by the shortcut name",
			index, sc,
		)
	}
	// 禁在 sub-op 重复填 spreadsheet 定位 —— 由 +batch-update 顶层 --url/--token 统一提供。
	for _, k := range reservedSubOpKeys {
		if _, has := input[k]; has {
			return nil, sheetsValidationForFlag(
				"operations",
				"operations[%d] (%s): do not pass input.%s — it is already set from +batch-update top-level --url / --token",
				index, sc, k,
			)
		}
	}
	// 拒绝任何额外的 sub-op 顶层 key（防御未来 schema drift / 用户笔误）。
	for k := range op {
		if k != "shortcut" && k != "input" {
			return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): unknown top-level key %q (expected only 'shortcut' and 'input')", index, sc, k)
		}
	}
	fv := newMapFlagViewForCommand(sc, input)
	// operations is skipped by parse-time schema validation, so type-check the
	// sub-op's scalar fields here before the translator reads them via
	// Int/Bool/Float64 (which would otherwise coerce a wrong type to zero).
	if err := fv.validateRawTypes(); err != nil {
		return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
	}
	sheetIDFlag, sheetNameFlag := sheetSelectorFlagsForSubOp(sc)
	sheetID := strings.TrimSpace(fv.Str(sheetIDFlag))
	sheetName := strings.TrimSpace(fv.Str(sheetNameFlag))
	body, err := mapping.translate(fv, token, sheetID, sheetName)
	if err != nil {
		return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
	}
	return map[string]interface{}{
		"tool_name": mapping.mcpToolName,
		"input":     body,
	}, nil
}

// translateBatchOperations 翻译整个 ops 数组；fail-fast，遇错立即返回。
func translateBatchOperations(rawOps []interface{}, token string) ([]interface{}, error) {
	if len(rawOps) == 0 {
		return nil, sheetsValidationForFlag("operations", "--operations must be a non-empty JSON array")
	}
	out := make([]interface{}, 0, len(rawOps))
	for i, raw := range rawOps {
		translated, err := translateBatchOp(raw, token, i)
		if err != nil {
			return nil, err
		}
		out = append(out, translated)
	}
	return out, nil
}
