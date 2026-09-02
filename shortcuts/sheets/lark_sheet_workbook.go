// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/suggest"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/drive"
)

// ─── lark_sheet_workbook ──────────────────────────────────────────────
//
// Wraps two tools behind the One-OpenAPI: get_workbook_structure (read) and
// modify_workbook_structure (write, dispatched by `operation` enum).
//
// CLI Risk tiers diverge intentionally from the tool's single endpoint:
//   - +sheet-delete  is high-risk-write (irreversible)
//   - everything else is plain write
//
// +sheet-create only carries --url / --spreadsheet-token (no sheet selector):
// the create tool path needs no existing-sheet anchor, so the public sheet
// selector pair is dropped here to avoid a misleading XOR requirement.

// WorkbookInfo wraps get_workbook_structure: list a workbook's sub-sheets
// with their metadata (sheet_id, title, dimensions, freeze rows and cols,
// index, hidden). First step for every sheets task — downstream sheet-level
// operations all depend on the sheet_id returned here.
var WorkbookInfo = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-info",
	Description: "List sub-sheets of a spreadsheet with metadata (sheet_id, title, dimensions, freeze, hidden).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+workbook-info"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := resolveSpreadsheetToken(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
			"excel_id": token,
		})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
			"excel_id": token,
		})
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"First step for every sheets task — capture sheet_id from the result before doing any sheet-level operation.",
	},
}

// SheetCreate creates a new sub-sheet. --title is the new sheet's name;
// --index inserts at a specific position (omitted → appended). Default
// dimensions match the canonical schema (rows=100, cols=26 when omitted —
// tool's defaults differ but CLI surface stays predictable).
var SheetCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-create",
	Description: "Create a new sub-sheet with an optional position and initial dimensions.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-create"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		_, err = sheetCreateInput(runtime, token)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		input, _ := sheetCreateInput(runtime, token)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetTokenExec(runtime)
		if err != nil {
			return err
		}
		input, err := sheetCreateInput(runtime, token)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"+sheet-create makes an empty sub-sheet. To create a sub-sheet and fill it with typed data and/or styles in one step, use +table-put (missing sheets named in the payload are created automatically) with its --sheets and --styles flags.",
	},
}

func sheetCreateInput(runtime flagView, token string) (map[string]interface{}, error) {
	if strings.TrimSpace(runtime.Str("title")) == "" {
		return nil, common.ValidationErrorf("--title is required")
	}
	sheetType := strings.TrimSpace(runtime.Str("type"))
	if sheetType == "" {
		sheetType = "sheet"
	}
	if sheetType != "sheet" {
		return nil, common.ValidationErrorf("--type must be 'sheet'")
	}
	if n := runtime.Int("row-count"); n < 0 || n > 50000 {
		return nil, common.ValidationErrorf("--row-count must be between 0 and 50000")
	}
	if n := runtime.Int("col-count"); n < 0 || n > 200 {
		return nil, common.ValidationErrorf("--col-count must be between 0 and 200")
	}
	input := map[string]interface{}{
		"excel_id":   token,
		"operation":  "create",
		"sheet_name": strings.TrimSpace(runtime.Str("title")),
	}
	if runtime.Changed("index") {
		input["target_index"] = runtime.Int("index")
	}
	if n := runtime.Int("row-count"); n > 0 {
		input["rows"] = n
	}
	if n := runtime.Int("col-count"); n > 0 {
		input["columns"] = n
	}
	return input, nil
}

// sheetDeleteInput / sheetRenameInput / sheetVisibilityInput /
// sheetSetTabColorInput build the modify_workbook_structure body for the
// matching shortcut. Shared by standalone DryRun/Execute and by the
// +batch-update sub-op dispatch so both paths emit an identical body and the
// same friendly error when --sheet-id/--sheet-name (or the shortcut's own
// required flags) are missing.
func sheetDeleteInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	input := map[string]interface{}{"excel_id": token, "operation": "delete"}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

func sheetRenameInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runtime.Str("title")) == "" {
		return nil, common.ValidationErrorf("--title is required")
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "rename",
		"new_name":  strings.TrimSpace(runtime.Str("title")),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

func sheetVisibilityInput(runtime flagView, token, sheetID, sheetName, op string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	input := map[string]interface{}{"excel_id": token, "operation": op}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

func sheetSetTabColorInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	if !runtime.Changed("color") {
		return nil, common.ValidationErrorf("--color is required (empty string clears)")
	}
	input := map[string]interface{}{
		"excel_id":  token,
		"operation": "set_tab_color",
		"tab_color": runtime.Str("color"),
	}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	return input, nil
}

// SheetDelete deletes a sub-sheet. high-risk-write — framework rejects
// without --yes. Always preview with --dry-run first to confirm the target.
var SheetDelete = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-delete",
	Description: "Delete a sub-sheet (irreversible).",
	Risk:        "high-risk-write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-delete"),
	Validate:    validateViaInput(sheetDeleteInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := sheetDeleteInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input, err := sheetDeleteInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Sheet deletion is irreversible. Always run with --dry-run first to verify the target sheet_id/sheet_name.",
	},
}

// SheetRename renames a sub-sheet via --title (mapped to tool's new_name).
var SheetRename = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-rename",
	Description: "Rename a sub-sheet.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-rename"),
	Validate:    validateViaInput(sheetRenameInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := sheetRenameInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input, err := sheetRenameInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// SheetMove moves a sub-sheet to a new index. The tool requires sheet_id
// and source_index in addition to target_index. The CLI accepts:
//   - --sheet-id / --sheet-name to identify the sheet
//   - --source-index (optional) for explicit source position
//
// When --source-index is omitted, or when --sheet-name is used instead of
// --sheet-id, Execute issues a single get_workbook_structure read to derive
// the missing pieces. DryRun stays network-free: it uses <resolve> placeholders
// for any field that would need that read.
var SheetMove = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-move",
	Description: "Move a sub-sheet to a new position.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:read", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-move"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if !runtime.Changed("index") {
			return common.ValidationErrorf("--index is required")
		}
		if runtime.Int("index") < 0 {
			return common.ValidationErrorf("--index must be >= 0")
		}
		if runtime.Changed("source-index") && runtime.Int("source-index") < 0 {
			return common.ValidationErrorf("--source-index must be >= 0")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input := map[string]interface{}{
			"excel_id":     token,
			"operation":    "move",
			"sheet_id":     sheetSelectorPlaceholder(sheetID, sheetName),
			"target_index": runtime.Int("index"),
			"source_index": sourceIndexOrPlaceholder(runtime),
		}
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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

		resolvedID := sheetID
		var sourceIndex int
		needIDLookup := sheetID == ""
		needIndexLookup := !runtime.Changed("source-index")
		if needIDLookup || needIndexLookup {
			lookedID, lookedIdx, err := lookupSheetIndex(ctx, runtime, token, sheetID, sheetName)
			if err != nil {
				return err
			}
			resolvedID = lookedID
			sourceIndex = lookedIdx
		}
		if runtime.Changed("source-index") {
			sourceIndex = runtime.Int("source-index")
		}

		input := map[string]interface{}{
			"excel_id":     token,
			"operation":    "move",
			"sheet_id":     resolvedID,
			"source_index": sourceIndex,
			"target_index": runtime.Int("index"),
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
	Tips: []string{
		"Pass --source-index when you already know it to avoid the extra read; otherwise CLI derives it from --sheet-id/--sheet-name.",
	},
}

// sourceIndexOrPlaceholder returns the user-supplied source-index, or the
// string "<resolve>" when DryRun should signal that Execute will derive it.
func sourceIndexOrPlaceholder(runtime *common.RuntimeContext) interface{} {
	if runtime.Changed("source-index") {
		return runtime.Int("source-index")
	}
	return "<resolve>"
}

// SheetCopy duplicates a sub-sheet. --title (optional) names the copy;
// --index (optional) places it.
var SheetCopy = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-copy",
	Description: "Duplicate a sub-sheet, optionally renaming and repositioning the copy.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-copy"),
	Tips: []string{
		"Example: lark-cli sheets +sheet-copy --url <URL> --sheet-name 数据源 --title 数据源-副本",
		"--sheet-name / --sheet-id selects the SOURCE sheet; the copy's new name goes in --title.",
	},
	Validate: validateViaInput(sheetCopyInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := sheetCopyInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input, err := sheetCopyInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func sheetCopyInput(runtime flagView, token, sheetID, sheetName string) (map[string]interface{}, error) {
	if err := requireSheetSelector(sheetID, sheetName); err != nil {
		return nil, err
	}
	input := map[string]interface{}{"excel_id": token, "operation": "duplicate"}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if t := strings.TrimSpace(runtime.Str("title")); t != "" {
		input["new_name"] = t
	}
	if runtime.Changed("index") {
		input["target_index"] = runtime.Int("index")
	}
	return input, nil
}

// SheetHide / SheetUnhide toggle visibility. Visible bool semantics live in
// the operation enum so callers don't need a --visible flag.
var SheetHide = newSheetVisibilityShortcut(
	"+sheet-hide", "Hide a sub-sheet from the tabs bar.", "hide",
)

var SheetUnhide = newSheetVisibilityShortcut(
	"+sheet-unhide", "Restore a hidden sub-sheet.", "unhide",
)

func newSheetVisibilityShortcut(command, desc, op string) common.Shortcut {
	return common.Shortcut{
		Service:     "sheets",
		Command:     command,
		Description: desc,
		Risk:        "write",
		Scopes:      []string{"sheets:spreadsheet:write_only"},
		AuthTypes:   []string{"user", "bot"},
		HasFormat:   true,
		Flags:       flagsFor(command),
		Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
			token, err := resolveSpreadsheetToken(runtime)
			if err != nil {
				return err
			}
			sheetID := strings.TrimSpace(runtime.Str("sheet-id"))
			sheetName := strings.TrimSpace(runtime.Str("sheet-name"))
			_, err = sheetVisibilityInput(runtime, token, sheetID, sheetName, op)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input, _ := sheetVisibilityInput(runtime, token, sheetID, sheetName, op)
			return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
			input, err := sheetVisibilityInput(runtime, token, sheetID, sheetName, op)
			if err != nil {
				return err
			}
			out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
			if err != nil {
				return err
			}
			runtime.Out(out, nil)
			return nil
		},
	}
}

