// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestWorkbookShortcuts_DryRun covers all 9 lark_sheet_workbook shortcuts
// (WorkbookInfo + 8 sheet-* variants) by asserting the One-OpenAPI body
// the dry-run renders. Together they exercise every dispatch arm of
// modify_workbook_structure plus the read tool.
func TestWorkbookShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}{
		{
			name:     "+workbook-info read",
			sc:       WorkbookInfo,
			args:     []string{"--url", testURL},
			toolName: "get_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id": testToken,
			},
		},
		{
			name:     "+sheet-create with all options",
			sc:       SheetCreate,
			args:     []string{"--url", testURL, "--title", "Q1", "--index", "1", "--row-count", "300", "--col-count", "10"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":     testToken,
				"operation":    "create",
				"sheet_name":   "Q1",
				"target_index": float64(1),
				"rows":         float64(300),
				"columns":      float64(10),
			},
		},
		{
			name:     "+sheet-delete by id",
			sc:       SheetDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "delete",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-rename by name",
			sc:       SheetRename,
			args:     []string{"--url", testURL, "--sheet-name", "汇总", "--title", "Q1 汇总"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"operation":  "rename",
				"sheet_name": "汇总",
				"new_name":   "Q1 汇总",
			},
		},
		{
			name:     "+sheet-copy without explicit title",
			sc:       SheetCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "duplicate",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-copy with new title and index",
			sc:       SheetCopy,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--title", "副本", "--index", "0"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":     testToken,
				"operation":    "duplicate",
				"sheet_id":     testSheetID,
				"new_name":     "副本",
				"target_index": float64(0),
			},
		},
		{
			name:     "+sheet-hide",
			sc:       SheetHide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "hide",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-unhide",
			sc:       SheetUnhide,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "unhide",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-set-tab-color hex",
			sc:       SheetSetTabColor,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--color", "#FF0000"},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "set_tab_color",
				"sheet_id":  testSheetID,
				"tab_color": "#FF0000",
			},
		},
		{
			name:     "+sheet-set-tab-color empty clears",
			sc:       SheetSetTabColor,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--color", ""},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "set_tab_color",
				"sheet_id":  testSheetID,
				"tab_color": "",
			},
		},
		{
			name:     "+sheet-show-gridline",
			sc:       SheetShowGridline,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "show_gridline",
				"sheet_id":  testSheetID,
			},
		},
		{
			name:     "+sheet-hide-gridline",
			sc:       SheetHideGridline,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "modify_workbook_structure",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"operation": "hide_gridline",
				"sheet_id":  testSheetID,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestSheetMove_DryRunResolvePlaceholders verifies the move shortcut emits
// <resolve> placeholders for fields it would otherwise have to look up
// at execute time. DryRun must stay network-free.
func TestSheetMove_DryRunResolvePlaceholders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		args          []string
		wantSheetID   string
		wantSourceIdx interface{}
	}{
		{
			name:          "id only, no source-index → both literal + placeholder",
			args:          []string{"--url", testURL, "--sheet-id", testSheetID, "--index", "0"},
			wantSheetID:   testSheetID,
			wantSourceIdx: "<resolve>",
		},
		{
			name:          "name only → sheet_id placeholder + source_index placeholder",
			args:          []string{"--url", testURL, "--sheet-name", "汇总", "--index", "0"},
			wantSheetID:   "<resolve:汇总>",
			wantSourceIdx: "<resolve>",
		},
		{
			name:          "id + source-index → both literal",
			args:          []string{"--url", testURL, "--sheet-id", testSheetID, "--index", "0", "--source-index", "5"},
			wantSheetID:   testSheetID,
			wantSourceIdx: float64(5),
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, SheetMove, tt.args)
			input := decodeToolInput(t, body, "modify_workbook_structure")
			if got := input["sheet_id"]; got != tt.wantSheetID {
				t.Errorf("sheet_id = %#v, want %#v", got, tt.wantSheetID)
			}
			if got := input["source_index"]; got != tt.wantSourceIdx {
				t.Errorf("source_index = %#v, want %#v", got, tt.wantSourceIdx)
			}
			if got := input["target_index"]; got != float64(0) {
				t.Errorf("target_index = %#v, want 0", got)
			}
		})
	}
}

// TestSheetDelete_HighRiskWriteRequiresYes verifies the framework gate on
// high-risk-write — exit code 10 (confirmation_required) without --yes.
func TestSheetDelete_HighRiskWriteRequiresYes(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, SheetDelete, []string{"--url", testURL, "--sheet-id", testSheetID})
	requireProblem(t, err, errs.CategoryConfirmation, errs.SubtypeConfirmationRequired, "")
}

