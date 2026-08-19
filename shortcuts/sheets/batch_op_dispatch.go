// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/suggest"
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
	"+cells-set": {"set_cell_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		// The --writes plural form expands into its own atomic batch and
		// cannot nest; sub-ops carry one range+cells each.
		if fv.Changed("writes") {
			return nil, sheetsValidationForFlag("writes", `"writes" is not supported inside +batch-update (it expands into its own batch request); call +cells-set --writes standalone, or give each sub-op a single range + cells`)
		}
		return cellsSetInput(fv, token, sid, sname)
	}},
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
		// The --ranges plural form expands into its own atomic batch and
		// cannot nest; sub-ops carry one range each.
		if fv.Changed("ranges") {
			return nil, sheetsValidationForFlag("ranges", `"ranges" is not supported inside +batch-update (it expands into its own batch request); call +dim-delete --ranges standalone, or give each sub-op a single "range"`)
		}
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
	// The map form (--heights/--widths) fans out into its own batch_update
	// and cannot nest inside +batch-update; sub-ops must use the uniform
	// single-range form (range + height/width or type).
	"+rows-resize": {"resize_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		if err := rejectResizeMapInBatch(fv, "row"); err != nil {
			return nil, err
		}
		return resizeInput(fv, token, sid, sname, "row")
	}},
	"+cols-resize": {"resize_range", func(fv flagView, token, sid, sname string) (map[string]interface{}, error) {
		if err := rejectResizeMapInBatch(fv, "column"); err != nil {
			return nil, err
		}
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

// allowedBatchShortcuts lists every shortcut accepted inside +batch-update,
// sorted, for the not-allowed error hint.
func allowedBatchShortcuts() []string {
	out := make([]string, 0, len(batchOpDispatch))
	for sc := range batchOpDispatch {
		out = append(out, sc)
	}
	sort.Strings(out)
	return out
}

// subOpInputContract renders one shortcut's complete sub-op key vocabulary
// (wire-style underscore names) for the translator-failure hint: required
// flags are marked, the sheet selector pair collapses to a choose-one, and
// spreadsheet locators are omitted (reserved for the batch top level).
// Returns "" for shortcuts without a flag-defs entry.
func subOpInputContract(sc string) string {
	defs, _ := loadFlagDefs()
	spec, ok := defs[sc]
	if !ok {
		return ""
	}
	idFlag, nameFlag := sheetSelectorFlagsForSubOp(sc)
	var keys []string
	sheetSelector := ""
	for _, df := range spec.Flags {
		if df.Kind == "system" || df.Hidden {
			continue
		}
		switch df.Name {
		case "url", "spreadsheet-token":
			continue // reserved: supplied by +batch-update top level
		case idFlag, nameFlag:
			sheetSelector = strings.ReplaceAll(idFlag, "-", "_") + "|" + strings.ReplaceAll(nameFlag, "-", "_") + " (choose one)"
			continue
		}
		key := strings.ReplaceAll(df.Name, "-", "_")
		if df.Required == "required" {
			key += " (required)"
		}
		keys = append(keys, key)
	}
	if sheetSelector != "" {
		keys = append([]string{sheetSelector}, keys...)
	}
	return strings.Join(keys, ", ")
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

// wrappedSubOpInputKeys are nested MCP-body container keys that must never
// appear at a sub-op input's top level — their presence means the caller
// pasted a shortcut's structured *output* (e.g. a {"cell_styles":{…}} block)
// where the flattened flag keys belong. None of the batch sub-op translators
// read input under these names, so rejecting them is safe.
var wrappedSubOpInputKeys = []string{"cell_styles", "cell_merges", "styles"}

// subOpKeyVocabulary returns the set of hyphen-canonical flag names a sub-op
// input may carry for `sc`: every non-system flag in flag-defs except the
// spreadsheet locators (reserved for the batch top level). Nil when the
// shortcut has no flag-defs entry (vocabulary checks are then skipped).
func subOpKeyVocabulary(sc string) map[string]bool {
	defs, _ := loadFlagDefs()
	spec, ok := defs[sc]
	if !ok {
		return nil
	}
	vocab := make(map[string]bool, len(spec.Flags))
	for _, df := range spec.Flags {
		if df.Kind == "system" || df.Name == "url" || df.Name == "spreadsheet-token" {
			continue
		}
		vocab[df.Name] = true
	}
	return vocab
}

// camelToKebab converts a lowerCamelCase key to its kebab form
// (sheetName → sheet-name). Returns "" when the key carries no uppercase
// letter (nothing to convert).
func camelToKebab(key string) string {
	if strings.ToLower(key) == key {
		return ""
	}
	var b strings.Builder
	for i, r := range key {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r + ('a' - 'A'))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// normalizeSubOpInputKeys validates every sub-op input key against the
// shortcut's flag vocabulary, rewriting habitual spellings in place and
// rejecting anything that matches nothing. Eval traces show unknown keys were
// previously ignored silently, which turned "wrong key" (size for width,
// camelCase sheetName, an invented styles object) into misleading
// "missing required flag" errors downstream — the single largest batch error
// cluster. Rewrites applied, in order:
//
//   - underscore ↔ hyphen forms of a declared flag (already tolerated by
//     mapFlagView — accepted here as-is)
//   - lowerCamelCase → the declared flag (sheetName → sheet_name)
//   - the command's intuitive-alias table (size → width/height on the resize
//     pair) — the same commandFlagAliases the cobra path applies
//   - "ranges" with a single-entry array unwraps onto "range"; a multi-entry
//     array gets a split-into-sub-ops prescription instead
//
// Anything else errors with a did-you-mean. Returns a bare error; the caller
// wraps it with the operations[i] (<shortcut>) context and key contract.
func normalizeSubOpInputKeys(sc string, input map[string]interface{}) error {
	vocab := subOpKeyVocabulary(sc)
	if vocab == nil {
		return nil
	}
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	aliases := commandFlagAliases[sc]
	// canonical tracks which raw key already claimed each logical key, so two
	// spellings of the same flag (sheet-id / sheet_id / sheetId) can never both
	// survive into the tool body — the flag view resolves hyphen↔underscore
	// variants, so a leftover duplicate would be silently shadowed and could
	// send the write to the wrong sheet.
	canonical := map[string]string{}
	claim := func(logical, raw string) error {
		if prev, taken := canonical[logical]; taken {
			if jsonEqual(input[prev], input[raw]) {
				return nil // same value under two spellings: harmless
			}
			return fmt.Errorf("%s got conflicting values for %q under two spellings (%q and %q) — keep one", sc, strings.ReplaceAll(logical, "-", "_"), prev, raw) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
		}
		canonical[logical] = raw
		return nil
	}
	for _, k := range keys {
		hv := strings.ReplaceAll(k, "_", "-")
		if vocab[hv] {
			if err := claim(hv, k); err != nil {
				return err
			}
			// Normalize the surviving spelling to the underscore form the tool
			// bodies use, so exactly one key reaches the flag view.
			if target := strings.ReplaceAll(hv, "-", "_"); target != k {
				if _, taken := input[target]; !taken {
					input[target] = input[k]
					delete(input, k)
					canonical[hv] = target
				}
			}
			continue
		}
		if kebab := camelToKebab(k); kebab != "" && vocab[kebab] {
			if err := claim(kebab, k); err != nil {
				return err
			}
			target := strings.ReplaceAll(kebab, "-", "_")
			if _, taken := input[target]; taken {
				return fmt.Errorf("%s got both %q and %q — keep %q and drop the other", sc, k, target, target) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
			}
			if _, taken := input[kebab]; taken && kebab != target {
				return fmt.Errorf("%s got both %q and %q — keep %q and drop the other", sc, k, kebab, kebab) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
			}
			input[target] = input[k]
			delete(input, k)
			canonical[kebab] = target
			continue
		}
		if target, ok := aliases[strings.ToLower(hv)]; ok && vocab[target] {
			if err := claim(target, k); err != nil {
				return err
			}
			underscored := strings.ReplaceAll(target, "-", "_")
			_, hyphenTaken := input[target]
			_, underscoreTaken := input[underscored]
			if !hyphenTaken && !underscoreTaken {
				input[target] = input[k]
				delete(input, k)
				continue
			}
			// The alias AND its target are both present. This key is recognized,
			// so it must not fall through to the generic "unknown input key"
			// below — the claim() conflict message never fires here either,
			// because keys are walked in sorted order and the alias can sort
			// before its target ("size" < "width"), so nothing has claimed the
			// logical key yet. Name both spellings and the survivor.
			taken := target
			if underscoreTaken {
				taken = underscored
			}
			if jsonEqual(input[k], input[taken]) {
				delete(input, k) // same value under two names: drop the alias.
				// Hand the logical key over to the surviving spelling, or the
				// claim recorded above would still point at the deleted alias
				// and make that spelling's own turn read as a conflict.
				canonical[target] = taken
				continue
			}
			return fmt.Errorf("%s got both %q and %q, which are two names for the same flag, with different values — keep %q", sc, k, taken, taken) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
		}
		if strings.ToLower(hv) == "ranges" && vocab["range"] && !vocab["ranges"] {
			if _, taken := input["range"]; taken {
				return fmt.Errorf("%s got both %q and \"range\" — keep \"range\" and drop %q", sc, k, k) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
			}
			if arr, isArr := input[k].([]interface{}); isArr {
				if len(arr) == 1 {
					if s, isStr := arr[0].(string); isStr {
						input["range"] = s
						delete(input, k)
						continue
					}
				}
				return fmt.Errorf("%s takes a single \"range\" per sub-op, got %d entries in %q — split them into %d sub-ops (one per range)", sc, len(arr), k, len(arr)) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
			}
			if s, isStr := input[k].(string); isStr {
				input["range"] = s
				delete(input, k)
				continue
			}
		}
		msg := fmt.Sprintf("unknown input key %q", k)
		display := make([]string, 0, len(vocab))
		for name := range vocab {
			display = append(display, strings.ReplaceAll(name, "-", "_"))
		}
		sort.Strings(display)
		if match := suggest.Closest(strings.ToLower(hv), display, 1); len(match) > 0 {
			msg += fmt.Sprintf(" — did you mean %q?", match[0])
		}
		return fmt.Errorf("%s", msg) //nolint:forbidigo // intermediate error; the batch dispatcher wraps it into a typed operations validation error
	}
	return nil
}

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
//   - input 顶层出现 cell_styles / cell_merges / styles（误贴 MCP body 包裹结构）
//   - 子操作的 translator 报错（如缺必填字段）
func translateBatchOp(raw interface{}, token string, index int) (map[string]interface{}, error) {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return nil, sheetsValidationForFlag("operations", "operations[%d] must be a JSON object", index)
	}
	scRaw, present := op["shortcut"]
	if !present {
		return nil, sheetsValidationForFlag("operations", "operations[%d]: 'shortcut' field is required", index).
			WithHint(`each entry must look like {"shortcut":"+cells-set","input":{"sheet_name":"…","range":"A1:B2","cells":[[…]]}} — input uses the shortcut's own flag names`)
	}
	sc, ok := scRaw.(string)
	if !ok || sc == "" {
		return nil, sheetsValidationForFlag("operations", "operations[%d]: 'shortcut' must be a non-empty string (got %T)", index, scRaw)
	}
	mapping, ok := batchOpDispatch[sc]
	if !ok {
		// Inline the full allow-list: an agent that guessed a read op or a
		// fan-out wrapper can pick the right shortcut immediately instead of
		// spending a --print-schema round trip on the operations enum.
		return nil, sheetsValidationForFlag(
			"operations",
			"operations[%d]: shortcut %q not allowed in +batch-update "+
				"(read ops / fan-out wrappers like +batch-update / +styles-put / +cells-batch-set-style / +cells-batch-clear / +dropdown-{update,delete} are excluded)",
			index, sc,
		).WithHint("allowed shortcuts: %s", strings.Join(allowedBatchShortcuts(), ", "))
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
	// 连字符 / 下划线两种写法都算命中（spreadsheet-token 与 spreadsheet_token 同罪）。
	for userKey := range input {
		normalized := strings.ReplaceAll(userKey, "-", "_")
		for _, k := range reservedSubOpKeys {
			if normalized == k {
				return nil, sheetsValidationForFlag(
					"operations",
					"operations[%d] (%s): do not pass input.%s — it is already set from +batch-update top-level --url / --token",
					index, sc, userKey,
				)
			}
		}
	}
	// Reject a "wrapped structure" sub-op input: agents copy a shortcut's nested
	// output container (e.g. +workbook-create --styles' {"cell_styles":{…}}) into
	// the op input, but the op input is the shortcut's own flags flattened into
	// JSON keys, not that wrapper. Left unflagged this surfaces far downstream as
	// an unrelated "at least one style flag is required" (helpers.go), which never
	// points at the real mistake.
	for _, k := range wrappedSubOpInputKeys {
		if _, has := input[k]; has {
			return nil, sheetsValidationForFlag(
				"operations",
				`operations[%d] (%s): op input is the shortcut's flags flattened as JSON keys (e.g. "background_color": "#EBF1F8"); do not wrap in %s`,
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
	// Reject / rewrite off-vocabulary input keys BEFORE any value reads: an
	// unknown key silently ignored surfaces later as a misleading
	// "missing required flag" error (the top batch error cluster in evals).
	if err := normalizeSubOpInputKeys(sc, input); err != nil {
		verr := sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
		if contract := subOpInputContract(sc); contract != "" {
			verr = verr.WithHint("%s input keys: %s", sc, contract)
		}
		return nil, verr
	}
	fv := newMapFlagViewForCommand(sc, input)
	// operations is skipped by parse-time schema validation, so type-check the
	// sub-op's scalar fields here before the translator reads them via
	// Int/Bool/Float64 (which would otherwise coerce a wrong type to zero).
	if err := fv.validateRawTypes(); err != nil {
		return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
	}
	if err := fv.normalizeAndValidateEnums(); err != nil {
		return nil, sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
	}
	// Fill the selector from a "Sheet1!A1:D20" range before it is read below.
	fv.normalizeRangeSheetPrefix()
	sheetIDFlag, sheetNameFlag := sheetSelectorFlagsForSubOp(sc)
	sheetID := strings.TrimSpace(fv.Str(sheetIDFlag))
	sheetName := strings.TrimSpace(fv.Str(sheetNameFlag))
	body, err := mapping.translate(fv, token, sheetID, sheetName)
	if err != nil {
		// The inner error names one problem at a time (first missing flag);
		// the hint lists the sub-op's complete key contract so an agent fixes
		// every gap in a single retry instead of iterating flag by flag.
		verr := sheetsValidationForFlag("operations", "operations[%d] (%s): %v", index, sc, err)
		if contract := subOpInputContract(sc); contract != "" {
			verr = verr.WithHint("%s input keys: %s", sc, contract)
		}
		return nil, verr
	}
	return map[string]interface{}{
		"tool_name": mapping.mcpToolName,
		"input":     body,
	}, nil
}

// maxBatchOperations caps how many sub-operations a single +batch-update may
// carry. Every translated op (with its own cells/properties payload) is held in
// the out slice at once before the whole batch is marshaled, so an unbounded
// operation count is the same unbounded-materialization hazard as the fan-out
// matrix, on the operations axis.
const maxBatchOperations = 100

// batchOpErrorDisplayLimit bounds how many per-op validation failures ride
// on one aggregated --operations error, mirroring the schema validator's
// display cap.
const batchOpErrorDisplayLimit = 5

// translateBatchOperations 翻译整个 ops 数组。逐 op 校验并**收集全部失败**
// 一次性返回（不再 fail-fast）——agent 一轮就能修完所有坏 op，而不是
// 修一个、重试、再撞下一个。cell 安全上限仍是全局判定，命中即返回。
func translateBatchOperations(rawOps []interface{}, token string) ([]interface{}, error) {
	if len(rawOps) == 0 {
		return nil, sheetsValidationForFlag("operations", "--operations must be a non-empty JSON array")
	}
	if len(rawOps) > maxBatchOperations {
		batches := (len(rawOps) + maxBatchOperations - 1) / maxBatchOperations
		return nil, sheetsValidationForFlag("operations", "--operations accepts at most %d entries; got %d", maxBatchOperations, len(rawOps)).
			WithHint("split the operations into %d separate +batch-update calls of at most %d entries each", batches, maxBatchOperations)
	}
	// Preflight the cell footprint before any translator can materialize a
	// matrix. Translators intentionally aggregate validation errors, so a bad
	// early op must not let later valid +cells-set* ops allocate their payloads
	// outside the batch-wide safety budget before being discarded.
	var estimatedCells int64
	var budgetErr error
	for _, raw := range rawOps {
		estimatedCells += estimatedBatchOpCells(raw)
		if estimatedCells > maxStampMatrixCells && budgetErr == nil {
			budgetErr = sheetsValidationForFlag("operations",
				"--operations materialize %d cells total, over the %d-cell safety cap; reduce the number or size of cell operations",
				estimatedCells, maxStampMatrixCells)
		}
	}
	if budgetErr != nil {
		return nil, budgetErr
	}
	out := make([]interface{}, 0, len(rawOps))
	var totalCells int64
	var opErrs []error
	for i, raw := range rawOps {
		translated, err := translateBatchOp(raw, token, i)
		if err != nil {
			opErrs = append(opErrs, err)
			continue
		}
		if len(opErrs) > 0 {
			continue // already failing — keep scanning for more bad ops, skip cell math.
		}
		totalCells += translatedCellCount(translated)
		if totalCells > maxStampMatrixCells {
			return nil, sheetsValidationForFlag("operations",
				"--operations materialize %d cells total, over the %d-cell safety cap; reduce the number or size of cell operations",
				totalCells, maxStampMatrixCells)
		}
		out = append(out, translated)
	}
	switch len(opErrs) {
	case 0:
		return out, nil
	case 1:
		return nil, opErrs[0] // single failure keeps the historical error byte-for-byte.
	}
	shown := opErrs
	truncated := false
	if len(shown) > batchOpErrorDisplayLimit {
		shown = shown[:batchOpErrorDisplayLimit]
		truncated = true
	}
	parts := make([]string, 0, len(shown))
	for i, e := range shown {
		// aggregatedIssueText keeps each op's own hint (the "<shortcut> input
		// keys: …" contract) inline: folding N errors leaves one Hint slot, so
		// without this the multi-op error would carry LESS guidance than the
		// single-op one it replaces.
		parts = append(parts, fmt.Sprintf("%d) %s", i+1, aggregatedIssueText(e)))
	}
	msg := fmt.Sprintf("%d of %d operations failed validation: %s", len(opErrs), len(rawOps), strings.Join(parts, "; "))
	if truncated {
		msg += fmt.Sprintf("; (%d more not shown — fix these first)", len(opErrs)-batchOpErrorDisplayLimit)
	}
	return nil, sheetsValidationForFlag("operations", "%s", msg).WithCause(opErrs[0])
}

// estimatedBatchOpCells returns the cell footprint without invoking a
// translator. It is deliberately best-effort: malformed inputs return zero and
// are still reported by the normal translator, while well-shaped cell payloads
// are budgeted before any matrix materialization can occur.
func estimatedBatchOpCells(raw interface{}) int64 {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return 0
	}
	shortcut, _ := op["shortcut"].(string)
	input, _ := op["input"].(map[string]interface{})
	if input == nil {
		return 0
	}
	lookup := func(name string) interface{} {
		if v, ok := input[name]; ok {
			return v
		}
		if v, ok := input[strings.ReplaceAll(name, "-", "_")]; ok {
			return v
		}
		if v, ok := input[strings.ReplaceAll(name, "_", "-")]; ok {
			return v
		}
		return nil
	}
	if shortcut == "+cells-set" {
		// The payload is still in whatever shape the caller sent: the
		// translator's normalizers run later, so counting the raw value alone
		// would score a {"cells": …} envelope, a lone cell object or a payload
		// spelled "values" as zero and let it materialize outside the budget.
		// Shape-only unwrapping, no mutation — the per-cell rewrites are the
		// translator's to apply and change no count.
		payload := lookup("cells")
		if payload == nil {
			payload = lookup("values")
		}
		cells, ok := wrapLoneCellObject(unwrapCellsEnvelope(payload)).([]interface{})
		if !ok {
			return 0
		}
		var total int64
		for _, row := range cells {
			r, ok := row.([]interface{})
			if !ok || int64(len(r)) > maxStampMatrixCells-total {
				return maxStampMatrixCells + 1
			}
			total += int64(len(r))
		}
		return total
	}
	if shortcut != "+cells-set-style" && shortcut != "+dropdown-set" {
		return 0
	}
	rangeStr, ok := lookup("range").(string)
	if !ok {
		return 0
	}
	rows, cols, err := rangeDimensions(rangeStr)
	if err != nil || rows <= 0 || cols <= 0 {
		return 0
	}
	return int64(rows) * int64(cols)
}

func translatedCellCount(op map[string]interface{}) int64 {
	input, _ := op["input"].(map[string]interface{})
	switch cells := input["cells"].(type) {
	case [][]interface{}:
		var total int64
		for _, row := range cells {
			total += int64(len(row))
		}
		return total
	case []interface{}:
		var total int64
		for _, rawRow := range cells {
			if row, ok := rawRow.([]interface{}); ok {
				total += int64(len(row))
			}
		}
		return total
	default:
		return 0
	}
}
