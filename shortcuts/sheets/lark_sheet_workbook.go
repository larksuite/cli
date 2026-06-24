// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
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
	Validate:    validateViaInput(sheetCopyInput),
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
			_, err = parseWorkbookCreateSheetStyles(runtime, payload)
			return err
		}
		// Untyped --values path: parse (and validate) --styles as a single sheet
		// style item, then synthesize --values into a type-less typed payload —
		// the same construction buildValuesPayload runs at execute time, so any
		// malformed --values / --styles is caught here before a workbook is made.
		sheetStyles, err := parseValuesSheetStyles(runtime)
		if err != nil {
			return err
		}
		if _, err := buildValuesPayload(runtime, sheetStyles); err != nil {
			return err
		}
		return nil
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
			_ = applyWorkbookCreateStylesToMatrix(matrix, sheetStyles.styleFor(i), col0, row0, fmt.Sprintf("--styles for sheet %q", s.Name))
			input := map[string]interface{}{
				"excel_id":   "<new-token>",
				"sheet_name": s.Name,
				"range":      tablePutFullRange(s, len(matrix)),
				"cells":      matrix,
			}
			wireBody, _ := buildToolBody("set_cell_range", input)
			dry.POST("/open-apis/sheet_ai/v2/spreadsheets/<new-token>/tools/invoke_write").
				Desc(fmt.Sprintf("write sheet %q (%d data rows × %d cols) via set_cell_range", s.Name, len(s.Rows), len(s.Columns))).
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
		styles, err := parseWorkbookCreateSheetStyles(runtime, payload)
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
	return &tablePayload{Sheets: []tableSheetSpec{{
		Name:    valuesSheetName,
		Mode:    "overwrite",
		Header:  &noHeader,
		Columns: cols,
		Rows:    rows,
	}}}, nil
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
	return parseWorkbookCreateStyleItem(items[0], "--styles.styles[0]")
}

// parseWorkbookCreateSheetStyles parses --styles for the typed --sheets path.
// The outer protocol is always {"styles":[...]}, and the array is aligned with
// --sheets.sheets. Each item must name the same sheet at the same index.
func parseWorkbookCreateSheetStyles(runtime flagView, payload *tablePayload) (*workbookCreateSheetStyles, error) {
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
	for i, item := range items {
		name, _ := item["name"].(string)
		if strings.TrimSpace(name) == "" {
			return nil, common.ValidationErrorf("--styles.styles[%d].name is required", i)
		}
		if name != payload.Sheets[i].Name {
			return nil, common.ValidationErrorf("--styles.styles[%d].name %q must match --sheets.sheets[%d].name %q", i, name, i, payload.Sheets[i].Name)
		}
		style, err := parseWorkbookCreateStyleItem(item, fmt.Sprintf("--styles.styles[%d]", i))
		if err != nil {
			return nil, err
		}
		out.ByIndex[i] = style
		out.ByName[name] = style
	}
	return out, nil
}

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

func parseWorkbookCreateStyleItem(item map[string]interface{}, path string) (*workbookCreateStylePayload, error) {
	payload := &workbookCreateStylePayload{}
	var err error
	if raw, ok := item["cell_styles"]; ok {
		payload.CellStyles, err = parseWorkbookCreateCellStyleOps(raw, path+".cell_styles")
		if err != nil {
			return nil, err
		}
	}
	if raw, ok := item["row_sizes"]; ok {
		payload.RowSizes, err = parseWorkbookCreateResizeOps(raw, path+".row_sizes", "row")
		if err != nil {
			return nil, err
		}
	}
	if raw, ok := item["col_sizes"]; ok {
		payload.ColSizes, err = parseWorkbookCreateResizeOps(raw, path+".col_sizes", "column")
		if err != nil {
			return nil, err
		}
	}
	if raw, ok := item["cell_merges"]; ok {
		payload.CellMerges, err = parseWorkbookCreateMergeOps(raw, path+".cell_merges")
		if err != nil {
			return nil, err
		}
	}
	if len(payload.CellStyles) == 0 && len(payload.RowSizes) == 0 && len(payload.ColSizes) == 0 && len(payload.CellMerges) == 0 {
		return nil, common.ValidationErrorf("%s must include at least one of cell_styles/row_sizes/col_sizes/cell_merges", path)
	}
	return payload, nil
}

