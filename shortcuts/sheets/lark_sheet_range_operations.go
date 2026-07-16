// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── lark_sheet_range_operations ──────────────────────────────────────
//
// Four tools, nine shortcuts:
//
//   - clear_cell_range  → +cells-clear              (high-risk-write)
//   - merge_cells       → +cells-merge / +cells-unmerge
//   - resize_range      → +rows-resize / +cols-resize
//   - transform_range   → +range-move / +range-copy / +range-fill / +range-sort
//
// +rows-resize / +cols-resize are grouped under "工作表" for CLI discoverability
// even though the backing tool lives in this skill.

// CellsClear wraps clear_cell_range.
//
// CLI's --scope vocabulary (content / formats / all) is normalized to the
// tool's clear_type vocabulary (contents / formats / all) — the spec's
// singular/plural mismatch is intentionally absorbed here.
var CellsClear = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-clear",
	Description: "Clear cell content, formats, or both within a range (irreversible).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cells-clear"),
	Validate:    validateViaInput(cellsClearInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := cellsClearInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "clear_cell_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := cellsClearInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "clear_cell_range", input)
		if err != nil {
			return annotateEmbeddedBlockClearErr(err)
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"high-risk-write — always preview with --dry-run; clear is not undoable.",
		"Can't delete an embedded pivot/chart by clearing cells — remove the object itself with +pivot-delete / +chart-delete.",
	},
}

func cellsClearInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"range":      strings.TrimSpace(runtime.Str("range")),
		"clear_type": normalizeClearType(runtime.Str("scope")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// normalizeClearType maps the CLI --scope vocabulary (content / formats / all)
// to the clear_cell_range tool's clear_type vocabulary (contents / formats /
// all). The content↔contents singular/plural mismatch is absorbed here so both
// +cells-clear and the +cells-batch-clear fan-out stay in lockstep.
func normalizeClearType(scope string) string {
	switch scope {
	case "formats", "all":
		return scope
	default: // "content" or unset
		return "contents"
	}
}

// annotateEmbeddedBlockClearErr augments the backend's "embedded block" clear
// failure with the concrete fix. clear_cell_range only clears cell values /
// formats — it cannot delete an embedded object (pivot table / chart) that
// overlaps the range, which is what the backend's "can not find embedded block"
// actually means. Trajectories burned dozens of commands trying to recover a
// pivot-occupied A1 with cells-clear; point the agent at the object's own
// delete command instead. Non-matching errors pass through untouched.
func annotateEmbeddedBlockClearErr(err error) error {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	if !strings.Contains(strings.ToLower(p.Message), "embedded block") {
		return err
	}
	const hint = "the range overlaps an embedded object (pivot table / chart); " +
		"cells-clear only clears cell values/formats and cannot delete it — " +
		"delete the object with its own command (+pivot-delete / +chart-delete; find the id via +pivot-list / +chart-list)"
	if p.Hint == "" {
		p.Hint = hint
	} else {
		p.Hint += "; " + hint
	}
	return err
}

// CellsMerge / CellsUnmerge share the merge_cells tool, dispatched by the
// `operation` enum. --merge-type applies to merge only and maps to tool
// field merge_type (`all` / `rows` / `columns`).
var CellsMerge = newMergeShortcut(
	"+cells-merge", "Merge cells in a range.", "merge", true,
)
var CellsUnmerge = newMergeShortcut(
	"+cells-unmerge", "Unmerge cells in a range.", "unmerge", false,
)

func newMergeShortcut(command, desc, op string, withMergeType bool) common.Shortcut {
	flags := flagsFor(command)
	return common.Shortcut{
		Service:     "sheets",
		Command:     command,
		Description: desc,
		Risk:        "write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flags,
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = mergeInput(runtime, token, sheetID, sheetName, op, withMergeType)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := mergeInput(runtime, token, sheetID, sheetName, op, withMergeType)
			return invokeToolDryRun(token, ToolKindWrite, "merge_cells", input)
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetTokenExec(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			input, err := mergeInput(runtime, token, sheetID, sheetName, op, withMergeType)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "merge_cells", input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func mergeInput(runtime flagView, token, sheetID, sheetName, op string, withMergeType bool) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"range":     strings.TrimSpace(runtime.Str("range")),
		"operation": op,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if withMergeType {
		if mt := runtime.Str("merge-type"); mt != "" && mt != "all" {
			input["merge_type"] = mt
		} else {
			input["merge_type"] = "all"
		}
	}
	return input, nil
}

// resize_range exposes two CLI shortcuts, each with two input forms:
//
//   +rows-resize / +cols-resize — set row heights / column widths.
//
//   Uniform form: --range + --height/--width <px>; the pixel mode is implied
//   so --type can be omitted (or set to `pixel` — equivalent). Non-pixel
//   modes go through --type standard / --type auto (rows only) and cannot be
//   combined with the pixel flag. --range is an A1 closed range ("2:10" /
//   "5" rows or "A:E" / "C" columns); single-element form is expanded to
//   "N:N" before send because resize_range rejects bare single-element
//   ranges.
//
//   Map form: --heights / --widths carries a JSON object of per-row/column
//   sizes ({"A": 100, "C:E": 120, "G": "standard"}) and fans out into one
//   atomic batch_update of resize_range ops — different sizes for many
//   rows/columns in a single CLI call, no +batch-update needed. Mutually
//   exclusive with --range/--height/--width/--type, and not accepted as a
//   +batch-update sub-op (nested batch_update is unsupported upstream).
//
// Wire shape: resize_height / resize_width carries { type, value? }, e.g.
//   { "type": "pixel", "value": 30 }  or  { "type": "standard" }.
//
// Units are pixels. Column widths in Excel character units (openpyxl /
// xlsxwriter mental model, px ≈ chars × 8 + 16) are a real agent trap, so
// widths below minSaneColumnWidthPx are rejected with a conversion hint.

// RowsResize wraps resize_range for row heights. Pass --range + --height
// <px> for a uniform pixel height, --heights '{"1":50,"2:20":30}' for
// per-row heights, or --type standard/auto for non-pixel modes.
var RowsResize = common.Shortcut{
	Service:     "sheets",
	Command:     "+rows-resize",
	Description: "Resize rows in pixels: --range + --height <px> for one uniform height, --heights '{\"1\":50,\"2:20\":30,\"21\":\"auto\"}' for per-row heights in one atomic call, or --type standard/auto (--range is 1-based A1 like \"2:10\" or \"5\").",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+rows-resize"),
	Validate:    validateViaResize("row"),
	DryRun:      resizeDryRun("row"),
	Execute:     resizeExecute("row"),
}

// ColsResize wraps resize_range for column widths. Pass --range + --width
// <px> for a uniform pixel width, --widths '{"A":100,"C:E":120}' for
// per-column widths, or --type standard for the default width. Column
// widths do not support auto-fit — --type does not accept auto.
var ColsResize = common.Shortcut{
	Service:     "sheets",
	Command:     "+cols-resize",
	Description: "Resize columns in pixels (NOT Excel char units): --range + --width <px> for one uniform width, --widths '{\"A\":100,\"C:E\":120}' for per-column widths in one atomic call, or --type standard to reset (--range is column letters like \"A:E\" or \"C\"; no auto for cols).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+cols-resize"),
	Validate:    validateViaResize("column"),
	DryRun:      resizeDryRun("column"),
	Execute:     resizeExecute("column"),
}

// resizeDryRun / resizeExecute route a resize shortcut through resizeToolCall
// so the uniform form hits resize_range and the map form hits batch_update
// with identical inputs in preview and execution.
func resizeDryRun(dimension string) func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	return func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		toolName, input, _ := resizeToolCall(runtime, token, sheetID, sheetName, dimension)
		return invokeToolDryRun(token, ToolKindWrite, toolName, input)
	}
}

func resizeExecute(dimension string) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		toolName, input, err := resizeToolCall(runtime, token, sheetID, sheetName, dimension)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, toolName, input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	}
}

// validateViaResize wires the standalone Validate to resizeToolCall so both
// forms (uniform + map) are fully validated before execution.
func validateViaResize(dimension string) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, _, err = resizeToolCall(runtime, token, sheetID, sheetName, dimension)
		return err
	}
}

// resizeToolCall picks the input form: map form (--heights/--widths) builds a
// batch_update of resize_range ops; uniform form builds a single resize_range
// input. Returns the tool name to invoke alongside its input.
func resizeToolCall(runtime flagView, token, sheetID, sheetName, dimension string) (string, map[string]interface{}, error) {
	if runtime.Changed(sizeMapFlag(dimension)) {
		input, err := resizeMapInput(runtime, token, sheetID, sheetName, dimension)
		return "batch_update", input, err
	}
	input, err := resizeInput(runtime, token, sheetID, sheetName, dimension)
	return "resize_range", input, err
}

