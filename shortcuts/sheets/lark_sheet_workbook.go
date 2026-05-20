// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
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
		token, err := resolveSpreadsheetToken(runtime)
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
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("title")) == "" {
			return common.FlagErrorf("--title is required")
		}
		if n := runtime.Int("row-count"); n < 0 || n > 50000 {
			return common.FlagErrorf("--row-count must be between 0 and 50000")
		}
		if n := runtime.Int("col-count"); n < 0 || n > 200 {
			return common.FlagErrorf("--col-count must be between 0 and 200")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", sheetCreateInput(runtime, token))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", sheetCreateInput(runtime, token))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func sheetCreateInput(runtime *common.RuntimeContext, token string) map[string]interface{} {
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
	return input
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
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, _, err := resolveSheetSelector(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input := map[string]interface{}{"excel_id": token, "operation": "delete"}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input := map[string]interface{}{"excel_id": token, "operation": "delete"}
		sheetSelectorForToolInput(input, sheetID, sheetName)
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
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if strings.TrimSpace(runtime.Str("title")) == "" {
			return common.FlagErrorf("--title is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "rename",
			"new_name":  strings.TrimSpace(runtime.Str("title")),
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "rename",
			"new_name":  strings.TrimSpace(runtime.Str("title")),
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
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
			return common.FlagErrorf("--index is required")
		}
		if runtime.Int("index") < 0 {
			return common.FlagErrorf("--index must be >= 0")
		}
		if runtime.Changed("source-index") && runtime.Int("source-index") < 0 {
			return common.FlagErrorf("--source-index must be >= 0")
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
		token, err := resolveSpreadsheetToken(runtime)
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
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		_, _, err := resolveSheetSelector(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", sheetCopyInput(runtime, token, sheetID, sheetName))
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
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", sheetCopyInput(runtime, token, sheetID, sheetName))
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

func sheetCopyInput(runtime *common.RuntimeContext, token, sheetID, sheetName string) map[string]interface{} {
	input := map[string]interface{}{"excel_id": token, "operation": "duplicate"}
	sheetSelectorForToolInput(input, sheetID, sheetName)
	if t := strings.TrimSpace(runtime.Str("title")); t != "" {
		input["new_name"] = t
	}
	if runtime.Changed("index") {
		input["target_index"] = runtime.Int("index")
	}
	return input
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
			if _, err := resolveSpreadsheetToken(runtime); err != nil {
				return err
			}
			_, _, err := resolveSheetSelector(runtime)
			return err
		},
		DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
			token, _ := resolveSpreadsheetToken(runtime)
			sheetID, sheetName, _ := resolveSheetSelector(runtime)
			input := map[string]interface{}{"excel_id": token, "operation": op}
			sheetSelectorForToolInput(input, sheetID, sheetName)
			return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
			input := map[string]interface{}{"excel_id": token, "operation": op}
			sheetSelectorForToolInput(input, sheetID, sheetName)
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
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if !runtime.Changed("color") {
			return common.FlagErrorf("--color is required (empty string clears)")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "set_tab_color",
			"tab_color": runtime.Str("color"),
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		return invokeToolDryRun(token, ToolKindWrite, "modify_workbook_structure", input)
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
		input := map[string]interface{}{
			"excel_id":  token,
			"operation": "set_tab_color",
			"tab_color": runtime.Str("color"),
		}
		sheetSelectorForToolInput(input, sheetID, sheetName)
		out, err := callTool(ctx, runtime, token, ToolKindWrite, "modify_workbook_structure", input)
		if err != nil {
			return err
		}
		runtime.Out(out, nil)
		return nil
	},
}

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
	Description: "Create a new spreadsheet (optionally pre-filled with --headers and --values).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:create", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags:       flagsFor("+workbook-create"),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("title")) == "" {
			return common.FlagErrorf("--title is required")
		}
		if runtime.Str("headers") != "" {
			v, err := parseJSONFlag(runtime, "headers")
			if err != nil {
				return err
			}
			if _, ok := v.([]interface{}); !ok {
				return common.FlagErrorf("--headers must be a JSON array")
			}
		}
		if runtime.Str("values") != "" {
			v, err := parseJSONFlag(runtime, "values")
			if err != nil {
				return err
			}
			rows, ok := v.([]interface{})
			if !ok {
				return common.FlagErrorf("--values must be a JSON 2D array")
			}
			for i, r := range rows {
				if _, ok := r.([]interface{}); !ok {
					return common.FlagErrorf("--values[%d] must be an array", i)
				}
			}
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
		if runtime.Str("headers") != "" || runtime.Str("values") != "" {
			fill, _ := buildInitialFillInput(runtime)
			wireBody, _ := buildToolBody("set_cell_range", fill)
			dry.POST("/open-apis/sheet_ai/v2/spreadsheets/<new-token>/tools/invoke_write").
				Desc("fill headers + data via set_cell_range").
				Body(wireBody)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{"title": strings.TrimSpace(runtime.Str("title"))}
		if v := strings.TrimSpace(runtime.Str("folder-token")); v != "" {
			body["folder_token"] = v
		}
		data, err := runtime.CallAPI("POST", "/open-apis/sheets/v3/spreadsheets", nil, body)
		if err != nil {
			return err
		}
		ss := common.GetMap(data, "spreadsheet")
		token := common.GetString(ss, "spreadsheet_token")
		if token == "" {
			token = common.GetString(ss, "token")
		}
		if token == "" {
			return output.Errorf(output.ExitAPI, "api_error", "spreadsheet created but token missing in response")
		}

		result := map[string]interface{}{"spreadsheet": ss}

		if runtime.Str("headers") != "" || runtime.Str("values") != "" {
			fill, err := buildInitialFillInput(runtime)
			if err != nil {
				return err
			}
			fill["excel_id"] = token
			fillOut, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", fill)
			if err != nil {
				// Spreadsheet exists; surface the fill failure but keep the new
				// token in the envelope so the caller can recover or retry.
				return fmt.Errorf("spreadsheet %s created but initial fill failed: %w", token, err)
			}
			result["initial_fill"] = fillOut
		}
		runtime.Out(result, nil)
		return nil
	},
	Tips: []string{
		"--headers and --values are optional follow-up writes. They use the same set_cell_range tool as +cells-set; partial failure leaves the spreadsheet created but empty.",
	},
}

// buildInitialFillInput zips --headers + --values into a single set_cell_range
// payload writing to the first sheet starting at A1.
func buildInitialFillInput(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	var rows [][]interface{}
	if runtime.Str("headers") != "" {
		v, _ := parseJSONFlag(runtime, "headers")
		headerArr, _ := v.([]interface{})
		row := make([]interface{}, 0, len(headerArr))
		for _, h := range headerArr {
			row = append(row, map[string]interface{}{"value": h})
		}
		rows = append(rows, row)
	}
	if runtime.Str("values") != "" {
		v, _ := parseJSONFlag(runtime, "values")
		dataArr, _ := v.([]interface{})
		for _, r := range dataArr {
			cells, _ := r.([]interface{})
			row := make([]interface{}, 0, len(cells))
			for _, c := range cells {
				row = append(row, map[string]interface{}{"value": c})
			}
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}
	// Normalize rows to the same length so cells matrix is rectangular.
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], map[string]interface{}{})
		}
	}
	endCol := columnIndexToLetter(maxCols - 1)
	rangeStr := fmt.Sprintf("A1:%s%d", endCol, len(rows))
	return map[string]interface{}{
		"range":    rangeStr,
		"cells":    rows,
		"sheet_id": "", // filled in by caller if sheet_id known; otherwise server picks first sheet
	}, nil
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
			return common.FlagErrorf("--sheet-id is required when --file-extension=csv")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		body := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": ext,
		}
		if sid := strings.TrimSpace(runtime.Str("sheet-id")); sid != "" {
			body["sub_id"] = sid
		}
		dry := common.NewDryRunAPI().
			POST("/open-apis/drive/v1/export_tasks").
			Desc("create export task").
			Body(body).
			GET("/open-apis/drive/v1/export_tasks/<ticket>").
			Desc("poll task status").
			Params(map[string]interface{}{"token": token})
		if strings.TrimSpace(runtime.Str("output-path")) != "" {
			dry.GET("/open-apis/drive/v1/export_tasks/file/<file_token>/download").
				Desc("download exported file")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		body := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": ext,
		}
		if sid := strings.TrimSpace(runtime.Str("sheet-id")); sid != "" {
			body["sub_id"] = sid
		}
		taskData, err := runtime.CallAPI("POST", "/open-apis/drive/v1/export_tasks", nil, body)
		if err != nil {
			return err
		}
		ticket := common.GetString(taskData, "ticket")
		if ticket == "" {
			return output.Errorf(output.ExitAPI, "api_error", "export task created but ticket missing")
		}

		result := map[string]interface{}{
			"ticket":         ticket,
			"file_extension": ext,
		}

		// Poll up to ~30s for completion.
		var fileToken, fileName string
		for attempt := 0; attempt < 15; attempt++ {
			status, err := pollExportTask(runtime, token, ticket)
			if err != nil {
				return err
			}
			switch status.JobStatus {
			case 0: // success
				fileToken = status.FileToken
				fileName = status.FileName
				result["file_token"] = fileToken
				result["file_name"] = fileName
				result["file_size"] = status.FileSize
				attempt = 999 // break outer loop
			case 1, 2: // pending / in progress
				time.Sleep(2 * time.Second)
				continue
			default: // any non-zero status outside the in-progress window is a failure
				if status.JobErrorMsg != "" {
					return output.Errorf(output.ExitAPI, "api_error", "export task %s failed: %s", ticket, status.JobErrorMsg)
				}
				return output.Errorf(output.ExitAPI, "api_error", "export task %s failed with job_status=%d", ticket, status.JobStatus)
			}
		}
		if fileToken == "" {
			result["status"] = "polling_timeout"
			runtime.Out(result, nil)
			return nil
		}

		outPath := strings.TrimSpace(runtime.Str("output-path"))
		if outPath == "" {
			runtime.Out(result, nil)
			return nil
		}

		saved, err := downloadExportFile(ctx, runtime, fileToken, outPath, fileName)
		if err != nil {
			return err
		}
		result["saved_path"] = saved
		runtime.Out(result, nil)
		return nil
	},
	Tips: []string{
		"Polls up to ~30s (15 × 2s). For very large workbooks rerun and pass --output-path to capture the file once status flips to success.",
	},
}