// SheetSetTabColor sets the tab color of a sub-sheet. --color "" clears.
var SheetSetTabColor = common.Shortcut{
	Service:     "sheets",
	Command:     "+sheet-set-tab-color",
	Description: "Set or clear the tab color of a sub-sheet.",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+sheet-set-tab-color"),
	Validate:    validateViaInput(sheetSetTabColorInput),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input, _ := sheetSetTabColorInput(runtime, token, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input, err := sheetSetTabColorInput(runtime, token, sheetID, sheetName)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

// SheetShowGridline / SheetHideGridline toggle a sub-sheet's gridline display.
// Gridline show/hide is the same two-state-via-operation shape as
// +sheet-hide/+sheet-unhide (no --visible flag), so they reuse
// newSheetVisibilityShortcut; only the operation enum differs.
var SheetShowGridline = newSheetVisibilityShortcut(
	"+sheet-show-gridline", "Show gridlines on a sub-sheet.", "show_gridline",
)

var SheetHideGridline = newSheetVisibilityShortcut(
	"+sheet-hide-gridline", "Hide gridlines on a sub-sheet.", "hide_gridline",
)

// ─── +workbook-create (legacy OAPI, cli_status: cli-only) ────────────
//
// Creates a brand-new spreadsheet via POST /sheets/v3/spreadsheets, then
// optionally fills the first sheet's header row and initial data block
// via a follow-up callTool(set_cell_range). Not exposed as an MCP tool —
// hence the direct legacy OAPI call instead of going through callTool.

// WorkbookCreate creates a brand-new spreadsheet in the user's drive
// (optionally inside --folder-token) and can pre-fill the first row of
// headers and an initial data block.
var WorkbookCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-create",
	Description: "Create a new spreadsheet, optionally pre-filled with untyped --values or typed --sheets (type-faithful one-step create + write).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:create", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+workbook-create"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("title")) == "" {
			return common.ValidationErrorf("--title is required")
		}
		// --sheets (typed JSON) is the typed data entry, mutually exclusive
		// with the untyped --values. Gating on Changed (not just non-empty)
		// catches an explicitly-given but empty payload as an error instead
		// of letting it fall through to creating an empty workbook.
		sheetsGiven := runtime.Changed("sheets")
		if sheetsGiven && runtime.Str("values") != "" {
			return common.ValidationErrorf("--values is mutually exclusive with --sheets")
		}
		if sheetsGiven {
			if strings.TrimSpace(runtime.Str("sheets")) == "" {
				return common.ValidationErrorf("--sheets was given but resolved to empty (empty stdin/file?); pass a typed payload, or drop --sheets to create an empty workbook")
			}
			payload, err := parseTablePutPayload(runtime)
			if err != nil {
				return err
			}
			styles, err := parseWorkbookCreateSheetStyles(runtime, payload, false)
			if err != nil {
				return err
			}
			if err := payload.checkCellBudgetWithStyles(styles); err != nil {
				return err
			}
			return checkStylesAnchors(payload, styles, true)
		}
		// Untyped --values path: parse (and validate) --styles as a single sheet
		// style item, then synthesize --values into a type-less typed payload —
		// the same construction buildValuesPayload runs at execute time, so any
		// malformed --values / --styles is caught here before a workbook is made.
		sheetStyles, err := parseValuesSheetStyles(runtime)
		if err != nil {
			return err
		}
		payload, err := buildValuesPayload(runtime, sheetStyles)
		if err != nil {
			return err
		}
		return checkStylesAnchors(payload, sheetStyles, true)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{"title": strings.TrimSpace(runtime.Str("title"))}
		if v := strings.TrimSpace(runtime.Str("folder-token")); v != "" {
			body["folder_token"] = v
		}
		dry := common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets").
			Desc("create spreadsheet").
			Body(body)
		// Both data entries (typed --sheets and untyped --values) resolve to the
		// same typed payload and preview through the same set_cell_range path: one
		// write per sheet, the first adopting the new workbook's default sheet.
		// Mirrors +table-put's dry-run against a placeholder token.
		payload, sheetStyles, _ := workbookCreateData(runtime)
		if payload == nil {
			// Style-only payload with no cell-rectangle extent (e.g. only
			// row_sizes or col_sizes). No set_cell_range to render, but the
			// visual ops (merges / row+col sizes) still run in Execute, so
			// they should show up in the dry-run plan too.
			if styles := sheetStyles.styleFor(0); styles != nil {
				appendWorkbookCreateVisualOpsDryRun(dry, "<new-token>", "", valuesSheetName, styles)
			}
			return dry
		}
		for i := range payload.Sheets {
			s := &payload.Sheets[i]
			matrix, _ := buildSheetMatrix(s, headerOn(s))
			_, col0, row0, _ := sheetAnchor(s)
			matrix, _ = applyWorkbookCreateStylesToMatrix(matrix, sheetStyles.styleFor(i), col0, row0, fmt.Sprintf("--styles for sheet %q", s.Name))
			// Padding can widen / lengthen the matrix past the data, so build the
			// range from the padded dims to match what Execute writes.
			rng := tablePutFullRange(s, len(matrix))
			writeCols := len(s.Columns)
			if len(matrix) > 0 {
				writeCols = len(matrix[0])
				rng = fmt.Sprintf("%s%d:%s%d",
					columnIndexToLetter(col0), row0+1,
					columnIndexToLetter(col0+writeCols-1), row0+len(matrix))
			}
			input := map[string]interface{}{
				"excel_id":   "<new-token>",
				"sheet_name": s.Name,
				"range":      rng,
				"cells":      matrix,
			}
			wireBody, _ := buildToolBody("set_cell_range", input)
			dry.POST("/open-apis/sheet_ai/v2/spreadsheets/<new-token>/tools/invoke_write").
				Desc(fmt.Sprintf("write sheet %q (%d data rows × %d cols) via set_cell_range", s.Name, len(s.Rows), writeCols)).
				Body(wireBody)
			appendWorkbookCreateVisualOpsDryRun(dry, "<new-token>", "", s.Name, sheetStyles.styleFor(i))
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{"title": strings.TrimSpace(runtime.Str("title"))}
		if v := strings.TrimSpace(runtime.Str("folder-token")); v != "" {
			body["folder_token"] = v
		}
		data, err := runtime.CallAPITyped("POST", "/open-apis/sheets/v3/spreadsheets", nil, body)
		if err != nil {
			return err
		}
		ss := common.GetMap(data, "spreadsheet")
		token := common.GetString(ss, "spreadsheet_token")
		if token == "" {
			token = common.GetString(ss, "token")
		}
		if token == "" {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "spreadsheet created but token missing in response")
		}

		result := map[string]interface{}{"spreadsheet": ss}

		// Both data entries resolve to the same typed payload: --sheets directly,
		// --values synthesized into a type-less payload. Both write through
		// writeTypedSheets, adopting the brand-new workbook's default sheet as the
		// first payload sheet so no empty "Sheet1" is left behind.
		payload, sheetStyles, err := workbookCreateData(runtime)
		if err != nil {
			return err // already validated; defensive
		}
		if payload != nil {
			firstSheetID, err := lookupFirstSheetID(ctx, runtime, token)
			if err != nil {
				return workbookCreatedButFillFailed(runtime, token, "resolving its default sheet for the write failed", err)
			}
			written, err := writeTypedSheets(ctx, runtime, token, payload, firstSheetID, sheetStyles)
			if err != nil {
				return workbookCreatedButFillFailed(runtime, token, "initial fill failed", err)
			}
			result["sheets"] = written
		} else if styles := sheetStyles.styleFor(0); styles != nil {
			// Style-only payloads (e.g. --styles with only row_sizes or col_sizes
			// and no --values/--sheets) don't write any cells but still need their
			// visual ops applied — otherwise the merges/sizes would be silently
			// dropped. workbookCreateStyleDimensions can't expand a row-only or
			// column-only range into a cell rectangle, so the no-data branch lives
			// here.
			firstSheetID, err := lookupFirstSheetID(ctx, runtime, token)
			if err != nil {
				return workbookCreatedButFillFailed(runtime, token, "resolving its default sheet for the write failed", err)
			}
			if err := applyWorkbookCreateVisualOps(ctx, runtime, token, firstSheetID, styles); err != nil {
				return workbookCreatedButFillFailed(runtime, token, "applying visual styles failed", err)
			}
		}
		runtime.Out(result, nil)
		return nil
	},
	Tips: []string{
		"--values is an optional untyped fill (one JSON 2D array). It writes through the same batched set_cell_range path as --sheets; pair it with --styles to set number formats, colors, merges, and row/col sizes. Partial failure leaves the spreadsheet created but empty.",
		"--sheets writes typed, type-faithful data (dates → real dates, numbers keep precision) in one step — the create + typed write that +table-put can't do on its own. Mutually exclusive with --values; the new workbook's default sheet becomes the first sheet (no empty Sheet1 left behind).",
	},
}

// workbookCreatedButFillFailed reports a workbook-create where the spreadsheet
// POST succeeded but the follow-up initial fill did not. It is the same
// partial-state shape as +table-put's multi-sheet half-write: stdout carries an
// ok:false envelope with the new spreadsheet_token (so the caller can retry the
// fill via +cells-set / +csv-put, or delete the orphan), and the process exits
// with the partial-failure signal — keeping a single sheets-domain contract for
// "the side effect landed but the follow-up didn't" instead of two (this used to
// surface as a typed failed_precondition on stderr, which agents couldn't tell
// apart from a plain validation refusal). The underlying cause's typed shape is
// flattened into a structured `cause` field so the inner subtype / category /
// message stays diagnosable from the JSON envelope alone.
func workbookCreatedButFillFailed(runtime *common.RuntimeContext, token, reason string, cause error) error {
	data := map[string]interface{}{
		"spreadsheet_token": token,
		"reason":            fmt.Sprintf("spreadsheet %s created but %s", token, reason),
		"hint":              "the spreadsheet exists; retry the fill with the returned spreadsheet_token (+cells-set / +csv-put), or delete it",
	}
	if cause != nil {
		if p, ok := errs.ProblemOf(cause); ok {
			data["cause"] = map[string]interface{}{
				"category": string(p.Category),
				"subtype":  string(p.Subtype),
				"message":  p.Message,
			}
		} else {
			data["cause"] = map[string]interface{}{"message": cause.Error()}
		}
	}
	return runtime.OutPartialFailure(data, nil)
}

// valuesSheetName is the synthesized sheet name for the untyped --values path.
// It matches a freshly created workbook's default sheet, so writeTypedSheets
// adopts that sheet in place (no rename, no stray sheet) — see its adopt logic.
// Lark Sheets names the default sheet "Sheet1" on create.
const valuesSheetName = "Sheet1"

// workbookCreateData resolves the data to write into a freshly created workbook:
// typed --sheets directly, or untyped --values synthesized as a single sheet of
// type-less (raw passthrough) columns. Both go through writeTypedSheets so the
// two entries share one batched set_cell_range writer. Returns (nil, nil, nil)
// when there's nothing to fill (no --sheets, and no --values/--styles extent).
func workbookCreateData(runtime *common.RuntimeContext) (*tablePayload, *workbookCreateSheetStyles, error) {
	if runtime.Changed("sheets") {
		payload, err := parseTablePutPayload(runtime)
		if err != nil {
			return nil, nil, err
		}
		styles, err := parseWorkbookCreateSheetStyles(runtime, payload, false)
		if err != nil {
			return nil, nil, err
		}
		return payload, styles, nil
	}
	styles, err := parseValuesSheetStyles(runtime)
	if err != nil {
		return nil, nil, err
	}
	payload, err := buildValuesPayload(runtime, styles)
	if err != nil {
		return nil, nil, err
	}
	return payload, styles, nil
}

// parseValuesSheetStyles parses --styles for the untyped --values path and wraps
// the single style item as a one-sheet workbookCreateSheetStyles, so --values
// reuses writeSheetData's styleFor application. The item's name is ignored (the
// synthesized sheet is always index 0). Returns nil when --styles is absent.
func parseValuesSheetStyles(runtime flagView) (*workbookCreateSheetStyles, error) {
	p, err := parseWorkbookCreateStyles(runtime)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	return &workbookCreateSheetStyles{ByIndex: []*workbookCreateStylePayload{p}}, nil
}