// nonPixelTypes lists the --type values a given dimension accepts (rows also
// accept auto; columns only accept standard). Used to shape the hint printed
// when --type is missing or invalid.
func nonPixelTypes(dimension string) string {
	if dimension == "row" {
		return "standard / auto"
	}
	return "standard"
}

// pixelFlag maps a dimension to its pixel-value flag name (--height for rows,
// --width for cols). The wire block always emits "pixel" as the mode; the
// per-dimension flag name is just the surface knob.
func pixelFlag(dimension string) string {
	if dimension == "row" {
		return "height"
	}
	return "width"
}

// sizeMapFlag maps a dimension to its map-form flag name (--heights for rows,
// --widths for cols).
func sizeMapFlag(dimension string) string {
	return pixelFlag(dimension) + "s"
}

// rejectResizeMapInBatch blocks the map form inside +batch-update sub-ops:
// it expands into its own batch_update and nesting batch_update is
// unsupported upstream. Called by the batch dispatch closures only — the
// standalone path routes the map form through resizeMapInput instead.
func rejectResizeMapInBatch(fv flagView, dimension string) error {
	mapFlag := sizeMapFlag(dimension)
	if !fv.Changed(mapFlag) {
		return nil
	}
	return sheetsValidationForFlag(mapFlag,
		"%q is not supported inside +batch-update (it expands into its own atomic batch); call %s --%s standalone, or give each sub-op the single-range form (range + %s/type)",
		mapFlag, commandForDimension(dimension), mapFlag, pixelFlag(dimension))
}

// minSaneColumnWidthPx is the floor below which a column width almost
// certainly means the caller thought in Excel character units (openpyxl /
// xlsxwriter widths run 8-30 chars) instead of pixels. 10px columns are
// unusable; real pixel spacer columns start around 20px.
const minSaneColumnWidthPx = 20

// checkPixelSize validates a pixel value for one dimension. label names the
// offending input in the error ("--width" for the uniform flag, "--widths
// key \"A\"" for a map entry).
func checkPixelSize(dimension, flagName, label string, px int) error {
	if px <= 0 {
		return sheetsValidationForFlag(flagName, "%s must be > 0", label)
	}
	if dimension == "column" && px < minSaneColumnWidthPx {
		return sheetsValidationForFlag(flagName,
			"%s = %dpx is below %dpx and looks like an Excel character-unit width — column widths here are pixels (px ≈ chars × 8 + 16, so %d chars ≈ %dpx)",
			label, px, minSaneColumnWidthPx, px, px*8+16)
	}
	return nil
}

// commandForDimension returns the shortcut command name a given dimension
// belongs to; used in error messages so users see "+rows-resize" / "+cols-resize"
// instead of the internal "row" / "column" tag.
func commandForDimension(dimension string) string {
	if dimension == "row" {
		return "+rows-resize"
	}
	return "+cols-resize"
}