// TestWorkbook_Validation covers a few critical validation paths shared
// across the package's helpers (XOR token, XOR sheet selector, required
// flags).
func TestWorkbook_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		sc      common.Shortcut
		args    []string
		wantMsg string
		// cobraNative=true means the error originates from cobra's native
		// flag parsing (e.g. required-flag enforcement) which is not wrapped
		// into a typed errs.ValidationError, so the test falls back to a
		// substring match on err.Error().
		cobraNative bool
	}{
		{
			name:    "+workbook-info needs --url or --spreadsheet-token",
			sc:      WorkbookInfo,
			args:    []string{},
			wantMsg: "at least one of --url or --spreadsheet-token",
		},
		{
			name:        "+workbook-info rejects both url and token",
			sc:          WorkbookInfo,
			args:        []string{"--url", testURL, "--spreadsheet-token", testToken},
			wantMsg:     "mutually exclusive",
			cobraNative: true,
		},
		{
			name:    "+sheet-delete needs sheet selector",
			sc:      SheetDelete,
			args:    []string{"--url", testURL},
			wantMsg: "at least one of --sheet-id or --sheet-name",
		},
		{
			name:        "+sheet-create requires --title",
			sc:          SheetCreate,
			args:        []string{"--url", testURL},
			wantMsg:     "required flag(s) \"title\" not set",
			cobraNative: true,
		},
		{
			name:    "+sheet-create row-count over cap",
			sc:      SheetCreate,
			args:    []string{"--url", testURL, "--title", "X", "--row-count", "999999"},
			wantMsg: "--row-count must be between",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, tt.sc, append(tt.args, "--dry-run"))
			if tt.cobraNative {
				if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
					t.Errorf("error message missing %q; got=%v", tt.wantMsg, err)
				}
				return
			}
			requireValidation(t, err, tt.wantMsg)
		})
	}
}

// ─── +workbook-create / +workbook-export (legacy OAPI) ───────────────