// buildValuesPayload turns untyped --values into a single-sheet typed payload of
// type-less columns (Header=false), so --values shares --sheets' batched
// set_cell_range writer. Rows are normalized to a rectangle wide/long enough to
// also cover any --styles cell ranges (matching the old buildInitialFillInput,
// where a style on B3 extends the written block). Returns (nil, nil) when there
// is nothing to write — no --values rows and no style-driven extent.
func buildValuesPayload(runtime flagView, sheetStyles *workbookCreateSheetStyles) (*tablePayload, error) {
	rows, err := parseValuesRows(runtime)
	if err != nil {
		return nil, err
	}
	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}
	var styleRows, styleCols int
	if sheetStyles != nil {
		styleRows, styleCols = workbookCreateStyleDimensions(sheetStyles.styleFor(0), 0, 0)
	}
	if styleCols > maxCols {
		maxCols = styleCols
	}
	nrows := len(rows)
	if styleRows > nrows {
		nrows = styleRows
	}
	if maxCols == 0 || nrows == 0 {
		return nil, nil // nothing to write (e.g. --values '[]' with no styles)
	}
	if err := checkTablePutCellBudget(int64(nrows) * int64(maxCols)); err != nil {
		return nil, err
	}
	// Pad to a rectangle; nil cells become empty cells in buildTypedCell.
	for len(rows) < nrows {
		rows = append(rows, nil)
	}
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], nil)
		}
	}
	cols := make([]tableColumnSpec, maxCols)
	for i := range cols {
		cols[i] = tableColumnSpec{Name: fmt.Sprintf("col%d", i+1)} // type-less
	}
	noHeader := false
	payload := &tablePayload{Sheets: []tableSheetSpec{{
		Name:    valuesSheetName,
		Mode:    "overwrite",
		Header:  &noHeader,
		Columns: cols,
		Rows:    rows,
	}}}
	// --values bypasses tablePayload.validate(), so enforce the cell budget here
	// too — otherwise a giant --values array materializes unbounded.
	if err := payload.checkCellBudget(); err != nil {
		return nil, err
	}
	return payload, nil
}

// parseValuesRows decodes --values (JSON 2D array, with @file/stdin already
// resolved by the flag layer) using UseNumber so numeric cells keep full
// precision (large order IDs survive). Empty --values yields no rows.
func parseValuesRows(runtime flagView) ([][]interface{}, error) {
	raw := strings.TrimSpace(runtime.Str("values"))
	if raw == "" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil, common.ValidationErrorf("--values: invalid JSON: %v", err)
	}
	// Reject trailing non-whitespace after the first JSON value: see
	// decoderExpectEOF in lark_sheet_table_io.go for the rationale.
	if err := decoderExpectEOF(dec); err != nil {
		return nil, common.ValidationErrorf("--values: %v", err).WithCause(err)
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil, common.ValidationErrorf("--values must be a JSON 2D array")
	}
	rows := make([][]interface{}, len(arr))
	for i, r := range arr {
		cells, ok := r.([]interface{})
		if !ok {
			return nil, common.ValidationErrorf("--values[%d] must be an array", i)
		}
		rows[i] = cells
	}
	return rows, nil
}

type workbookCreateStylePayload struct {
	CellStyles []workbookCreateCellStyleOp
	RowSizes   []workbookCreateResizeOp
	ColSizes   []workbookCreateResizeOp
	CellMerges []workbookCreateMergeOp
	Freeze     *workbookCreateFreezeOp
}

// workbookCreateFreezeOp freezes the first Rows rows / Cols columns.
// Zero means "that axis ends up UNFROZEN", not "leave it alone": freeze is
// full-state replacement server-side (see workbookCreateVisualOpInput's freeze
// branch), so a declarative spec that omits an axis is stating it should not be
// frozen. All-zero means "unfreeze both axes" and is only accepted on carriers
// targeting an existing sheet (parseWorkbookCreateFreezeOp rejects it on the
// create path, where a new sheet starts unfrozen anyway).
type workbookCreateFreezeOp struct {
	Rows int
	Cols int
}

type workbookCreateCellStyleOp struct {
	Range string
	Style map[string]interface{}
}

type workbookCreateMergeOp struct {
	Range     string
	MergeType string
}

type workbookCreateResizeOp struct {
	Range      string
	ResizeType string
	Size       int
}

type workbookCreateSheetStyles struct {
	ByIndex []*workbookCreateStylePayload
	ByName  map[string]*workbookCreateStylePayload
}

func (s *workbookCreateSheetStyles) styleFor(index int) *workbookCreateStylePayload {
	if s == nil {
		return nil
	}
	if index >= 0 && index < len(s.ByIndex) && s.ByIndex[index] != nil {
		return s.ByIndex[index]
	}
	return nil
}

// parseWorkbookCreateStyles parses --styles for +workbook-create's untyped
// initial-fill path. The outer protocol is always {"styles":[...]}; untyped
// initial fill consumes exactly one item from that array.
func parseWorkbookCreateStyles(runtime flagView) (*workbookCreateStylePayload, error) {
	if strings.TrimSpace(runtime.Str("styles")) == "" {
		return nil, nil
	}
	v, err := parseJSONFlag(runtime, "styles")
	if err != nil {
		return nil, err
	}
	items, err := parseWorkbookCreateStylesItems(v)
	if err != nil {
		return nil, err
	}
	if len(items) != 1 {
		return nil, common.ValidationErrorf("--styles.styles must contain exactly one item when using --values")
	}
	payload, probs := parseWorkbookCreateStyleItem(items[0], "--styles.styles[0]", false)
	if err := joinStyleValidationErrors(probs); err != nil {
		return nil, err
	}
	return payload, nil
}

// parseWorkbookCreateSheetStyles parses --styles for the typed --sheets path.
// The outer protocol is always {"styles":[...]}, and the array is aligned with
// --sheets.sheets. Each item must name the same sheet at the same index.
// existingSheet says whether the carrier targets an existing spreadsheet
// (+table-put) rather than one being created (+workbook-create) — it gates
// whether an all-zero freeze may express "unfreeze both axes".
func parseWorkbookCreateSheetStyles(runtime flagView, payload *tablePayload, existingSheet bool) (*workbookCreateSheetStyles, error) {
	if strings.TrimSpace(runtime.Str("styles")) == "" {
		return nil, nil
	}
	v, err := parseJSONFlag(runtime, "styles")
	if err != nil {
		return nil, err
	}
	items, err := parseWorkbookCreateStylesItems(v)
	if err != nil {
		return nil, err
	}
	if len(items) != len(payload.Sheets) {
		return nil, common.ValidationErrorf("--styles.styles has %d items, want %d to match --sheets.sheets", len(items), len(payload.Sheets))
	}
	out := &workbookCreateSheetStyles{ByName: map[string]*workbookCreateStylePayload{}}
	out.ByIndex = make([]*workbookCreateStylePayload, len(payload.Sheets))
	var probs []error
	for i, item := range items {
		name, _ := item["name"].(string)
		if strings.TrimSpace(name) == "" {
			probs = append(probs, common.ValidationErrorf("--styles.styles[%d].name is required", i))
			continue
		}
		if name != payload.Sheets[i].Name {
			probs = append(probs, common.ValidationErrorf("--styles.styles[%d].name %q must match --sheets.sheets[%d].name %q", i, name, i, payload.Sheets[i].Name))
			continue
		}
		style, itemProbs := parseWorkbookCreateStyleItem(item, fmt.Sprintf("--styles.styles[%d]", i), existingSheet)
		if len(itemProbs) > 0 {
			probs = append(probs, itemProbs...)
			continue
		}
		out.ByIndex[i] = style
		out.ByName[name] = style
	}
	if err := joinStyleValidationErrors(probs); err != nil {
		return nil, err
	}
	return out, nil
}

const (
	// Keep declarative style payloads bounded before parsing allocates per-entry
	// error objects or the coalescer scans them. The operation budget is 100,
	// while a larger raw list is useful when adjacent same-style stamps merge.
	maxStyleItems        = 100
	maxStyleSectionItems = 1000
	maxStyleProblems     = 9 // eight displayed plus one truncation marker
)

func parseWorkbookCreateStylesItems(v interface{}) ([]map[string]interface{}, error) {
	root, ok := v.(map[string]interface{})
	if !ok {
		return nil, common.ValidationErrorf("--styles must be a JSON object shaped as {\"styles\":[...]}")
	}
	rawItems, ok := root["styles"]
	if !ok {
		return nil, common.ValidationErrorf("--styles.styles is required")
	}
	arr, ok := rawItems.([]interface{})
	if !ok {
		return nil, common.ValidationErrorf("--styles.styles must be an array")
	}
	if len(arr) > maxStyleItems {
		return nil, common.ValidationErrorf("--styles.styles accepts at most %d items; got %d", maxStyleItems, len(arr))
	}
	items := make([]map[string]interface{}, len(arr))
	for i, raw := range arr {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return nil, common.ValidationErrorf("--styles.styles[%d] must be an object", i)
		}
		items[i] = item
	}
	return items, nil
}

// parseWorkbookCreateStyleItem parses one --styles item. All four sections
// are validated even after one fails, and every issue is returned in the
// slice: eval traces show agents fixing --styles errors one round trip per
// error (border side, then row_sizes.type, then size…) because only the
// first was ever reported.
// workbookCreateStyleItemKeys is the full top-level vocabulary of one
// --styles item, shared by the three carriers (+workbook-create /
// +table-put / +styles-put).
var workbookCreateStyleItemKeys = []string{"name", "cell_styles", "row_sizes", "col_sizes", "cell_merges", "freeze"}

func boundedStyleProblems(probs *[]error, extra []error) {
	for _, err := range extra {
		if len(*probs) >= maxStyleProblems {
			return
		}
		*probs = append(*probs, err)
	}
}