// resizeInput builds the resize_range tool input. dimension is "row" /
// "column" (selected by the calling shortcut); --range must match that
// dimension (row → digits like "2:10" / "5"; column → letters like "A:E" /
// "C"). Single-element form is expanded to "N:N" because resize_range
// rejects bare single-element ranges.
//
// Surface: pixel size goes through --height / --width (dimension-specific).
// --type is optional when the pixel flag is present (defaults to "pixel");
// explicit --type pixel is accepted and equivalent. --type standard / auto
// select non-pixel modes and cannot be combined with the pixel flag.
func resizeInput(runtime flagView, token, sheetID, sheetName, dimension string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.Changed("range") {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	rangeStr := strings.TrimSpace(runtime.Str("range"))
	parsedDim, _, _, err := parseA1Range(rangeStr)
	if err != nil {
		return nil, sheetsValidationForFlag("range", "invalid --range %q: %v", rangeStr, err)
	}
	if parsedDim != dimension {
		want := "row numbers (e.g. \"2:10\")"
		if dimension == "column" {
			want = "column letters (e.g. \"A:E\")"
		}
		return nil, sheetsValidationForFlag("range", "--range %q is a %s range; %s expects %s", rangeStr, parsedDim, commandForDimension(dimension), want)
	}
	if !strings.Contains(rangeStr, ":") {
		rangeStr = rangeStr + ":" + rangeStr
	}

	sizeFlag := pixelFlag(dimension)
	hasSize := runtime.Changed(sizeFlag)
	typ := strings.TrimSpace(runtime.Str("type"))
	hasType := typ != ""

	if !hasSize && !hasType {
		return nil, common.ValidationErrorf("give --%s <px> for a pixel size, or --type %s", sizeFlag, nonPixelTypes(dimension)).WithParams(sheetsInvalidParam(sizeFlag, "required"), sheetsInvalidParam("type", "required"))
	}
	if hasSize && hasType && typ != "pixel" {
		return nil, common.ValidationErrorf("--%s cannot be combined with --type %s", sizeFlag, typ).WithParams(sheetsInvalidParam(sizeFlag, "mutually exclusive"), sheetsInvalidParam("type", "mutually exclusive"))
	}
	if hasType && dimension == "column" && typ == "auto" {
		return nil, sheetsValidationForFlag("type", "--type auto is rows-only (column widths do not support auto-fit); use +rows-resize")
	}
	if hasType && typ == "pixel" && !hasSize {
		return nil, common.ValidationErrorf("--type pixel requires --%s <px>", sizeFlag).WithParams(sheetsInvalidParam("type", "required"), sheetsInvalidParam(sizeFlag, "required"))
	}

	sizeBlock := map[string]interface{}{}
	if hasSize {
		px := runtime.Int(sizeFlag)
		if err := checkPixelSize(dimension, sizeFlag, "--"+sizeFlag, px); err != nil {
			return nil, err
		}
		sizeBlock["type"] = "pixel"
		sizeBlock["value"] = px
	} else {
		sizeBlock["type"] = typ
	}

	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if dimension == "row" {
		input["resize_height"] = sizeBlock
	} else {
		input["resize_width"] = sizeBlock
	}
	return input, nil
}

// resizeMapInput builds the batch_update input for the map form: every
// --heights/--widths entry becomes one resize_range op inside a single atomic
// batch. Keys are single rows/columns ("5" / "A") or closed ranges ("2:8" /
// "C:E") matching the command's dimension; values are positive pixel ints or
// the non-pixel mode strings ("standard", and "auto" for rows). Ops are
// sorted by start position so dry-run output and execution order are
// deterministic (JSON object order is not preserved by Go maps).
func resizeMapInput(runtime flagView, token, sheetID, sheetName, dimension string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	mapFlag := sizeMapFlag(dimension)
	for _, other := range []string{"range", pixelFlag(dimension), "type"} {
		if runtime.Changed(other) {
			return nil, common.ValidationErrorf("--%s is a self-contained map; do not combine it with --%s", mapFlag, other).WithParams(sheetsInvalidParam(mapFlag, "mutually exclusive"), sheetsInvalidParam(other, "mutually exclusive"))
		}
	}
	parsed, err := parseJSONFlag(runtime, mapFlag)
	if err != nil {
		return nil, err
	}
	entries, ok := parsed.(map[string]interface{})
	if !ok || parsed == nil {
		return nil, sheetsValidationForFlag(mapFlag, "--%s must be a JSON object like {\"%s\": 100}", mapFlag, exampleMapKey(dimension))
	}
	if len(entries) == 0 {
		return nil, sheetsValidationForFlag(mapFlag, "--%s must contain at least one entry", mapFlag)
	}
	if len(entries) > maxBatchOperations {
		return nil, sheetsValidationForFlag(mapFlag, "--%s accepts at most %d entries; got %d", mapFlag, maxBatchOperations, len(entries))
	}

	type resizeOp struct {
		start    int
		end      int
		rangeKey string
		input    map[string]interface{}
	}
	ops := make([]resizeOp, 0, len(entries))
	seen := make(map[string]string, len(entries)) // normalized range → original key
	for key, raw := range entries {
		parsedDim, startIdx, endIdx, err := parseA1Range(key)
		if err != nil {
			return nil, sheetsValidationForFlag(mapFlag, "--%s key %q: %v", mapFlag, key, err)
		}
		if parsedDim != dimension {
			want := "row numbers (e.g. \"2:10\")"
			if dimension == "column" {
				want = "column letters (e.g. \"A:E\")"
			}
			return nil, sheetsValidationForFlag(mapFlag, "--%s key %q is a %s range; %s expects %s", mapFlag, key, parsedDim, commandForDimension(dimension), want)
		}
		normalized := strings.TrimSpace(key)
		if !strings.Contains(normalized, ":") {
			normalized = normalized + ":" + normalized
		}
		if prev, dup := seen[normalized]; dup {
			return nil, sheetsValidationForFlag(mapFlag, "--%s keys %q and %q target the same range %s; merge them into one entry", mapFlag, prev, key, normalized)
		}
		seen[normalized] = key

		sizeBlock := map[string]interface{}{}
		switch v := raw.(type) {
		case float64:
			px := int(v)
			if float64(px) != v {
				return nil, sheetsValidationForFlag(mapFlag, "--%s[%q] must be an integer pixel value, got %v", mapFlag, key, v)
			}
			if err := checkPixelSize(dimension, mapFlag, fmt.Sprintf("--%s[%q]", mapFlag, key), px); err != nil {
				return nil, err
			}
			sizeBlock["type"] = "pixel"
			sizeBlock["value"] = px
		case string:
			mode := strings.TrimSpace(v)
			if mode == "auto" && dimension == "column" {
				return nil, sheetsValidationForFlag(mapFlag, "--%s[%q]: \"auto\" is rows-only (column widths do not support auto-fit); estimate a pixel width instead (px ≈ chars × 8 + 16)", mapFlag, key)
			}
			if mode != "standard" && !(mode == "auto" && dimension == "row") {
				return nil, sheetsValidationForFlag(mapFlag, "--%s[%q] = %q is invalid; use a pixel integer or %s", mapFlag, key, v, nonPixelTypes(dimension))
			}
			sizeBlock["type"] = mode
		default:
			return nil, sheetsValidationForFlag(mapFlag, "--%s[%q] must be a pixel integer or a mode string (%s), got %s", mapFlag, key, nonPixelTypes(dimension), jsonTypeName(raw))
		}

		opInput := map[string]interface{}{
			"excel_id": token,
			"range":    normalized,
		}
		sheetSelectorForToolInput(opInput, sheetID, sheetName)
		if dimension == "row" {
			opInput["resize_height"] = sizeBlock
		} else {
			opInput["resize_width"] = sizeBlock
		}
		ops = append(ops, resizeOp{
			start:    startIdx,
			end:      endIdx,
			rangeKey: normalized,
			input:    opInput,
		})
	}

	sort.Slice(ops, func(i, j int) bool {
		if ops[i].start != ops[j].start {
			return ops[i].start < ops[j].start
		}
		return ops[i].end < ops[j].end
	})
	for i := 1; i < len(ops); i++ {
		if ops[i].start <= ops[i-1].end {
			return nil, sheetsValidationForFlag(
				mapFlag,
				"--%s ranges %q and %q overlap; use non-overlapping ranges",
				mapFlag, ops[i-1].rangeKey, ops[i].rangeKey,
			)
		}
	}
	operations := make([]interface{}, 0, len(ops))
	for _, op := range ops {
		operations = append(operations, map[string]interface{}{
			"tool_name": "resize_range",
			"input":     op.input,
		})
	}
	return map[string]interface{}{
		"excel_id":   token,
		"operations": operations,
	}, nil
}

// exampleMapKey renders a dimension-appropriate sample key for error hints.
func exampleMapKey(dimension string) string {
	if dimension == "row" {
		return "2:10"
	}
	return "A"
}

// ─── transform_range (4 shortcuts) ────────────────────────────────────
//
// move / copy take --source-range + --target-range (+ optional cross-sheet
// target). fill takes --source-range + --target-range + --series-type. sort
// takes --range + --sort-keys + --has-header.

// RangeMove cuts data from --source-range and pastes at --target-range,
// optionally on another sheet.
var RangeMove = common.Shortcut{
	Service:     "sheets",
	Command:     "+range-move",
	Description: "Cut a range and paste it at a new location (optionally cross-sheet).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+range-move"),
	Validate:    validateRangeMoveOrCopy("move", false),
	DryRun:      transformDryRunFn("move", false, false),
	Execute:     transformExecuteFn("move", false, false),
}