type exportTaskStatus struct {
	JobStatus     int
	JobErrorMsg   string
	FileToken     string
	FileName      string
	FileSize      int64
	FileExtension string
}

func pollExportTask(runtime *common.RuntimeContext, token, ticket string) (exportTaskStatus, error) {
	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/export_tasks/%s", validate.EncodePathSegment(ticket)),
		map[string]interface{}{"token": token},
		nil,
	)
	if err != nil {
		return exportTaskStatus{}, err
	}
	result := common.GetMap(data, "result")
	if result == nil {
		return exportTaskStatus{}, output.Errorf(output.ExitAPI, "api_error", "export task %s: empty result", ticket)
	}
	js, _ := util.ToFloat64(result["job_status"])
	fs, _ := util.ToFloat64(result["file_size"])
	return exportTaskStatus{
		JobStatus:     int(js),
		JobErrorMsg:   common.GetString(result, "job_error_msg"),
		FileToken:     common.GetString(result, "file_token"),
		FileName:      common.GetString(result, "file_name"),
		FileSize:      int64(fs),
		FileExtension: common.GetString(result, "file_extension"),
	}, nil
}

func downloadExportFile(ctx context.Context, runtime *common.RuntimeContext, fileToken, outPath, preferredName string) (string, error) {
	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
	}, larkcore.WithFileDownload())
	if err != nil {
		return "", output.ErrNetwork("download failed: %s", err)
	}
	if apiResp.StatusCode >= 400 {
		return "", output.ErrNetwork("download failed: HTTP %d: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}
	target := outPath
	if info, statErr := runtime.FileIO().Stat(outPath); statErr == nil && info.IsDir() {
		name := strings.TrimSpace(preferredName)
		if name == "" {
			name = client.ResolveFilename(apiResp)
		}
		target = filepath.Join(outPath, name)
	}
	if _, err := runtime.FileIO().Save(target, fileio.SaveOptions{
		ContentType:   apiResp.Header.Get("Content-Type"),
		ContentLength: int64(len(apiResp.RawBody)),
	}, strings.NewReader(string(apiResp.RawBody))); err != nil {
		return "", common.WrapSaveErrorByCategory(err, "io")
	}
	resolved, _ := runtime.FileIO().ResolvePath(target)
	if resolved == "" {
		resolved = target
	}
	return resolved, nil
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
		return "", 0, output.Errorf(output.ExitAPI, "tool_output", "get_workbook_structure returned non-object output")
	}
	sheets, _ := m["sheets"].([]interface{})
	for _, raw := range sheets {
		sm, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := sm["sheet_id"].(string)
		name, _ := sm["sheet_name"].(string)
		if (sheetID != "" && id == sheetID) || (sheetName != "" && name == sheetName) {
			idx, ok := util.ToFloat64(sm["index"])
			if !ok {
				return "", 0, output.Errorf(output.ExitAPI, "tool_output", "sheet entry missing index field")
			}
			return id, int(idx), nil
		}
	}
	target := sheetID
	if target == "" {
		target = sheetName
	}
	return "", 0, output.Errorf(output.ExitAPI, "not_found", fmt.Sprintf("sheet %q not found in workbook", target))
}