func parseWorkbookCreateStyleItem(item map[string]interface{}, path string, existingSheet bool) (*workbookCreateStylePayload, []error) {
	payload := &workbookCreateStylePayload{}
	var probs []error
	oversized := make(map[string]bool)
	for _, section := range styleItemRangeSections {
		if raw, ok := item[section]; ok {
			if arr, ok := raw.([]interface{}); ok && len(arr) > maxStyleSectionItems {
				probs = append(probs, common.ValidationErrorf("%s.%s accepts at most %d items; got %d", path, section, maxStyleSectionItems, len(arr)))
				oversized[section] = true
			}
		}
	}
	// Reject unknown top-level keys first: a typo like "freezee" would
	// otherwise be silently dropped while the rest of the item applies.
	var unknown []string
	unknownCount := 0
	for k := range item {
		known := false
		for _, lk := range workbookCreateStyleItemKeys {
			if k == lk {
				known = true
				break
			}
		}
		if !known {
			unknownCount++
			if len(unknown) < maxStyleProblems {
				unknown = append(unknown, k)
			}
		}
	}
	if unknownCount > len(unknown) {
		// Keep the deterministic sorted sample bounded; the aggregate formatter
		// reports the omitted count without retaining every unknown key.
		probs = append(probs, common.ValidationErrorf("%s has %d unknown keys (showing the first %d)", path, unknownCount, len(unknown)))
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		if len(probs) >= maxStyleProblems {
			break
		}
		msg := fmt.Sprintf("%s has unknown key %q", path, k)
		if match := suggest.Closest(strings.ToLower(k), workbookCreateStyleItemKeys, 1); len(match) > 0 {
			msg += fmt.Sprintf(" — did you mean %q?", match[0])
		}
		probs = append(probs, common.ValidationErrorf("%s", msg))
	}
	// Normalize "Sheet!" range prefixes before the section parsers see them:
	// the target sheet is named by the item (or, on +workbook-create --values,
	// by the single sheet being created), so a prefix is at best redundant and
	// at worst a silent retarget. Stripping is unconditional — an item without
	// a name (the --values path, where name is optional) must not be left with
	// prefixed ranges the section parsers then reject as malformed. Only the
	// "names a DIFFERENT sheet" report needs a name to compare against, so it
	// is skipped when there is none.
	name, _ := item["name"].(string)
	probs = append(probs, normalizeStyleItemRangePrefixes(item, path, strings.TrimSpace(name))...)
	if raw, ok := item["cell_styles"]; ok && !oversized["cell_styles"] {
		var errsHere []error
		payload.CellStyles, errsHere = parseWorkbookCreateCellStyleOps(raw, path+".cell_styles")
		boundedStyleProblems(&probs, errsHere)
	}
	if raw, ok := item["row_sizes"]; ok && !oversized["row_sizes"] {
		var errsHere []error
		payload.RowSizes, errsHere = parseWorkbookCreateResizeOps(raw, path+".row_sizes", "row")
		boundedStyleProblems(&probs, errsHere)
	}
	if raw, ok := item["col_sizes"]; ok && !oversized["col_sizes"] {
		var errsHere []error
		payload.ColSizes, errsHere = parseWorkbookCreateResizeOps(raw, path+".col_sizes", "column")
		boundedStyleProblems(&probs, errsHere)
	}
	if raw, ok := item["cell_merges"]; ok && !oversized["cell_merges"] {
		var errsHere []error
		payload.CellMerges, errsHere = parseWorkbookCreateMergeOps(raw, path+".cell_merges")
		boundedStyleProblems(&probs, errsHere)
	}
	if raw, ok := item["freeze"]; ok {
		freeze, err := parseWorkbookCreateFreezeOp(raw, path+".freeze", existingSheet)
		if err != nil {
			boundedStyleProblems(&probs, []error{err})
		} else {
			payload.Freeze = freeze
		}
	}
	if len(probs) > 0 {
		return nil, probs
	}
	if len(payload.CellStyles) == 0 && len(payload.RowSizes) == 0 && len(payload.ColSizes) == 0 && len(payload.CellMerges) == 0 && payload.Freeze == nil {
		return nil, []error{common.ValidationErrorf("%s must include at least one of cell_styles/row_sizes/col_sizes/cell_merges/freeze", path)}
	}
	return payload, nil
}

// styleItemRangeSections are the --styles item sections whose entries carry an
// A1 range that may be written with a redundant "Sheet!" prefix.
var styleItemRangeSections = []string{"cell_styles", "row_sizes", "col_sizes", "cell_merges"}

// normalizeStyleItemRangePrefixes strips an optional "Sheet!" prefix from every
// range in one --styles item, in place, and reports the ones naming a sheet
// other than the item's own.
//
// Stripping has to happen before the section parsers run: parseWorkbookCreateResizeOp
// feeds the range straight to parseA1Range, so row_sizes like "Sheet1!2:3" fail
// as malformed even though the intent is unambiguous — the target sheet is
// already carried by the item name and by each expanded sub-op's sheet selector.
// A prefix naming a DIFFERENT sheet is an error rather than a strip, because
// stripping alone would silently retarget the operation onto the item's sheet
// (name "Summary" + range "Detail!A1:D1" applying to Summary). It is stripped
// anyway so the section parser reports the entry's own issues instead of piling
// a redundant syntax error on top of the mismatch.
//
// name is "" on +workbook-create --values, whose single styles item needs no
// name (the workbook has exactly one sheet, still unnamed at spec time). There
// is then no sheet to disagree with, so ranges are stripped without the
// mismatch report — stripping still has to happen, or the section parsers see
// a prefixed range and reject it as malformed.
func normalizeStyleItemRangePrefixes(item map[string]interface{}, path, name string) []error {
	var probs []error
	rewrite := func(section, rangeStr string) (string, bool) {
		idx := strings.Index(rangeStr, "!")
		if idx < 0 {
			return "", false
		}
		prefix := strings.Trim(strings.TrimSpace(rangeStr[:idx]), "'")
		if name != "" && prefix != name {
			probs = append(probs, common.ValidationErrorf(
				"%s.%s range %q names sheet %q but the item targets %q — drop the prefix, or move the entry into the item for %q",
				path, section, rangeStr, prefix, name, prefix))
		}
		return strings.TrimSpace(rangeStr[idx+1:]), true
	}
	for _, key := range styleItemRangeSections {
		arr, ok := item[key].([]interface{})
		if !ok {
			continue // a wrong-shaped section is the section parser's to report.
		}
		if len(arr) > maxStyleSectionItems {
			continue // parseWorkbookCreateStyleItem reports the bounded-size error.
		}
		for i, elem := range arr {
			section := fmt.Sprintf("%s[%d]", key, i)
			switch v := elem.(type) {
			case map[string]interface{}:
				rangeStr, ok := v["range"].(string)
				if !ok {
					continue // non-string/missing range: the section parser reports it.
				}
				if stripped, changed := rewrite(section, rangeStr); changed {
					v["range"] = stripped
				}
			case string:
				// cell_merges also accepts a bare range string.
				if key != "cell_merges" {
					continue
				}
				if stripped, changed := rewrite(section, v); changed {
					arr[i] = stripped
				}
			}
		}
	}
	return probs
}

// parseWorkbookCreateFreezeOp parses a {rows, cols} freeze section. Freeze is
// full-state replacement server-side, so on a carrier that targets an EXISTING
// sheet (existingSheet true: +styles-put, +table-put) an explicit all-zero op
// states "both axes unfrozen" and emits the bare unfreeze operation — the same
// state +dim-freeze --rows 0 --cols 0 expresses. On a create-path carrier
// (+workbook-create) a new sheet starts unfrozen, so all-zero is a no-op the
// caller almost certainly didn't mean and is rejected.
func parseWorkbookCreateFreezeOp(raw interface{}, path string, existingSheet bool) (*workbookCreateFreezeOp, error) {
	obj, ok := raw.(map[string]interface{})
	if !ok {
		return nil, common.ValidationErrorf("%s must be an object like {\"rows\":1} or {\"rows\":1,\"cols\":2}", path)
	}
	// "cols" and "columns" are aliases for the same field, so accepting both in
	// one object would make the result depend on Go's randomized map iteration
	// order — the same payload could freeze 1 column on one run and 2 on the
	// next. Reject the conflict instead of silently picking a winner.
	if _, hasCols := obj["cols"]; hasCols {
		if _, hasColumns := obj["columns"]; hasColumns {
			if !jsonEqual(obj["cols"], obj["columns"]) {
				return nil, common.ValidationErrorf("%s got conflicting values for \"cols\" and \"columns\" (aliases of the same field) — keep one", path)
			}
		}
	}
	out := &workbookCreateFreezeOp{}
	// Iterate deterministically so error reporting is stable across runs too.
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := obj[k]
		n, isNum := v.(float64)
		if !isNum || n != float64(int(n)) || n < 0 {
			return nil, common.ValidationErrorf("%s.%s must be a non-negative integer", path, k)
		}
		switch k {
		case "rows":
			out.Rows = int(n)
		case "cols", "columns":
			out.Cols = int(n)
		default:
			return nil, common.ValidationErrorf("%s.%s is not a supported field (want rows/cols)", path, k)
		}
	}
	if out.Rows == 0 && out.Cols == 0 {
		if !existingSheet {
			return nil, common.ValidationErrorf("%s must freeze at least one dimension (rows or cols > 0) — a newly created sheet starts unfrozen", path)
		}
		if len(obj) == 0 {
			return nil, common.ValidationErrorf("%s must specify rows or cols; use {\"rows\":0,\"cols\":0} explicitly to unfreeze both axes", path)
		}
	}
	return out, nil
}

// joinStyleValidationErrors folds the issues collected across one --styles
// parse into a single typed error that lists them all, so the caller can fix
// the whole payload in one retry instead of one error per round trip.
func joinStyleValidationErrors(probs []error) error {
	switch len(probs) {
	case 0:
		return nil
	case 1:
		// Re-attribute to the outer flag even for a single issue: the inner
		// error is scoped to a nested path and carries no Param, so an agent
		// would have to parse prose to learn which flag to fix. Message text
		// is preserved; only the typed attribution is added — and the inner
		// hint rides along, since a lone issue has the outer Hint slot free.
		msg, hint := aggregatedIssueParts(probs[0])
		verr := sheetsValidationForFlag("styles", "%s", msg).WithCause(probs[0])
		if hint != "" {
			verr = verr.WithHint("%s", hint)
		}
		return verr
	}
	const maxShown = 8
	msgs := collapseAggregatedIssues(probs)
	distinct := len(msgs)
	suffix := ""
	if len(msgs) > maxShown {
		suffix = fmt.Sprintf(" (+%d more)", len(msgs)-maxShown)
		msgs = msgs[:maxShown]
	}
	if distinct < len(probs) {
		return sheetsValidationForFlag("styles", "--styles has %d issues (%d distinct): %s%s", len(probs), distinct, strings.Join(msgs, " | "), suffix).
			WithCause(probs[0])
	}
	return sheetsValidationForFlag("styles", "--styles has %d issues: %s%s", len(probs), strings.Join(msgs, " | "), suffix).
		WithCause(probs[0])
}

func parseWorkbookCreateCellStyleOps(v interface{}, path string) ([]workbookCreateCellStyleOp, []error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, []error{common.ValidationErrorf("%s must be an array", path)}
	}
	ops := make([]workbookCreateCellStyleOp, 0, len(arr))
	var probs []error
	for i, raw := range arr {
		op, err := parseWorkbookCreateCellStyleOp(raw, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			probs = append(probs, err)
			continue
		}
		ops = append(ops, op)
	}
	return ops, probs
}

func parseWorkbookCreateCellStyleOp(raw interface{}, path string) (workbookCreateCellStyleOp, error) {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return workbookCreateCellStyleOp{}, common.ValidationErrorf("%s must be an object", path)
	}
	rangeStr, err := requireWorkbookCreateRange(op, path)
	if err != nil {
		return workbookCreateCellStyleOp{}, err
	}
	if _, _, _, _, err := workbookCreateStyleRangeBounds(rangeStr); err != nil {
		return workbookCreateCellStyleOp{}, common.ValidationErrorf("%s.range %q: %v", path, rangeStr, err)
	}
	styleObj := make(map[string]interface{}, len(op)-1)
	for k, v := range op {
		if k == "range" {
			continue
		}
		styleObj[k] = v
	}
	style, err := normalizeWorkbookCreateStyleObject(styleObj, path)
	if err != nil {
		return workbookCreateCellStyleOp{}, err
	}
	if len(style) == 0 {
		return workbookCreateCellStyleOp{}, common.ValidationErrorf("%s must include at least one style field", path)
	}
	return workbookCreateCellStyleOp{Range: rangeStr, Style: style}, nil
}