func parseWorkbookCreateCellStyleOps(v interface{}, path string) ([]workbookCreateCellStyleOp, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, common.ValidationErrorf("%s must be an array", path)
	}
	ops := make([]workbookCreateCellStyleOp, 0, len(arr))
	for i, raw := range arr {
		op, ok := raw.(map[string]interface{})
		if !ok {
			return nil, common.ValidationErrorf("%s[%d] must be an object", path, i)
		}
		rangeStr, err := requireWorkbookCreateRange(op, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if _, _, _, _, err := workbookCreateStyleRangeBounds(rangeStr); err != nil {
			return nil, common.ValidationErrorf("%s[%d].range %q: %v", path, i, rangeStr, err)
		}
		styleObj := make(map[string]interface{}, len(op)-1)
		for k, v := range op {
			if k == "range" {
				continue
			}
			styleObj[k] = v
		}
		style, err := normalizeWorkbookCreateStyleObject(styleObj, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if len(style) == 0 {
			return nil, common.ValidationErrorf("%s[%d] must include at least one style field", path, i)
		}
		ops = append(ops, workbookCreateCellStyleOp{Range: rangeStr, Style: style})
	}
	return ops, nil
}

func parseWorkbookCreateMergeOps(v interface{}, path string) ([]workbookCreateMergeOp, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, common.ValidationErrorf("%s must be an array", path)
	}
	ops := make([]workbookCreateMergeOp, 0, len(arr))
	for i, raw := range arr {
		op, ok := raw.(map[string]interface{})
		if !ok {
			return nil, common.ValidationErrorf("%s[%d] must be an object", path, i)
		}
		rangeStr, err := requireWorkbookCreateRange(op, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if _, _, _, _, err := workbookCreateStyleRangeBounds(rangeStr); err != nil {
			return nil, common.ValidationErrorf("%s[%d].range %q: %v", path, i, rangeStr, err)
		}
		mergeType := "all"
		if raw, ok := op["merge_type"]; ok {
			v, ok := raw.(string)
			if !ok || strings.TrimSpace(v) == "" {
				return nil, common.ValidationErrorf("%s[%d].merge_type must be a non-empty string", path, i)
			}
			mergeType = strings.TrimSpace(v)
		}
		switch mergeType {
		case "all", "rows", "columns":
		default:
			return nil, common.ValidationErrorf("%s[%d].merge_type %q is invalid (want all/rows/columns)", path, i, mergeType)
		}
		if err := rejectUnexpectedWorkbookStyleFields(op, fmt.Sprintf("%s[%d]", path, i), "range", "merge_type"); err != nil {
			return nil, err
		}
		ops = append(ops, workbookCreateMergeOp{Range: rangeStr, MergeType: mergeType})
	}
	return ops, nil
}

func parseWorkbookCreateResizeOps(v interface{}, path, dimension string) ([]workbookCreateResizeOp, error) {
	arr, ok := v.([]interface{})
	if !ok {
		return nil, common.ValidationErrorf("%s must be an array", path)
	}
	ops := make([]workbookCreateResizeOp, 0, len(arr))
	for i, raw := range arr {
		op, ok := raw.(map[string]interface{})
		if !ok {
			return nil, common.ValidationErrorf("%s[%d] must be an object", path, i)
		}
		rangeStr, err := requireWorkbookCreateRange(op, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		parsedDim, _, _, err := parseA1Range(rangeStr)
		if err != nil {
			want := "row numbers like 2:10"
			if dimension == "column" {
				want = "column letters like A:E"
			}
			return nil, common.ValidationErrorf("%s[%d].range %q must use %s: %v", path, i, rangeStr, want, err)
		}
		if parsedDim != dimension {
			want := "row numbers like 2:10"
			if dimension == "column" {
				want = "column letters like A:E"
			}
			return nil, common.ValidationErrorf("%s[%d].range %q must use %s", path, i, rangeStr, want)
		}
		resizeType, _ := op["type"].(string)
		resizeType = strings.TrimSpace(resizeType)
		if resizeType == "" {
			return nil, common.ValidationErrorf("%s[%d].type is required (pixel/standard%s)", path, i, autoSuffix(dimension))
		}
		if dimension == "column" && resizeType == "auto" {
			return nil, common.ValidationErrorf("%s[%d].type auto is rows-only", path, i)
		}
		switch resizeType {
		case "pixel", "standard", "auto":
		default:
			return nil, common.ValidationErrorf("%s[%d].type %q is invalid (want pixel/standard%s)", path, i, resizeType, autoSuffix(dimension))
		}
		size := 0
		if raw, ok := op["size"]; ok {
			n, ok := util.ToFloat64(raw)
			if !ok || n <= 0 {
				return nil, common.ValidationErrorf("%s[%d].size must be a positive number", path, i)
			}
			size = int(n)
		}
		if resizeType == "pixel" && size <= 0 {
			return nil, common.ValidationErrorf("%s[%d].type pixel requires size", path, i)
		}
		if resizeType != "pixel" && size > 0 {
			return nil, common.ValidationErrorf("%s[%d].size is only valid with type pixel", path, i)
		}
		if err := rejectUnexpectedWorkbookStyleFields(op, fmt.Sprintf("%s[%d]", path, i), "range", "type", "size"); err != nil {
			return nil, err
		}
		ops = append(ops, workbookCreateResizeOp{Range: normalizeWorkbookResizeRange(rangeStr), ResizeType: resizeType, Size: size})
	}
	return ops, nil
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
			if err := validateWorkbookBorderStyles(m, path); err != nil {
				return nil, err
			}
			out["border_styles"] = m
		case "value", "formula", "rich_text", "multiple_values", "note", "data_validation":
			return nil, common.ValidationErrorf("%s is for styles only; put content in --values or use --sheets for typed cell objects", path)
		default:
			if !workbookCreateCellStyleField(k) {
				return nil, common.ValidationErrorf("%s.%s is not a supported style field", path, k)
			}
			cellStyle[k] = v
		}
	}
	if len(cellStyle) > 0 {
		out["cell_styles"] = cellStyle
	}
	return out, nil
}