// RangeCopy duplicates a range to a new location with optional paste-type
// filter (values / formulas / formats / all).
var RangeCopy = common.Shortcut{
	Service:     "sheets",
	Command:     "+range-copy",
	Description: "Copy a range to a new location (--paste-type controls what is copied).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+range-copy"),
	Validate:    validateRangeMoveOrCopy("copy", true),
	DryRun:      transformDryRunFn("copy", true, false),
	Execute:     transformExecuteFn("copy", true, false),
}

// RangeFill performs autofill from a template range into a target range.
// --series-type is a 5-value CLI vocabulary; the tool only distinguishes
// `copyCells` from `fillSeries`. The mapping is documented in
// fillSeriesToToolType.
var RangeFill = common.Shortcut{
	Service:     "sheets",
	Command:     "+range-fill",
	Description: "Autofill a target range from a source template (copy / linear / growth / date series).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+range-fill"),
	Validate:    validateViaInput(rangeFillInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := rangeFillInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := rangeFillInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "transform_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// RangeSort sorts rows within a range by one or more columns.
var RangeSort = common.Shortcut{
	Service:     "sheets",
	Command:     "+range-sort",
	Description: "Sort rows within a range by one or more columns.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+range-sort"),
	Validate:    validateViaInput(rangeSortInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := rangeSortInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := rangeSortInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "transform_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// ─── transform_range helpers ──────────────────────────────────────────

// validateRangeMoveOrCopy wires the standalone Validate to transformMoveCopyInput
// so missing --source-range / --target-range fire the same friendly error on
// the batch sub-op path.
func validateRangeMoveOrCopy(op string, withPasteType bool) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
		sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
		_, err = transformMoveCopyInput(runtime, token, sheetID, sheetName, op, withPasteType)
		return err
	}
}

