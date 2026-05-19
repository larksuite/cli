// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"

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
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "A1 range to clear (e.g. A1:C10 / D3:D / 3:3)"},
		common.Flag{Name: "scope", Enum: []string{"content", "formats", "all"}, Default: "content",
			Desc: "what to clear: content (values+formulas only, default) / formats / all"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return common.FlagErrorf("--range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "clear_cell_range", cellsClearInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "clear_cell_range", cellsClearInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"high-risk-write — always preview with --dry-run; clear is not undoable.",
	},
}

func cellsClearInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	scope := runtime.Str("scope")
	clearType := "contents"
	switch scope {
	case "content", "":
		clearType = "contents"
	case "formats", "all":
		clearType = scope
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"range":      strings.TrimSpace(runtime.Str("range")),
		"clear_type": clearType,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input
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
	flags := append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "A1 range to merge / unmerge (e.g. A1:C3)"},
	)
	if withMergeType {
		flags = append(flags, common.Flag{
			Name: "merge-type", Enum: []string{"all", "rows", "columns"}, Default: "all",
			Desc: "merge strategy: all (one cell) / rows / columns",
		})
	}
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
			if _, err := resolveSpreadsheetToken(runtime); err != nil {
				return err
			}
			if _, _, err := resolveSheetSelector(runtime); err != nil {
				return err
			}
			if strings.TrimSpace(runtime.Str("range")) == "" {
				return common.FlagErrorf("--range is required")
			}
			return nil
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			return invokeToolDryRun(token, ToolKindWrite, "merge_cells", mergeInput(runtime, token, sheetID, sheetName, op, withMergeType))
		},
		Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID, sheetName, err := resolveSheetSelector(runtime)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "merge_cells", mergeInput(runtime, token, sheetID, sheetName, op, withMergeType))
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

func mergeInput(runtime *common.RuntimeContext, token, sheetID, sheetName, op string, withMergeType bool) map[string]interface{} {
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
	return input
}

// resize_range now exposes two CLI shortcuts:
//
//   +rows-resize / +cols-resize — set row heights / column widths. The new
//   --type enum (pixel / standard / [auto]) replaces the old --size/--reset
//   pair; --type pixel still takes a --size pixel value, --type standard
//   restores the sheet default, --type auto auto-fits row heights (rows only).
//
// Wire shape: resize_height / resize_width carries { type, value? }, e.g.
//   { "type": "pixel", "value": 30 }  or  { "type": "standard" }.
//
// Both shortcuts share the underlying resize_range tool; --end is inclusive
// in the new CLI surface (was exclusive in the legacy +dim-resize).

var rowsResizeTypeEnum = []string{"pixel", "standard", "auto"}
var colsResizeTypeEnum = []string{"pixel", "standard"}

// RowsResize wraps resize_range for row heights. --type auto enables
// auto-fit (rows only); --type pixel requires --size.
var RowsResize = common.Shortcut{
	Service:     "sheets",
	Command:     "+rows-resize",
	Description: "Resize rows by pixel / standard / auto (--type pixel needs --size; --start/--end are 0-based inclusive).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "start", Type: "int", Required: true, Desc: "0-based start row (inclusive)"},
		common.Flag{Name: "end", Type: "int", Required: true, Desc: "0-based end row (inclusive)"},
		common.Flag{Name: "type", Required: true, Enum: rowsResizeTypeEnum,
			Desc: "sizing mode: `pixel` (needs --size) / `standard` (reset to default) / `auto` (auto-fit row height)"},
		common.Flag{Name: "size", Type: "int", Default: "0", Desc: "row height in pixels (e.g. 30); required with --type pixel, ignored otherwise"},
	),
	Validate: validateResize("row"),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "resize_range", resizeInput(runtime, token, sheetID, sheetName, "row"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "resize_range", resizeInput(runtime, token, sheetID, sheetName, "row"))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// ColsResize wraps resize_range for column widths. Column widths do not
// support auto-fit — --type only accepts pixel / standard.
var ColsResize = common.Shortcut{
	Service:     "sheets",
	Command:     "+cols-resize",
	Description: "Resize columns by pixel / standard (--type pixel needs --size; --start/--end are 0-based inclusive; no auto for cols).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "start", Type: "int", Required: true, Desc: "0-based start column (inclusive)"},
		common.Flag{Name: "end", Type: "int", Required: true, Desc: "0-based end column (inclusive)"},
		common.Flag{Name: "type", Required: true, Enum: colsResizeTypeEnum,
			Desc: "sizing mode: `pixel` (needs --size) / `standard` (reset to default); `auto` is rows-only"},
		common.Flag{Name: "size", Type: "int", Default: "0", Desc: "column width in pixels (e.g. 120); required with --type pixel, ignored otherwise"},
	),
	Validate: validateResize("column"),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "resize_range", resizeInput(runtime, token, sheetID, sheetName, "column"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "resize_range", resizeInput(runtime, token, sheetID, sheetName, "column"))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// validateResize returns a Validate closure shared by both rows/cols shortcuts.
// dimension is either "row" or "column"; the closure rejects --type auto on
// columns (column widths do not support auto-fit).
func validateResize(dimension string) func(ctx context.Context, runtime *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if !runtime.Changed("start") || !runtime.Changed("end") {
			return common.FlagErrorf("--start and --end are required")
		}
		if runtime.Int("start") < 0 || runtime.Int("end") < runtime.Int("start") {
			return common.FlagErrorf("invalid range: --start (%d) must be >= 0 and --end (%d) must be >= --start", runtime.Int("start"), runtime.Int("end"))
		}
		typ := strings.TrimSpace(runtime.Str("type"))
		if typ == "" {
			return common.FlagErrorf("--type is required (pixel / standard%s)", autoSuffix(dimension))
		}
		if dimension == "column" && typ == "auto" {
			return common.FlagErrorf("--type auto is rows-only (column widths do not support auto-fit); use +rows-resize")
		}
		hasSize := runtime.Changed("size") && runtime.Int("size") > 0
		if typ == "pixel" && !hasSize {
			return common.FlagErrorf("--type pixel requires --size <px>")
		}
		if typ != "pixel" && hasSize {
			return common.FlagErrorf("--size is only valid with --type pixel")
		}
		return nil
	}
}