func parseWorkbookCreateMergeOps(v interface{}, path string) ([]workbookCreateMergeOp, []error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, []error{common.ValidationErrorf("%s must be an array", path)}
	}
	ops := make([]workbookCreateMergeOp, 0, len(arr))
	var probs []error
	for i, raw := range arr {
		op, err := parseWorkbookCreateMergeOp(raw, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			probs = append(probs, err)
			continue
		}
		ops = append(ops, op)
	}
	return ops, probs
}

func parseWorkbookCreateMergeOp(raw interface{}, path string) (workbookCreateMergeOp, error) {
	// A bare range string means {range: s, merge_type: all} — the only
	// possible reading (07-20 eval hit).
	if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
		raw = map[string]interface{}{"range": strings.TrimSpace(s)}
	}
	op, ok := raw.(map[string]interface{})
	if !ok {
		return workbookCreateMergeOp{}, common.ValidationErrorf("%s must be an object", path)
	}
	rangeStr, err := requireWorkbookCreateRange(op, path)
	if err != nil {
		return workbookCreateMergeOp{}, err
	}
	if _, _, _, _, err := workbookCreateStyleRangeBounds(rangeStr); err != nil {
		return workbookCreateMergeOp{}, common.ValidationErrorf("%s.range %q: %v", path, rangeStr, err)
	}
	mergeType := "all"
	if raw, ok := op["merge_type"]; ok {
		v, ok := raw.(string)
		if !ok || strings.TrimSpace(v) == "" {
			return workbookCreateMergeOp{}, common.ValidationErrorf("%s.merge_type must be a non-empty string", path)
		}
		mergeType = normalizeMergeType(strings.TrimSpace(v))
	}
	switch mergeType {
	case "all", "rows", "columns":
	default:
		return workbookCreateMergeOp{}, common.ValidationErrorf("%s.merge_type %q is invalid (want all/rows/columns)", path, mergeType)
	}
	if err := rejectUnexpectedWorkbookStyleFields(op, path, "range", "merge_type"); err != nil {
		return workbookCreateMergeOp{}, err
	}
	return workbookCreateMergeOp{Range: rangeStr, MergeType: mergeType}, nil
}

// normalizeMergeType maps the raw OpenAPI merge vocabulary (MERGE_ALL /
// MERGE_ROWS / MERGE_COLUMNS — which agents reproduce from the Lark API
// docs) onto the CLI's all/rows/columns. Unknown values pass through for
// the caller's enum check to reject.
func normalizeMergeType(v string) string {
	lower := strings.ToLower(v)
	lower = strings.TrimPrefix(lower, "merge_")
	switch lower {
	case "all", "rows", "columns":
		return lower
	}
	return v
}

func parseWorkbookCreateResizeOps(v interface{}, path, dimension string) ([]workbookCreateResizeOp, []error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, []error{common.ValidationErrorf("%s must be an array", path)}
	}
	ops := make([]workbookCreateResizeOp, 0, len(arr))
	var probs []error
	for i, raw := range arr {
		op, err := parseWorkbookCreateResizeOp(raw, fmt.Sprintf("%s[%d]", path, i), dimension)
		if err != nil {
			probs = append(probs, err)
			continue
		}
		ops = append(ops, op)
	}
	return ops, probs
}

// resizeOpExample renders a complete valid op for the dimension, inlined on
// every type/size error: eval traces show the field errors chaining (type
// "custom" → fixed to pixel → "pixel requires size"), each costing a round
// trip, because no error ever showed a whole valid op at once.
func resizeOpExample(dimension string) string {
	if dimension == "column" {
		return `{"range":"A:C","type":"pixel","size":120} (or {"range":"A:C","type":"standard"} to reset)`
	}
	return `{"range":"2:10","type":"pixel","size":32} (or "type":"auto" to fit content)`
}

func parseWorkbookCreateResizeOp(raw interface{}, path, dimension string) (workbookCreateResizeOp, error) {
	op, ok := raw.(map[string]interface{})
	if !ok {
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s must be an object", path)
	}
	rangeStr, err := requireWorkbookCreateRange(op, path)
	if err != nil {
		return workbookCreateResizeOp{}, err
	}
	parsedDim, _, _, err := parseA1Range(rangeStr)
	if err != nil {
		want := "row numbers like 2:10"
		if dimension == "column" {
			want = "column letters like A:E"
		}
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s.range %q must use %s: %v", path, rangeStr, want, err)
	}
	if parsedDim != dimension {
		want := "row numbers like 2:10"
		if dimension == "column" {
			want = "column letters like A:E"
		}
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s.range %q must use %s", path, rangeStr, want)
	}
	typeHint := "pixel/standard"
	if dimension == "row" {
		typeHint = "pixel/standard/auto"
	}
	resizeType, _ := op["type"].(string)
	resizeType = strings.TrimSpace(resizeType)
	if resizeType != "" {
		if dimension == "column" && resizeType == "auto" {
			return workbookCreateResizeOp{}, common.ValidationErrorf("%s.type auto is rows-only", path)
		}
		switch resizeType {
		case "pixel", "standard", "auto":
		default:
			return workbookCreateResizeOp{}, common.ValidationErrorf("%s.type %q is invalid (want %s), e.g. %s", path, resizeType, typeHint, resizeOpExample(dimension))
		}
	}
	// size is the canonical dimension key (uniform across row_sizes and
	// col_sizes — the array name already carries the dimension). The Excel-
	// vocabulary alias (height on rows, width on columns) is accepted
	// silently; the WRONG dimension's word is a targeted error, never a
	// silent rewrite.
	alias, wrongDim := "height", "width"
	if dimension == "column" {
		alias, wrongDim = "width", "height"
	}
	if _, has := op[wrongDim]; has {
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s.%s does not apply to this array (the array name carries the dimension); use size, e.g. %s", path, wrongDim, resizeOpExample(dimension))
	}
	sizeRaw, hasSize := op["size"]
	if aliasRaw, hasAlias := op[alias]; hasAlias {
		if hasSize {
			return workbookCreateResizeOp{}, common.ValidationErrorf("%s: give either size or %s, not both", path, alias)
		}
		sizeRaw, hasSize = aliasRaw, true
	}
	size := 0
	if hasSize {
		n, ok := util.ToFloat64(sizeRaw)
		if !ok || n <= 0 {
			return workbookCreateResizeOp{}, common.ValidationErrorf("%s.size must be a positive number", path)
		}
		size = int(n)
	}
	// type is optional ceremony when a pixel size is given: {range, size}
	// means a pixel resize, exactly as --width/--height without --type does
	// on the flag path. Explicit standard/auto still needs type.
	if resizeType == "" {
		if size <= 0 {
			return workbookCreateResizeOp{}, common.ValidationErrorf("%s needs size (px) or type (%s), e.g. %s", path, typeHint, resizeOpExample(dimension))
		}
		resizeType = "pixel"
	}
	if resizeType == "pixel" && size <= 0 {
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s.type pixel requires size, e.g. %s", path, resizeOpExample(dimension))
	}
	if resizeType != "pixel" && size > 0 {
		return workbookCreateResizeOp{}, common.ValidationErrorf("%s.size is only valid with type pixel", path)
	}
	if err := rejectUnexpectedWorkbookStyleFields(op, path, "range", "type", "size", alias); err != nil {
		return workbookCreateResizeOp{}, err
	}
	return workbookCreateResizeOp{Range: normalizeWorkbookResizeRange(rangeStr), ResizeType: resizeType, Size: size}, nil
}

func requireWorkbookCreateRange(op map[string]interface{}, path string) (string, error) {
	rangeRaw, ok := op["range"]
	if !ok {
		return "", common.ValidationErrorf("%s.range is required", path)
	}
	rangeStr, ok := rangeRaw.(string)
	if !ok || strings.TrimSpace(rangeStr) == "" {
		return "", common.ValidationErrorf("%s.range must be a non-empty string", path)
	}
	return strings.TrimSpace(rangeStr), nil
}

func rejectUnexpectedWorkbookStyleFields(op map[string]interface{}, path string, allowed ...string) error {
	allow := map[string]struct{}{}
	for _, k := range allowed {
		allow[k] = struct{}{}
	}
	for k := range op {
		if _, ok := allow[k]; !ok {
			return common.ValidationErrorf("%s.%s is not valid here", path, k)
		}
	}
	return nil
}

func normalizeWorkbookResizeRange(rangeStr string) string {
	rangeStr = strings.TrimSpace(rangeStr)
	if !strings.Contains(rangeStr, ":") {
		return rangeStr + ":" + rangeStr
	}
	return rangeStr
}

func normalizeWorkbookCreateStyleObject(in map[string]interface{}, path string) (map[string]interface{}, error) {
	if len(in) == 0 {
		return nil, nil
	}
	if err := foldBorderFamilyAliases(in, path); err != nil {
		return nil, err
	}
	if err := normalizeCellStyleAliases(in, path); err != nil {
		return nil, err
	}
	out := map[string]interface{}{}
	cellStyle := map[string]interface{}{}
	for k, v := range in {
		switch k {
		case "cell_styles":
			return nil, common.ValidationErrorf("%s.cell_styles is not supported inside cell_styles[]; put style fields directly on the item", path)
		case "border_styles":
			m, ok := v.(map[string]interface{})
			if !ok {
				return nil, common.ValidationErrorf("%s.border_styles must be a JSON object", path)
			}
			expandBorderAllShorthand(m)
			if err := validateWorkbookBorderStyles(m, path); err != nil {
				return nil, err
			}
			out["border_styles"] = m
		case "value", "formula", "rich_text", "multiple_values", "note", "data_validation":
			return nil, common.ValidationErrorf("%s.%s is a content field — a styles spec carries no cell content; write values/formulas via +cells-set or +table-put", path, k)
		default:
			if !workbookCreateCellStyleField(k) {
				// Universal rejection with the full field list: this is the
				// mechanism that absorbs the infinite tail of spelling
				// permutations at a fixed one-retry cost — silent aliases are
				// reserved for high-frequency words from real external
				// vocabularies (see the style_vocab.go contract). A curated
				// prescription wins over did-you-mean; without one, the
				// distance match must be a near-typo (≤2 edits) — a
				// concept-swap neighbor (font_bold → font_color, distance 3)
				// misleads worse than silence.
				msg := fmt.Sprintf("%s.%s is not a supported style field", path, k)
				lower := strings.ToLower(k)
				if rx := styleFieldPrescriptionFor(k); rx != "" {
					msg += " — " + rx
				} else if match := suggest.Closest(lower, workbookCreateCellStyleFieldList, 1); len(match) > 0 && suggest.Levenshtein(lower, match[0]) <= 2 {
					msg += fmt.Sprintf(" — did you mean %q?", match[0])
				}
				msg += "; supported: " + strings.Join(workbookCreateCellStyleFieldList, ", ")
				return nil, common.ValidationErrorf("%s", msg)
			}
			cellStyle[k] = v
		}
	}
	if len(cellStyle) > 0 {
		out["cell_styles"] = cellStyle
	}
	return out, nil
}