func workbookCreateCellStyleField(name string) bool {
	switch name {
	case "font_color", "font_size", "font_weight", "font_style", "font_line",
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
			return common.ValidationErrorf("%s.border_styles.%s is not a valid side (want top/bottom/left/right)", path, side)
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

func applyWorkbookCreateStylesToMatrix(rows [][]interface{}, styles *workbookCreateStylePayload, baseCol, baseRow int, label string) error {
	if styles == nil {
		return nil
	}
	for i, op := range styles.CellStyles {
		startCol, startRow, endCol, endRow, err := workbookCreateStyleRangeBounds(op.Range)
		if err != nil {
			return common.ValidationErrorf("%s[%d].range %q: %v", label, i, op.Range, err)
		}
		if startCol < baseCol || startRow < baseRow || endRow-baseRow >= len(rows) || len(rows) == 0 || endCol-baseCol >= len(rows[0]) {
			return common.ValidationErrorf("%s[%d].range %q is outside the write range %s%d:%s%d",
				label, i, op.Range,
				columnIndexToLetter(baseCol), baseRow+1,
				columnIndexToLetter(baseCol+len(rows[0])-1), baseRow+len(rows))
		}
		for r := startRow - baseRow; r <= endRow-baseRow; r++ {
			for c := startCol - baseCol; c <= endCol-baseCol; c++ {
				mergeWorkbookCreateStyle(rows[r][c], op.Style)
			}
		}
	}
	return nil
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
			Desc(fmt.Sprintf("apply %s %s", op.Kind, op.Range)).
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
					p.Hint = fmt.Sprintf("failed while applying %s on %s", op.Kind, op.Range)
				}
				return err
			}
			return errs.NewInternalError(errs.SubtypeUnknown, "%s %s failed", op.Kind, op.Range).WithCause(err)
		}
	}
	return nil
}

func workbookCreateVisualOps(styles *workbookCreateStylePayload) []workbookCreateStyleOp {
	if styles == nil {
		return nil
	}
	ops := make([]workbookCreateStyleOp, 0, len(styles.CellMerges)+len(styles.RowSizes)+len(styles.ColSizes))
	for _, op := range styles.CellMerges {
		ops = append(ops, workbookCreateStyleOp{Kind: "cell_merge", Range: op.Range, MergeType: op.MergeType})
	}
	for _, op := range styles.RowSizes {
		ops = append(ops, workbookCreateStyleOp{Kind: "row_size", Range: op.Range, ResizeType: op.ResizeType, Size: op.Size})
	}
	for _, op := range styles.ColSizes {
		ops = append(ops, workbookCreateStyleOp{Kind: "col_size", Range: op.Range, ResizeType: op.ResizeType, Size: op.Size})
	}
	return ops
}

type workbookCreateStyleOp struct {
	Kind       string
	Range      string
	MergeType  string
	ResizeType string
	Size       int
}

func workbookCreateVisualOpInput(token, sheetID, sheetName string, op workbookCreateStyleOp) (map[string]interface{}, string) {
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
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
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
		applyWorkbookOutputPath(&p, runtime.FileIO(), runtime.Str("output-path"))
		return drive.RunExport(ctx, runtime, p)
	},
	Tips: []string{
		"Polls for a bounded window; if the export is still running it returns a resume reference instead of blocking. Pass --output-path to download the file once ready (omit it to only create the export task and get the file token back).",
	},
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
// (drive.RunImport); this shortcut only pins the target type to "sheet" and
// omits the bitable-only --target-token. Symmetric with +workbook-export.
// Not exposed as an MCP tool.

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
		return drive.ValidateImport(workbookImportParams(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return drive.PlanImportDryRun(runtime, workbookImportParams(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return drive.RunImport(ctx, runtime, workbookImportParams(runtime))
	},
}

// workbookImportParams builds the drive import request for +workbook-import,
// pinning DocType to "sheet". The bitable-only --target-token is intentionally
// not exposed here — use drive +import for non-sheet import targets.
func workbookImportParams(runtime *common.RuntimeContext) drive.ImportParams {
	return drive.ImportParams{
		File:        runtime.Str("file"),
		DocType:     "sheet",
		FolderToken: runtime.Str("folder-token"),
		Name:        runtime.Str("name"),
	}
}