func transformDryRunFn(op string, withPasteType, _ bool) func(context.Context, *common.RuntimeContext) *common.DryRunAPI {
	return func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := transformMoveCopyInput(runtime, token, sheetID, sheetName, op, withPasteType)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range", input)
	}
}

func transformExecuteFn(op string, withPasteType, _ bool) func(context.Context, *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		input, err := transformMoveCopyInput(runtime, token, sheetID, sheetName, op, withPasteType)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "transform_range", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	}
}

func transformMoveCopyInput(runtime flagView, token, sheetID, sheetName, op string, withPasteType bool) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("source-range")) == "" {
		return nil, sheetsValidationForFlag("source-range", "--source-range is required")
	}
	if strings.TrimSpace(runtime.Str("target-range")) == "" {
		return nil, sheetsValidationForFlag("target-range", "--target-range is required")
	}
	input := map[string]interface{}{
		"excel_id":          token,
		"operation":         op,
		"range":             strings.TrimSpace(runtime.Str("source-range")),
		"destination_range": strings.TrimSpace(runtime.Str("target-range")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if tgt := strings.TrimSpace(runtime.Str("target-sheet-id")); tgt != "" {
		input["destination_sheet_id"] = tgt
	}
	if withPasteType {
		if pt := runtime.Str("paste-type"); pt != "" && pt != "all" {
			input["paste_type"] = pasteTypeToTool(pt)
		}
	}
	return input, nil
}

// pasteTypeToTool maps the CLI vocabulary (values / formulas / formats / all)
// to the tool's paste_type field (all / value_only / formula_only / format_only).
func pasteTypeToTool(pt string) string {
	switch pt {
	case "values":
		return "value_only"
	case "formulas":
		return "formula_only"
	case "formats":
		return "format_only"
	}
	return "all"
}

func rangeFillInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("source-range")) == "" {
		return nil, sheetsValidationForFlag("source-range", "--source-range is required")
	}
	if strings.TrimSpace(runtime.Str("target-range")) == "" {
		return nil, sheetsValidationForFlag("target-range", "--target-range is required")
	}
	input := map[string]interface{}{
		"excel_id":          token,
		"operation":         "fill",
		"range":             strings.TrimSpace(runtime.Str("source-range")),
		"destination_range": strings.TrimSpace(runtime.Str("target-range")),
		"fill_type":         fillSeriesToToolType(runtime.Str("series-type")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// fillSeriesToToolType maps the CLI series vocabulary to the tool's fill_type.
// The tool only distinguishes copy vs series; the CLI's series flavor (linear /
// growth / date / auto) all collapse to fillSeries — the actual progression is
// inferred by the server from the source cells.
func fillSeriesToToolType(seriesType string) string {
	if seriesType == "copy" {
		return "copyCells"
	}
	return "fillSeries"
}

func rangeSortInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("range")) == "" {
		return nil, sheetsValidationForFlag("range", "--range is required")
	}
	// requireJSONArray runs the embedded JSON Schema for --sort-keys
	// via parseJSONFlag → validateParsedJSONFlag, so each item is
	// already pinned to {column: string, ascending: bool} with the
	// failing index reported. No per-item hand-written guard needed.
	keys, err := requireJSONArray(runtime, "sort-keys")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":        token,
		"operation":       "sort",
		"range":           strings.TrimSpace(runtime.Str("range")),
		"sort_conditions": keys,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if runtime.Bool("has-header") {
		input["has_header"] = true
	}
	return input, nil
}