// workbookCreateCellStyleFieldList is what a caller may WRITE in a cell_styles
// item, in display order for the unknown-field hint — the canonical scalar
// vocabulary (workbookCreateCellStyleField) plus the two border carriers.
// "border" is the documented four-sides shorthand rather than a field the
// switch above ever sees: foldBorderFamilyAliases folds it into border_styles
// first. It belongs in this list because the list answers "what may I write",
// not "what survives normalization".
var workbookCreateCellStyleFieldList = []string{
	"font_color", "font_family", "font_size", "font_weight", "font_style", "font_line",
	"background_color", "horizontal_alignment", "vertical_alignment",
	"number_format", "word_wrap", "border", "border_styles",
}

func workbookCreateCellStyleField(name string) bool {
	switch name {
	case "font_color", "font_family", "font_size", "font_weight", "font_style", "font_line",
		"background_color", "horizontal_alignment", "vertical_alignment",
		"number_format", "word_wrap":
		return true
	default:
		return false
	}
}

// validateWorkbookBorderStyles checks a border_styles object's internal shape
// (per-side style/weight enums + color) at parse time. --styles is on
// parseJSONFlagSkip so it bypasses the generic schema validator; this keeps
// border errors caught in the CLI (mirroring +cells-set-style) rather than being
// passed straight through to the backend.
func validateWorkbookBorderStyles(m map[string]interface{}, path string) error {
	for side, raw := range m {
		switch side {
		case "top", "bottom", "left", "right":
		default:
			return common.ValidationErrorf("%s.border_styles.%s is not a valid side (want top/bottom/left/right; a horizontal line is the top/bottom side of its range, a vertical line is left/right)", path, side)
		}
		spec, ok := raw.(map[string]interface{})
		if !ok {
			return common.ValidationErrorf("%s.border_styles.%s must be a JSON object", path, side)
		}
		for k, v := range spec {
			switch k {
			case "style":
				if s, _ := v.(string); !workbookBorderStyleEnum(s) {
					return common.ValidationErrorf("%s.border_styles.%s.style %q is invalid (want solid/dashed/dotted/double/none)", path, side, s)
				}
			case "weight":
				if w, _ := v.(string); w != "thin" && w != "medium" && w != "thick" {
					return common.ValidationErrorf("%s.border_styles.%s.weight %q is invalid (want thin/medium/thick)", path, side, w)
				}
			case "color":
				if _, ok := v.(string); !ok {
					return common.ValidationErrorf("%s.border_styles.%s.color must be a string", path, side)
				}
			default:
				return common.ValidationErrorf("%s.border_styles.%s.%s is not valid (want style/weight/color)", path, side, k)
			}
		}
	}
	return nil
}

func workbookBorderStyleEnum(s string) bool {
	switch s {
	case "solid", "dashed", "dotted", "double", "none":
		return true
	}
	return false
}

func workbookCreateStyleDimensions(styles *workbookCreateStylePayload, baseCol, baseRow int) (rows, cols int) {
	if styles == nil {
		return 0, 0
	}
	expandCellRange := func(rng string) {
		startCol, startRow, endCol, endRow, err := workbookCreateStyleRangeBounds(rng)
		if err != nil {
			return
		}
		if startCol < baseCol || startRow < baseRow {
			return
		}
		if endCol-baseCol+1 > cols {
			cols = endCol - baseCol + 1
		}
		if endRow-baseRow+1 > rows {
			rows = endRow - baseRow + 1
		}
	}
	expandRowRange := func(rng string) {
		dim, _, endIdx, err := parseA1Range(rng)
		if err != nil || dim != "row" || endIdx < baseRow {
			return
		}
		if endIdx-baseRow+1 > rows {
			rows = endIdx - baseRow + 1
		}
	}
	expandColRange := func(rng string) {
		dim, _, endIdx, err := parseA1Range(rng)
		if err != nil || dim != "column" || endIdx < baseCol {
			return
		}
		if endIdx-baseCol+1 > cols {
			cols = endIdx - baseCol + 1
		}
	}
	for _, op := range styles.CellStyles {
		expandCellRange(op.Range)
	}
	// cell_merges / row_sizes / col_sizes also contribute to the write extent —
	// without this, a style-only payload (e.g. just cell_merges) would compute
	// extent 0 and the Execute path would skip writeTypedSheets entirely,
	// silently dropping the visual ops.
	for _, op := range styles.CellMerges {
		expandCellRange(op.Range)
	}
	for _, op := range styles.RowSizes {
		expandRowRange(op.Range)
	}
	for _, op := range styles.ColSizes {
		expandColRange(op.Range)
	}
	return rows, cols
}

// matrixDimensionsForStyles projects the padded matrix size without allocating
// it. Only cell_styles contribute; merges and row/column sizes use separate API
// calls. Ranges up/left of the write anchor are left for the caller to reject.
func matrixDimensionsForStyles(rows, cols int, styles *workbookCreateStylePayload, baseCol, baseRow int) (int, int) {
	if styles == nil {
		return rows, cols
	}
	for _, op := range styles.CellStyles {
		startCol, startRow, endCol, endRow, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil || startCol < baseCol || startRow < baseRow {
			continue // unparsable, or up/left of the anchor: not paddable
		}
		if endCol-baseCol+1 > cols {
			cols = endCol - baseCol + 1
		}
		if endRow-baseRow+1 > rows {
			rows = endRow - baseRow + 1
		}
	}
	return rows, cols
}

// padMatrixForStyles grows the matrix down and right to the projected style
// extent, appending empty cells that cell_styles can mutate in place.
func padMatrixForStyles(rows [][]interface{}, styles *workbookCreateStylePayload, baseCol, baseRow int) [][]interface{} {
	needCols := 0
	if len(rows) > 0 {
		needCols = len(rows[0])
	}
	needRows, needCols := matrixDimensionsForStyles(len(rows), needCols, styles, baseCol, baseRow)
	// Widen existing rows to needCols.
	for r := range rows {
		for len(rows[r]) < needCols {
			rows[r] = append(rows[r], map[string]interface{}{})
		}
	}
	// Append full empty rows to reach needRows.
	for len(rows) < needRows {
		row := make([]interface{}, needCols)
		for c := range row {
			row[c] = map[string]interface{}{}
		}
		rows = append(rows, row)
	}
	return rows
}

// checkStylesAnchors rejects cell_styles ranges whose top-left falls left of /
// above the sheet's write anchor — matrix padding cannot reach them, so the
// write phase would fail. Validate-time twin of the bounds check inside
// applyWorkbookCreateStylesToMatrix: running it BEFORE the workbook-create API
// call means the failure cannot strand an orphan workbook (live-verified: a
// start_cell B2 payload with a cell_styles range A1 used to create the
// workbook and then fail the fill). Anchor / range parse errors are skipped —
// the payload and styles parsers already report those with richer context.
//
// For append mode against a sheet that may already hold data, only the COLUMN
// is checked at Validate time: the contract ignores start_cell's row (the base
// row is resolved from the sheet's existing data at execute time), so
// comparing style rows against the ignored static row misfires — data ending
// at row 5 with start_cell B10 appends at row 6, making a B6 style legal even
// though it sits "above B10". newSheets says every payload sheet writes into a
// KNOWN-EMPTY sheet (+workbook-create: the workbook is being created), where
// append resolves its base row to the static anchor — the row check then
// applies to append too. For +table-put (newSheets false) the missing-target
// case is covered pre-creation by checkSheetStyleAnchors in writeTypedSheets;
// existing sheets keep the write-phase check with the real base row.
func checkStylesAnchors(payload *tablePayload, styles *workbookCreateSheetStyles, newSheets bool) error {
	if payload == nil || styles == nil {
		return nil
	}
	for i := range payload.Sheets {
		s := &payload.Sheets[i]
		checkRow := newSheets || s.Mode != "append"
		if err := checkSheetStyleAnchors(s, styles.styleFor(i), checkRow); err != nil {
			return err
		}
	}
	return nil
}

// checkSheetStyleAnchors is the per-sheet core of checkStylesAnchors. It is
// also called by writeTypedSheets (with checkRow true) right before CREATING a
// missing target sheet: the fresh sheet is empty, so even append resolves its
// base row to the static anchor — checking first keeps a bad style range from
// stranding a newly created empty sheet behind a "no sheets were written"
// failure.
func checkSheetStyleAnchors(s *tableSheetSpec, sp *workbookCreateStylePayload, checkRow bool) error {
	if sp == nil {
		return nil
	}
	_, col0, row0, err := sheetAnchor(s)
	if err != nil {
		return nil //nolint:nilerr // a malformed start_cell is the payload parser's to report with row/column context; this check only compares bounds
	}
	for j, op := range sp.CellStyles {
		startCol, startRow, _, _, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			continue
		}
		if startCol < col0 {
			return common.ValidationErrorf("--styles for sheet %q[%d].range %q starts left of the write range (its column must be at or after %s)",
				s.Name, j, op.Range, columnIndexToLetter(col0))
		}
		if checkRow && startRow < row0 {
			return common.ValidationErrorf("--styles for sheet %q[%d].range %q starts outside the write range (its top-left must be at or after %s%d)",
				s.Name, j, op.Range, columnIndexToLetter(col0), row0+1)
		}
	}
	return nil
}

// applyWorkbookCreateStylesToMatrix pads the matrix to cover the cell_styles
// ranges (see padMatrixForStyles), merges each op's style into the covered
// cells, and returns the padded matrix. A range that starts left of / above the
// write anchor can't be padded to and is rejected.
func applyWorkbookCreateStylesToMatrix(rows [][]interface{}, styles *workbookCreateStylePayload, baseCol, baseRow int, label string) ([][]interface{}, error) {
	if styles == nil {
		return rows, nil
	}
	rows = padMatrixForStyles(rows, styles, baseCol, baseRow)
	for i, op := range styles.CellStyles {
		startCol, startRow, endCol, endRow, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			return rows, common.ValidationErrorf("%s[%d].range %q: %v", label, i, op.Range, err)
		}
		// After padding, the matrix reaches every range that starts at or after
		// the anchor; a start left of / above it can't be covered. The endRow /
		// endCol checks stay as a defensive backstop (padding should have made
		// them unreachable).
		if startCol < baseCol || startRow < baseRow || len(rows) == 0 ||
			endRow-baseRow >= len(rows) || endCol-baseCol >= len(rows[0]) {
			return rows, common.ValidationErrorf("%s[%d].range %q starts outside the write range (its top-left must be at or after %s%d)",
				label, i, op.Range,
				columnIndexToLetter(baseCol), baseRow+1)
		}
		for r := startRow - baseRow; r <= endRow-baseRow; r++ {
			for c := startCol - baseCol; c <= endCol-baseCol; c++ {
				mergeWorkbookCreateStyle(rows[r][c], op.Style)
			}
		}
	}
	return rows, nil
}