// autoSuffix appends " / auto" to the enum hint for rows.
func autoSuffix(dimension string) string {
	if dimension == "row" {
		return " / auto"
	}
	return ""
}

// resizeInput builds the resize_range tool input. dimension is "row" /
// "column"; --end is inclusive on the CLI surface, dimRange wants
// exclusive end, so it is bumped by one here.
func resizeInput(runtime *common.RuntimeContext, token, sheetID, sheetName, dimension string) map[string]interface{} {
	rangeStr := dimRange(dimension, runtime.Int("start"), runtime.Int("end")+1)
	input := map[string]interface{}{
		"excel_id": token,
		"range":    rangeStr,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	typ := strings.TrimSpace(runtime.Str("type"))
	sizeBlock := map[string]interface{}{"type": typ}
	if typ == "pixel" {
		sizeBlock["value"] = runtime.Int("size")
	}
	if dimension == "row" {
		input["resize_height"] = sizeBlock
	} else {
		input["resize_width"] = sizeBlock
	}
	return input
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
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "source-range", Required: true, Desc: "source A1 range (e.g. A1:C5)"},
		common.Flag{Name: "target-range", Required: true, Desc: "target A1 starting cell (size derived from source)"},
		common.Flag{Name: "target-sheet-id", Desc: "destination sheet id (cross-sheet); omit for same sheet"},
	),
	Validate:    validateRangeMoveOrCopy,
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
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "source-range", Required: true, Desc: "source A1 range"},
		common.Flag{Name: "target-range", Required: true, Desc: "target A1 starting cell"},
		common.Flag{Name: "target-sheet-id", Desc: "destination sheet id (cross-sheet); omit for same sheet"},
		common.Flag{Name: "paste-type", Enum: []string{"values", "formulas", "formats", "all"}, Default: "all",
			Desc: "what to copy: values / formulas / formats / all (default)"},
	),
	Validate:    validateRangeMoveOrCopy,
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
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "source-range", Required: true, Desc: "template A1 range with seed cells"},
		common.Flag{Name: "target-range", Required: true, Desc: "target fill range (must be disjoint from source)"},
		common.Flag{Name: "series-type", Enum: []string{"auto", "linear", "growth", "date", "copy"}, Default: "auto",
			Desc: "auto / linear / growth / date → tool fillSeries; copy → tool copyCells"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("source-range")) == "" {
			return common.FlagErrorf("--source-range is required")
		}
		if strings.TrimSpace(runtime.Str("target-range")) == "" {
			return common.FlagErrorf("--target-range is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range", rangeFillInput(runtime, token, sheetID, sheetName))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "transform_range", rangeFillInput(runtime, token, sheetID, sheetName))
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
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "A1 range to sort"},
		common.Flag{Name: "sort-keys", Input: []string{common.File, common.Stdin}, Required: true,
			Desc: "sort keys JSON, e.g. [{\"col\":\"B\",\"order\":\"asc\"},{\"col\":\"D\",\"order\":\"desc\"}]"},
		common.Flag{Name: "has-header", Type: "bool", Desc: "treat first row as header (excluded from sort); default false"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("range")) == "" {
			return common.FlagErrorf("--range is required")
		}
		if _, err := requireJSONArray(runtime, "sort-keys"); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := rangeSortInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
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

func validateRangeMoveOrCopy(ctx context.Context, runtime *common.RuntimeContext) error {
	if _, err := resolveSpreadsheetToken(runtime); err != nil {
		return err
	}
	if _, _, err := resolveSheetSelector(runtime); err != nil {
		return err
	}
	if strings.TrimSpace(runtime.Str("source-range")) == "" {
		return common.FlagErrorf("--source-range is required")
	}
	if strings.TrimSpace(runtime.Str("target-range")) == "" {
		return common.FlagErrorf("--target-range is required")
	}
	return nil
}

func transformDryRunFn(op string, withPasteType, _ bool) func(context.Context, *common.RuntimeContext) *common.DryRunAPI {
	return func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "transform_range",
			transformMoveCopyInput(runtime, token, sheetID, sheetName, op, withPasteType))
	}
}

func transformExecuteFn(op string, withPasteType, _ bool) func(context.Context, *common.RuntimeContext) error {
	return func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "transform_range",
			transformMoveCopyInput(runtime, token, sheetID, sheetName, op, withPasteType))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	}
}

func transformMoveCopyInput(runtime *common.RuntimeContext, token, sheetID, sheetName, op string, withPasteType bool) map[string]interface{} {
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
	return input
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

func rangeFillInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{
		"excel_id":          token,
		"operation":         "fill",
		"range":             strings.TrimSpace(runtime.Str("source-range")),
		"destination_range": strings.TrimSpace(runtime.Str("target-range")),
		"fill_type":         fillSeriesToToolType(runtime.Str("series-type")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input
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

func rangeSortInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) (map[string]interface{}, error) {
	keys, err := requireJSONArray(runtime, "sort-keys")
	if err != nil {
		return nil, err
	}
	input := map[string]interface{}{
		"excel_id":         token,
		"operation":        "sort",
		"range":            strings.TrimSpace(runtime.Str("range")),
		"sort_conditions":  keys,
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if runtime.Bool("has-header") {
		input["has_header"] = true
	}
	return input, nil
}