// TestWorkbookCreate_DryRun verifies the two-step plan (create
// spreadsheet + optional set_cell_range follow-up) is rendered.
func TestWorkbookCreate_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("minimal title only", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{"--title", "MySheet"})
		if len(calls) != 1 {
			t.Fatalf("api calls = %d, want 1 (no values)", len(calls))
		}
		c := calls[0].(map[string]interface{})
		if c["url"] != "/open-apis/sheets/v3/spreadsheets" {
			t.Errorf("url = %v, want /open-apis/sheets/v3/spreadsheets", c["url"])
		}
		body, _ := c["body"].(map[string]interface{})
		if body["title"] != "MySheet" {
			t.Errorf("body.title = %v, want MySheet", body["title"])
		}
	})

	t.Run("with values → 2-step plan", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "Sales",
			"--values", `[["Name","Score"],["alice",95],["bob",88]]`,
		})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + fill)", len(calls))
		}
		fill := calls[1].(map[string]interface{})
		if !strings.Contains(fill["url"].(string), "/sheet_ai/v2/spreadsheets/") {
			t.Errorf("fill url = %v, want sheet_ai/v2 path", fill["url"])
		}
		body, _ := fill["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		if input["range"] != "A1:B3" {
			t.Errorf("fill range = %v, want A1:B3 (3 rows × 2 cols)", input["range"])
		}
	})

	t.Run("with styles merges into set_cell_range cells", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "Sales",
			"--values", `[["Name","Score"],["alice",95]]`,
			"--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1","font_weight":"bold","background_color":"#f5f5f5"},{"range":"B1","number_format":"0","border_styles":{"bottom":{"style":"solid","weight":"thin","color":"#000000"}}},{"range":"B2","font_color":"#0f7b0f"}]}]}`,
		})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + fill)", len(calls))
		}
		body, _ := calls[1].(map[string]interface{})["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		cells, _ := input["cells"].([]interface{})
		if len(cells) != 2 {
			t.Fatalf("cells rows = %#v, want 2", input["cells"])
		}
		headerRow, _ := cells[0].([]interface{})
		firstHeader, _ := headerRow[0].(map[string]interface{})
		firstStyle, _ := firstHeader["cell_styles"].(map[string]interface{})
		if firstStyle["font_weight"] != "bold" || firstStyle["background_color"] != "#f5f5f5" {
			t.Errorf("first header style = %#v, want bold + background", firstStyle)
		}
		secondHeader, _ := headerRow[1].(map[string]interface{})
		if secondHeader["border_styles"] == nil {
			t.Errorf("second header missing border_styles: %#v", secondHeader)
		}
		secondStyle, _ := secondHeader["cell_styles"].(map[string]interface{})
		if secondStyle["number_format"] != "0" {
			t.Errorf("second header number_format = %#v, want 0", secondStyle)
		}
		dataRow, _ := cells[1].([]interface{})
		firstData, _ := dataRow[0].(map[string]interface{})
		if _, ok := firstData["cell_styles"]; ok {
			t.Errorf("null style should leave first data cell unstyled: %#v", firstData)
		}
		secondData, _ := dataRow[1].(map[string]interface{})
		secondDataStyle, _ := secondData["cell_styles"].(map[string]interface{})
		if secondDataStyle["font_color"] != "#0f7b0f" {
			t.Errorf("second data style = %#v, want font color", secondDataStyle)
		}
	})

	t.Run("cell style range can cover the whole initial range", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "Sales",
			"--values", `[["Name","Score"],["alice",95]]`,
			"--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:B2","horizontal_alignment":"center"}]}]}`,
		})
		body, _ := calls[1].(map[string]interface{})["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		raw, _ := json.Marshal(input["cells"])
		if got := strings.Count(string(raw), "horizontal_alignment"); got != 4 {
			t.Errorf("horizontal_alignment occurrences = %d, want 4 in 2x2 range; cells=%s", got, raw)
		}
	})
	t.Run("style-only payload (cell_merges) still fills and emits merge_cells", func(t *testing.T) {
		t.Parallel()
		// Previously workbookCreateStyleDimensions only counted cell_styles, so a
		// payload with only cell_merges would compute extent 0; Execute then
		// skipped writeTypedSheets entirely and the visual ops were silently
		// dropped. The dry-run plan must include the create + fill + merge_cells.
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "X",
			"--styles", `{"styles":[{"name":"Sheet1","cell_merges":[{"range":"A1:B1"}]}]}`,
		})
		if len(calls) < 3 {
			t.Fatalf("api calls = %d, want >=3 (create + fill + merge_cells); calls=%#v", len(calls), calls)
		}
		// Walk every body and look for the merge_cells tool name in the input JSON.
		sawMerge := false
		for _, c := range calls {
			body, _ := c.(map[string]interface{})["body"].(map[string]interface{})
			if body == nil {
				continue
			}
			if toolName, _ := body["tool_name"].(string); toolName == "merge_cells" {
				sawMerge = true
				break
			}
		}
		if !sawMerge {
			t.Errorf("merge_cells tool call missing from dry-run plan; calls=%#v", calls)
		}
	})
	t.Run("style-only payload (col_sizes) still fills and emits resize_range", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "X",
			"--styles", `{"styles":[{"name":"Sheet1","col_sizes":[{"range":"A:C","type":"pixel","size":120}]}]}`,
		})
		sawResize := false
		for _, c := range calls {
			body, _ := c.(map[string]interface{})["body"].(map[string]interface{})
			if body == nil {
				continue
			}
			if toolName, _ := body["tool_name"].(string); toolName == "resize_range" {
				sawResize = true
				break
			}
		}
		if !sawResize {
			t.Errorf("resize_range tool call missing from dry-run plan; calls=%#v", calls)
		}
	})
	t.Run("overlapping cell_styles deep-merge fields, no cross-cell pollution", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "X",
			"--values", `[["a","b"]]`,
			"--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1:B1","font_weight":"bold"},{"range":"B1","font_color":"#ff0000"}]}]}`,
		})
		body, _ := calls[1].(map[string]interface{})["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		cells, _ := input["cells"].([]interface{})
		row0, _ := cells[0].([]interface{})
		// B1 hit by both ops → must keep BOTH font_weight (op1) and font_color (op2).
		b1, _ := row0[1].(map[string]interface{})
		b1s, _ := b1["cell_styles"].(map[string]interface{})
		if b1s["font_weight"] != "bold" || b1s["font_color"] != "#ff0000" {
			t.Errorf("B1 should deep-merge both ops, got %#v", b1s)
		}
		// A1 hit only by op1 → must NOT be polluted by op2's font_color (shared submap).
		a1, _ := row0[0].(map[string]interface{})
		a1s, _ := a1["cell_styles"].(map[string]interface{})
		if a1s["font_color"] != nil {
			t.Errorf("A1 must not be polluted by op2, got %#v", a1s)
		}
	})
}

// TestWorkbookCreate_DataValidation rejects bad JSON shape.
func TestWorkbookCreate_DataValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"values not 2D", []string{"--title", "X", "--values", `["a","b"]`}, "must be an array"},
		{"styles not object", []string{"--title", "X", "--styles", `"bold"`}, `shaped as {"styles":[...]}`},
		{"styles missing array", []string{"--title", "X", "--styles", `{"value":"x"}`}, "--styles.styles is required"},
		{"styles item missing groups", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","value":"x"}]}`}, "must include at least one of cell_styles/row_sizes/col_sizes/cell_merges"},
		{"cell styles must be array", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":{"range":"A1","font_weight":"bold"}}]}`}, "cell_styles must be an array"},
		{"cell style needs range", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"font_weight":"bold"}]}]}`}, "range is required"},
		{"nested cell_styles rejected", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1","cell_styles":{"font_weight":"bold"}}]}]}`}, "put style fields directly"},
		{"row size needs row range", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","row_sizes":[{"range":"A1","type":"pixel","size":20}]}]}`}, "must use row numbers"},
		{"col size needs pixel size", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","col_sizes":[{"range":"A:A","type":"pixel"}]}]}`}, "requires size"},
		{"border bad style enum", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1","border_styles":{"bottom":{"style":"NONSENSE"}}}]}]}`}, `style "NONSENSE" is invalid`},
		{"border invalid side", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1","border_styles":{"diagonal":{"style":"solid"}}}]}]}`}, "not a valid side"},
		{"border bad weight", []string{"--title", "X", "--values", `[["a"]]`, "--styles", `{"styles":[{"name":"Sheet1","cell_styles":[{"range":"A1","border_styles":{"top":{"weight":"xxl"}}}]}]}`}, `weight "xxl" is invalid`},
		{"--values trailing JSON rejected", []string{"--title", "X", "--values", `[["a"]] trailing`}, "trailing data after JSON value"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := runShortcutCapturingErr(t, WorkbookCreate, append(tt.args, "--dry-run"))
			requireValidation(t, err, tt.want)
		})
	}
}

// TestWorkbookExport_DryRun verifies the export dry-run now delegates to the
// shared drive export core: a single create-task POST (poll + download are
// described inline rather than as separate api entries).
func TestWorkbookExport_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("xlsx create-task body pins type=sheet", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{"--url", testURL, "--file-extension", "xlsx"})
		if len(calls) != 1 {
			t.Fatalf("api calls = %d, want 1 (create export task)", len(calls))
		}
		create := calls[0].(map[string]interface{})
		if create["url"] != "/open-apis/drive/v1/export_tasks" {
			t.Errorf("url = %v", create["url"])
		}
		body, _ := create["body"].(map[string]interface{})
		if body["type"] != "sheet" || body["file_extension"] != "xlsx" || body["token"] != testToken {
			t.Errorf("create body = %#v", body)
		}
	})

	t.Run("csv includes sub_id from --sheet-id", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--sheet-id", "sh1",
			"--output-path", "/tmp/out.csv",
		})
		if len(calls) != 1 {
			t.Fatalf("api calls = %d, want 1", len(calls))
		}
		body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
		if body["type"] != "sheet" || body["sub_id"] != "sh1" {
			t.Errorf("csv export body = %#v (want type=sheet, sub_id=sh1)", body)
		}
	})

	t.Run("csv requires --sheet-id", func(t *testing.T) {
		t.Parallel()
		_, _, err := runShortcutCapturingErr(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--dry-run",
		})
		requireValidation(t, err, "--sheet-id is required")
	})
}

// assertInputEquals compares the decoded tool input map against the wanted
// fields. Extra fields in `got` are allowed (defaults, optional fields);
// every key in `want` must match exactly.
func assertInputEquals(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("missing input key %q (got=%#v)", k, got)
			continue
		}
		if !deepEqualJSON(gv, wv) {
			t.Errorf("input[%q] = %#v, want %#v", k, gv, wv)
		}
	}
}

// deepEqualJSON compares JSON-shaped values (post-Unmarshal) — handles
// the fact that numbers come back as float64 and maps as map[string]interface{}.
func deepEqualJSON(a, b interface{}) bool {
	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			if !deepEqualJSON(v, bv[k]) {
				return false
			}
		}
		return true
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !deepEqualJSON(av[i], bv[i]) {
				return false
			}
		}
		return true
	}
	return a == b
}