func appendWorkbookCreateVisualOpsDryRun(dry *common.DryRunAPI, token, sheetID, sheetName string, styles *workbookCreateStylePayload) {
	if dry == nil || styles == nil {
		return
	}
	for _, op := range workbookCreateVisualOps(styles) {
		input, toolName := workbookCreateVisualOpInput(token, sheetID, sheetName, op)
		if toolName == "" {
			continue
		}
		wireBody, _ := buildToolBody(toolName, input)
		dry.POST(toolInvokePath(token, ToolKindWrite)).
			Desc(fmt.Sprintf("apply %s", op.describe())).
			Body(wireBody)
	}
}

func applyWorkbookCreateVisualOps(ctx context.Context, runtime *common.RuntimeContext, token, sheetID string, styles *workbookCreateStylePayload) error {
	if styles == nil {
		return nil
	}
	for _, op := range workbookCreateVisualOps(styles) {
		input, toolName := workbookCreateVisualOpInput(token, sheetID, "", op)
		if toolName == "" {
			continue
		}
		if _, err := callTool(ctx, runtime, token, ToolKindWrite, toolName, input); err != nil {
			// callTool already returns a typed error; pass it through unchanged
			// (re-wrapping would downgrade its classification) and attach the
			// failing op as a recovery hint when one isn't already set.
			if p, ok := errs.ProblemOf(err); ok {
				if p.Hint == "" {
					p.Hint = fmt.Sprintf("failed while applying %s", op.describe())
				}
				return err
			}
			return errs.NewInternalError(errs.SubtypeUnknown, "%s failed", op.describe()).WithCause(err)
		}
	}
	return nil
}

func workbookCreateVisualOps(styles *workbookCreateStylePayload) []workbookCreateStyleOp {
	if styles == nil {
		return nil
	}
	ops := make([]workbookCreateStyleOp, 0, len(styles.CellMerges)+len(styles.RowSizes)+len(styles.ColSizes)+2)
	for _, op := range styles.CellMerges {
		ops = append(ops, workbookCreateStyleOp{Kind: "cell_merge", Range: op.Range, MergeType: op.MergeType})
	}
	for _, op := range styles.RowSizes {
		ops = append(ops, workbookCreateStyleOp{Kind: "row_size", Range: op.Range, ResizeType: op.ResizeType, Size: op.Size})
	}
	for _, op := range styles.ColSizes {
		ops = append(ops, workbookCreateStyleOp{Kind: "col_size", Range: op.Range, ResizeType: op.ResizeType, Size: op.Size})
	}
	if styles.Freeze != nil {
		ops = append(ops, workbookCreateStyleOp{Kind: "freeze", FreezeRows: styles.Freeze.Rows, FreezeCols: styles.Freeze.Cols})
	}
	return ops
}

type workbookCreateStyleOp struct {
	Kind       string
	Range      string
	MergeType  string
	ResizeType string
	Size       int
	FreezeRows int
	FreezeCols int
}

// describe renders the op for dry-run text and failure hints. freeze carries
// counts instead of a range, so "%s %s" of kind and range would trail a blank.
func (op workbookCreateStyleOp) describe() string {
	if op.Kind != "freeze" {
		return op.Kind + " " + op.Range
	}
	if op.FreezeRows == 0 && op.FreezeCols == 0 {
		return "unfreeze"
	}
	parts := make([]string, 0, 2)
	if op.FreezeRows > 0 {
		parts = append(parts, fmt.Sprintf("rows=%d", op.FreezeRows))
	}
	if op.FreezeCols > 0 {
		parts = append(parts, fmt.Sprintf("cols=%d", op.FreezeCols))
	}
	return "freeze " + strings.Join(parts, " ")
}

func workbookCreateVisualOpInput(token, sheetID, sheetName string, op workbookCreateStyleOp) (map[string]interface{}, string) {
	// Every caller names the sheet through the selector, so a "Sheet!" prefix
	// left on the range would be a duplicate the backend range parser rejects.
	op.Range = stripSheetPrefix(op.Range)
	switch op.Kind {
	case "cell_merge":
		input := map[string]interface{}{
			"excel_id":   token,
			"range":      op.Range,
			"operation":  "merge",
			"merge_type": op.MergeType,
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		return input, "merge_cells"
	case "row_size", "col_size":
		input := map[string]interface{}{
			"excel_id": token,
			"range":    op.Range,
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		block := map[string]interface{}{"type": op.ResizeType}
		if op.ResizeType == "pixel" {
			block["value"] = op.Size
		}
		if op.Kind == "row_size" {
			input["resize_height"] = block
		} else {
			input["resize_width"] = block
		}
		return input, "resize_range"
	case "freeze":
		// Both axes travel in ONE operation because the backend treats freeze as
		// full-state replacement, not a per-axis patch: verified 07-31 on a live
		// sheet — freezing 1 row then 2 columns in two calls ends at
		// frozen_row_count 0 / frozen_column_count 2, the second call having
		// silently dropped the first axis. One call carrying both lands 1/2.
		// By the same rule an omitted axis is unfrozen, which is what a
		// declarative --styles spec should mean. An all-zero target is the bare
		// "unfreeze" operation, which carries no counts and clears everything —
		// the same request +dim-freeze --rows 0 --cols 0 sends.
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "unfreeze",
		}
		if op.FreezeRows > 0 || op.FreezeCols > 0 {
			input["operation"] = "freeze"
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		if op.FreezeRows > 0 {
			input["freeze_rows"] = op.FreezeRows
		}
		if op.FreezeCols > 0 {
			input["freeze_columns"] = op.FreezeCols
		}
		return input, "modify_sheet_structure"
	default:
		return nil, ""
	}
}

func workbookCreateStyleRangeBounds(rangeStr string) (startCol, startRow, endCol, endRow int, err error) {
	if idx := strings.Index(rangeStr, "!"); idx >= 0 {
		rangeStr = rangeStr[idx+1:]
	}
	rangeStr = strings.TrimSpace(rangeStr)
	if rangeStr == "" {
		return 0, 0, 0, 0, fmt.Errorf("empty range") //nolint:forbidigo // intermediate error; callers wrap it into a typed validation error with flag/param context
	}
	parts := strings.SplitN(rangeStr, ":", 2)
	if len(parts) == 1 {
		col, row, ok := splitCellRef(parts[0])
		if !ok {
			return 0, 0, 0, 0, fmt.Errorf("invalid cell ref %q", parts[0]) //nolint:forbidigo // intermediate error; callers wrap it into a typed validation error with flag/param context
		}
		return col, row, col, row, nil
	}
	startCol, startRow, ok1 := splitCellRef(parts[0])
	endCol, endRow, ok2 := splitCellRef(parts[1])
	if !ok1 || !ok2 {
		return 0, 0, 0, 0, fmt.Errorf("unsupported range form %q (need rectangular A1:B2)", rangeStr) //nolint:forbidigo // intermediate error; callers wrap it into a typed validation error with flag/param context
	}
	if endRow < startRow || endCol < startCol {
		return 0, 0, 0, 0, fmt.Errorf("end %q must be at or after start %q", parts[1], parts[0]) //nolint:forbidigo // intermediate error; callers wrap it into a typed validation error with flag/param context
	}
	return startCol, startRow, endCol, endRow, nil
}

// mergeWorkbookCreateStyle merges one cell_styles op's style map into a cell.
// cell_styles / border_styles are nested submaps: they are deep-merged one level
// (field-wise, last write wins) so overlapping cell_styles ops accumulate fields
// rather than the later op's submap wholesale-replacing the earlier one. A fresh
// submap is allocated each merge so the op.Style shared across the range's cells
// is never mutated.
func mergeWorkbookCreateStyle(cell interface{}, style map[string]interface{}) {
	if len(style) == 0 {
		return
	}
	m, ok := cell.(map[string]interface{})
	if !ok {
		return
	}
	for k, v := range style {
		if k == "cell_styles" || k == "border_styles" {
			if incoming, ok := v.(map[string]interface{}); ok {
				merged := map[string]interface{}{}
				if existing, ok := m[k].(map[string]interface{}); ok {
					for sk, sv := range existing {
						merged[sk] = sv
					}
				}
				for sk, sv := range incoming {
					merged[sk] = sv
				}
				m[k] = merged
				continue
			}
		}
		m[k] = v
	}
}

// ─── +workbook-export (legacy OAPI, cli_status: cli-only) ────────────
//
// Drives the three-step export flow against the classic drive endpoints:
// create export task → poll task status → optional binary download.
// Not exposed as an MCP tool.

// WorkbookExport drives the three-step export flow: create task → poll →
// optionally download. CSV mode requires --sheet-id (the API exports one
// sheet at a time as csv).
var WorkbookExport = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-export",
	Description: "Export a spreadsheet to xlsx or a single sheet to csv (async + poll + optional download).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read", "docs:document:export", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+workbook-export"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		if err := errLocalOfficeExportUnsupported(token); err != nil {
			return err
		}
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		if ext == "csv" && strings.TrimSpace(runtime.Str("sheet-id")) == "" {
			return common.ValidationErrorf("--sheet-id is required when --file-extension=csv")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		p, _ := workbookExportParams(runtime)
		p.OutputDir = strings.TrimSpace(runtime.Str("output-path"))
		return drive.PlanExportDryRun(runtime, p)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		p, err := workbookExportParams(runtime)
		if err != nil {
			return err
		}
		// workbookExportParams resolves --url network-free (DryRun shares it); a
		// /wiki/ URL carries a node_token that needs the get_node step only
		// Execute may take, so re-resolve the token here.
		if p.Token, err = resolveSpreadsheetTokenExec(runtime); err != nil {
			return err
		}
		// Re-check after the wiki hop: Validate only saw the node_token.
		if err := errLocalOfficeExportUnsupported(p.Token); err != nil {
			return err
		}
		applyWorkbookOutputPath(&p, runtime.FileIO(), runtime.Str("output-path"))
		return drive.RunExport(ctx, runtime, p)
	},
	Tips: []string{
		"Polls for a bounded window; if the export is still running it returns a resume reference instead of blocking. Pass --output-path to download the file once ready (omit it to only create the export task and get the file token back).",
	},
}

// errLocalOfficeExportUnsupported rejects an export whose target is an Office
// workbook rather than a Lark spreadsheet (common.IsLocalOfficeToken). Drive's export
// task only produces artifacts for native Lark documents, so these tokens fail
// on the backend — late, after the create + poll round trips, with an opaque
// message. Refuse up front and say why instead.
//
// The two token classes need different recovery, and conflating them hands the
// caller an action they cannot take:
//
//   - a "local_office_" / "fake_office_" prefix is a synthetic token the client
//     mints for a file opened from the user's own disk — the workbook is
//     already a local file, so there is nothing to fetch;
//   - an interleaved OFL0X token is an Office file stored in Lark (uploaded or
//     imported, and possibly only shared with the caller). There may be no
//     local copy at all, so the recovery is to download the stored file, or to
//     convert it into a real Lark spreadsheet that export does support.
func errLocalOfficeExportUnsupported(token string) error {
	if !common.IsLocalOfficeToken(token) {
		return nil
	}
	if isLocallyOpenedOfficeToken(token) {
		return errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"%s is a locally opened Office file, not a Lark spreadsheet — it cannot be exported", token).
			WithHint("This workbook is already a file on your own disk: use that file directly, no export needed. " +
				"To get a Lark spreadsheet you can export later, upload it first with `lark-cli sheets +workbook-import --file <path>`.")
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition,
		"%s is an Office file stored in Lark, not a Lark spreadsheet — export only produces artifacts for native Lark documents", token).
		WithHint(fmt.Sprintf("Download the stored file as-is with `lark-cli drive +download --file-token %s`. "+
			"If you need a Lark spreadsheet (to export it, or to edit it with the sheets commands), convert it first: "+
			"download it, then `lark-cli sheets +workbook-import --file <path>`.", token))
}

// isLocallyOpenedOfficeToken reports whether the token is one of the synthetic
// prefixes the client mints for a workbook opened from local disk, as opposed
// to an Office file that lives in Lark. Both are local-office tokens to
// common.IsLocalOfficeToken; only this class implies the caller holds the file.
func isLocallyOpenedOfficeToken(token string) bool {
	return strings.HasPrefix(token, common.FakeOfficeTokenPrefix) || strings.HasPrefix(token, common.LocalOfficeTokenPrefix)
}

// workbookExportParams builds the shared drive export request for
// +workbook-export: spreadsheet token + sheet locator, pinned to type=sheet.
// workbook-export has always overwritten the target, so Overwrite is set. The
// --output-path → OutputDir/FileName split (which needs a Stat) is applied
// separately by applyWorkbookOutputPath so Validate/DryRun stay I/O-free.
func workbookExportParams(runtime *common.RuntimeContext) (drive.ExportParams, error) {
	token, err := resolveSpreadsheetToken(runtime)
	if err != nil {
		return drive.ExportParams{}, err
	}
	ext := runtime.Str("file-extension")
	if ext == "" {
		ext = "xlsx"
	}
	return drive.ExportParams{
		Token:         token,
		DocType:       "sheet",
		FileExtension: ext,
		SubID:         strings.TrimSpace(runtime.Str("sheet-id")),
		Overwrite:     true,
	}, nil
}

// applyWorkbookOutputPath maps the single --output-path flag onto the drive
// export OutputDir/FileName pair, preserving the legacy behavior: empty = no
// download (return the ready file token only); an existing directory = download
// into it under the server-provided name; otherwise treat it as a file path and
// split into dir + base name.
func applyWorkbookOutputPath(p *drive.ExportParams, fio fileio.FileIO, outputPath string) {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return
	}
	if info, err := fio.Stat(outputPath); err == nil && info.IsDir() {
		p.OutputDir = outputPath
		return
	}
	p.OutputDir = filepath.Dir(outputPath)
	p.FileName = filepath.Base(outputPath)
}

// lookupSheetIndex finds a sub-sheet by id or name and returns its canonical
// id + current 0-based index. Caller is responsible for ensuring at least one
// of sheetID/sheetName is non-empty.
func lookupSheetIndex(ctx context.Context, runtime *common.RuntimeContext, token, sheetID, sheetName string) (resolvedID string, index int, err error) {
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
		"excel_id": token,
	})
	if err != nil {
		return "", 0, err
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		return "", 0, errs.NewInternalError(errs.SubtypeInvalidResponse, "get_workbook_structure returned non-object output")
	}
	sheets, _ := m["sheets"].([]interface{})
	for _, raw := range sheets {
		sm, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := sm["sheet_id"].(string)
		// get_workbook_structure surfaces the sub-sheet's display name as
		// "title"; older/alt payloads use "sheet_name". Match either so a
		// --sheet-name lookup resolves regardless of the field name.
		name, _ := sm["sheet_name"].(string)
		if name == "" {
			name, _ = sm["title"].(string)
		}
		if (sheetID != "" && id == sheetID) || (sheetName != "" && name == sheetName) {
			idx, ok := util.ToFloat64(sm["index"])
			if !ok {
				return "", 0, errs.NewInternalError(errs.SubtypeInvalidResponse, "sheet entry missing index field")
			}
			return id, int(idx), nil
		}
	}
	target := sheetID
	if target == "" {
		target = sheetName
	}
	return "", 0, errs.NewValidationError(errs.SubtypeFailedPrecondition, "sheet %q not found in workbook", target)
}

// lookupFirstSheetID returns the sheet_id of the sub-sheet at index 0 (the
// default sheet of a freshly created workbook). Used by +workbook-create to
// target the initial-fill set_cell_range write — set_cell_range rejects an
// empty sheet selector ("sheet_id or sheet_name is required"), and the v3
// create-spreadsheet response does not echo the default sheet's id.
func lookupFirstSheetID(ctx context.Context, runtime *common.RuntimeContext, token string) (string, error) {
	out, err := callTool(ctx, runtime, token, ToolKindRead, "get_workbook_structure", map[string]interface{}{
		"excel_id": token,
	})
	if err != nil {
		return "", err
	}
	m, ok := out.(map[string]interface{})
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "get_workbook_structure returned non-object output")
	}
	sheets, _ := m["sheets"].([]interface{})
	bestID := ""
	bestIdx := -1
	for _, raw := range sheets {
		sm, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := sm["sheet_id"].(string)
		if id == "" {
			continue
		}
		idx, ok := util.ToFloat64(sm["index"])
		if !ok {
			// No index field — fall back to first encountered sheet.
			if bestID == "" {
				bestID = id
			}
			continue
		}
		if bestIdx < 0 || int(idx) < bestIdx {
			bestIdx = int(idx)
			bestID = id
		}
	}
	if bestID == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "get_workbook_structure returned no sheets")
	}
	return bestID, nil
}

// ─── +workbook-import (reuses drive import core, cli_status: cli-only) ──
//
// Imports a local xlsx/xls/csv file as a brand-new spreadsheet. The full
// upload → create-task → poll flow is the shared drive import core
// (drive.RunImport); this shortcut only pins the target type to "sheet",
// omits the bitable-only --target-token, and — because spreadsheet source
// files are routinely misnamed (an .xlsx exported/renamed to .xls, etc.) —
// sniffs the file's real container so the drive import backend receives the
// true file_extension instead of failing with a cryptic
// "xml_version_not_support". Symmetric with +workbook-export. Not exposed as
// an MCP tool.

// WorkbookImport imports a local spreadsheet file as a new Feishu spreadsheet
// by delegating to the shared drive import core with type fixed to "sheet".
var WorkbookImport = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-import",
	Description: "Import a local xlsx/xls/csv file as a new spreadsheet (async + poll). Reuses the drive import core with type fixed to sheet.",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload", "docs:document:import"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+workbook-import"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params, err := workbookImportParams(runtime)
		if err != nil {
			return err
		}
		return drive.ValidateImport(params)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params, err := workbookImportParams(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dry := drive.PlanImportDryRun(runtime, params)
		if note := workbookImportMislabelNote(params); note != "" {
			dry.Desc(note)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		params, err := workbookImportParams(runtime)
		if err != nil {
			return err
		}
		// The corrected extension changes what the backend was asked to build,
		// so it rides out in the result as input_corrections rather than on
		// stderr, where a successful import looked like a failed one.
		params.InputCorrections = workbookImportCorrections(params)
		return drive.RunImport(ctx, runtime, params)
	},
}

// workbookImportParams builds the drive import request for +workbook-import,
// pinning DocType to "sheet". The bitable-only --target-token is intentionally
// not exposed here — use drive +import for non-sheet import targets. It also
// resolves a corrected file extension via content sniffing (see
// correctedWorkbookExtension) and surfaces it through ImportParams.FileExtension.
func workbookImportParams(runtime *common.RuntimeContext) (drive.ImportParams, error) {
	file := runtime.Str("file")
	params := drive.ImportParams{
		File:        file,
		DocType:     "sheet",
		FolderToken: runtime.Str("folder-token"),
		Name:        runtime.Str("name"),
	}
	ext, err := correctedWorkbookExtension(runtime.FileIO(), file)
	if err != nil {
		return params, err
	}
	params.FileExtension = ext
	return params, nil
}

// correctedWorkbookExtension returns an override extension when the file's
// declared .xls/.xlsx suffix disagrees with its real container, "" when the
// declared suffix is correct (or the extension is not in the Excel family, or
// the file cannot yet be read). A declared Excel file whose bytes match neither
// container yields a prescriptive validation error rather than deferring to the
// backend's opaque "xml_version_not_support".
func correctedWorkbookExtension(fio fileio.FileIO, filePath string) (string, error) {
	declared := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if declared != "xls" && declared != "xlsx" {
		return "", nil
	}

	sniffed, ok := sniffWorkbookContainer(fio, filePath)
	if !ok {
		// Not readable here; let the drive core's stat/upload surface any error.
		return "", nil
	}
	switch sniffed {
	case declared:
		return "", nil
	case "xls", "xlsx":
		return sniffed, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"file %s has a .%s extension but its content is neither an OOXML (.xlsx) nor a legacy Excel (.xls) workbook; re-save it as a real .xlsx/.xls (or export to .csv) before importing",
			filePath, declared).WithParam("--file")
	}
}

// sniffWorkbookContainer inspects a file's leading magic bytes to tell an OOXML
// workbook (zip container -> .xlsx) apart from a legacy OLE2/BIFF workbook
// (compound document -> .xls). The second return value is false when the file
// cannot be read far enough to judge (open error or fewer than the four
// discriminating bytes). When true, the format is "xlsx", "xls", or "" (bytes
// matching neither container).
func sniffWorkbookContainer(fio fileio.FileIO, filePath string) (string, bool) {
	f, err := fio.Open(filePath)
	if err != nil {
		return "", false
	}
	defer f.Close()

	var head [8]byte
	n, _ := io.ReadFull(f, head[:])
	if n < 4 {
		return "", false
	}
	switch {
	case head[0] == 0x50 && head[1] == 0x4B: // "PK" -> ZIP, i.e. OOXML .xlsx
		return "xlsx", true
	case head[0] == 0xD0 && head[1] == 0xCF && head[2] == 0x11 && head[3] == 0xE0: // OLE2 compound doc -> legacy .xls
		return "xls", true
	}
	return "", true
}

// workbookImportMislabelNote returns a user-facing note when content sniffing
// overrode the declared extension, or "" when no correction was applied.
// workbookImportCorrections is the machine-readable form of
// workbookImportMislabelNote, for the executed import's result. The prose note
// stays for the dry-run preview, which is human-facing.
func workbookImportCorrections(params drive.ImportParams) []drive.ImportInputCorrection {
	declared := strings.TrimPrefix(strings.ToLower(filepath.Ext(params.File)), ".")
	if params.FileExtension == "" || params.FileExtension == declared {
		return nil
	}
	return []drive.ImportInputCorrection{{
		Field:    "file_extension",
		Declared: declared,
		Actual:   params.FileExtension,
		Reason: fmt.Sprintf("%s is named .%s but its content is a .%s workbook; imported as .%s",
			filepath.Base(params.File), declared, params.FileExtension, params.FileExtension),
	}}
}

func workbookImportMislabelNote(params drive.ImportParams) string {
	declared := strings.TrimPrefix(strings.ToLower(filepath.Ext(params.File)), ".")
	if params.FileExtension == "" || params.FileExtension == declared {
		return ""
	}
	return fmt.Sprintf("Note: %s has a mislabeled .%s extension but is actually a .%s workbook; importing it as .%s.",
		filepath.Base(params.File), declared, params.FileExtension, params.FileExtension)
}
