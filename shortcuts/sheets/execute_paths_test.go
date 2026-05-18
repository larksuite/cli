// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// TestExecute_WorkbookInfo_Happy stubs the invoke_read endpoint and
// verifies the shortcut decodes the JSON-string output, surfaces it as
// envelope data, and finishes without error.
func TestExecute_WorkbookInfo_Happy(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"sh1","title":"Sheet1","row_count":1000,"column_count":26,"index":0}]}`)
	out, err := runShortcutWithStubs(t, WorkbookInfo, []string{"--url", testURL}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	sheets, _ := data["sheets"].([]interface{})
	if len(sheets) != 1 {
		t.Fatalf("sheets len = %d, want 1", len(sheets))
	}
	sheet, _ := sheets[0].(map[string]interface{})
	if sheet["sheet_id"] != "sh1" || sheet["title"] != "Sheet1" {
		t.Errorf("unexpected sheet: %#v", sheet)
	}
}

// TestExecute_WorkbookInfo_ToolError surfaces a non-zero code in the
// envelope shape and asserts CLI returns an error envelope.
func TestExecute_WorkbookInfo_ToolError(t *testing.T) {
	t.Parallel()
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
		Body: map[string]interface{}{
			"code": 1310201,
			"msg":  "spreadsheet not found",
			"data": map[string]interface{}{},
		},
	}
	stdout, stderr, err := func() (string, string, error) {
		parent, stdout, stderr, reg := newTestRig(t, WorkbookInfo)
		reg.Register(stub)
		parent.SetArgs([]string{"+workbook-info", "--url", testURL})
		err := parent.Execute()
		return stdout.String(), stderr.String(), err
	}()
	if err == nil {
		t.Fatalf("expected non-zero code to surface as error; stdout=%s stderr=%s", stdout, stderr)
	}
	combined := stdout + stderr + err.Error()
	if !strings.Contains(combined, "1310201") && !strings.Contains(combined, "not found") {
		t.Errorf("expected error code in envelope; got=%s|%s|%v", stdout, stderr, err)
	}
}

// TestExecute_SheetMove_LookupsIndex covers the two-step path: SheetMove
// when only --sheet-name is given (and --source-index omitted) first
// reads the workbook structure to derive sheet_id + source_index, then
// posts the modify_workbook_structure call.
func TestExecute_SheetMove_LookupsIndex(t *testing.T) {
	t.Parallel()
	lookup := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"sh1","sheet_name":"汇总","index":3}]}`)
	move := toolOutputStub(testToken, "write", `{"sheet_id":"sh1"}`)
	out, err := runShortcutWithStubs(t, SheetMove,
		[]string{"--url", testURL, "--sheet-name", "汇总", "--index", "0"},
		lookup, move,
	)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	// Inspect the captured move body: source_index should be 3 (looked up),
	// not <resolve>, and sheet_id should be the resolved id.
	if move.CapturedBody == nil {
		t.Fatal("move stub didn't capture a body")
	}
	body := decodeRawEnvelopeBody(t, move.CapturedBody)
	input := decodeToolInput(t, body, "modify_workbook_structure")
	if input["sheet_id"] != "sh1" {
		t.Errorf("sheet_id = %v, want sh1 (resolved from --sheet-name)", input["sheet_id"])
	}
	if input["source_index"].(float64) != 3 {
		t.Errorf("source_index = %v, want 3 (from lookup)", input["source_index"])
	}
	if input["target_index"].(float64) != 0 {
		t.Errorf("target_index = %v, want 0", input["target_index"])
	}
}

// TestExecute_CellsGet covers a multi-range read end-to-end.
func TestExecute_CellsGet(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "read", `{"ranges":[{"range":"A1:B2","cells":[[{"value":1}]]}]}`)
	out, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--ranges", "A1:B2"}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if data := decodeEnvelopeData(t, out); data["ranges"] == nil {
		t.Fatalf("expected ranges in output; got=%#v", data)
	}
}

// TestExecute_CellsSet covers the write path including allow-overwrite
// override.
func TestExecute_CellsSet(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"updated_cells":2}`)
	out, err := runShortcutWithStubs(t, CellsSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B1",
		"--cells", `[[{"value":"x"},{"value":"y"}]]`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "set_cell_range")
	if input["range"] != "A1:B1" {
		t.Errorf("wire range = %v", input["range"])
	}
	if data := decodeEnvelopeData(t, out); data["updated_cells"].(float64) != 2 {
		t.Errorf("updated_cells = %v", data["updated_cells"])
	}
}

// TestExecute_DropdownSet covers the fan-out → set_cell_range write.
func TestExecute_DropdownSet(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{}`)
	_, err := runShortcutWithStubs(t, DropdownSet, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A2:A4",
		"--options", `["x","y"]`,
		"--multiple",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	if len(cells) != 3 {
		t.Errorf("wire cells rows = %d, want 3", len(cells))
	}
}

// TestExecute_DropdownUpdate_Batch covers the batch_update fan-out for
// dropdown-update. Verifies the captured request has 2 ops.
func TestExecute_DropdownUpdate_Batch(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"results":[{"ok":true},{"ok":true}]}`)
	_, err := runShortcutWithStubs(t, DropdownUpdate, []string{
		"--url", testURL,
		"--ranges", `["sheet1!A2:A5","sheet1!C2:C5"]`,
		"--options", `["a","b"]`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 2 {
		t.Errorf("operations len = %d, want 2", len(ops))
	}
}

// TestExecute_CellsSearch covers the search read path with options.
func TestExecute_CellsSearch(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "read", `{"matches":[{"cell":"B2"}],"has_more":false}`)
	out, err := runShortcutWithStubs(t, CellsSearch, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--find", "foo", "--match-case",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	data := decodeEnvelopeData(t, out)
	if data["matches"] == nil {
		t.Errorf("matches missing: %#v", data)
	}
}

// TestExecute_RangeMove covers the transform_range write path.
func TestExecute_RangeMove(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"moved":true}`)
	out, err := runShortcutWithStubs(t, RangeMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--source-range", "A1:C5",
		"--target-range", "D1",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "transform_range")
	if input["operation"] != "move" {
		t.Errorf("operation = %v, want move", input["operation"])
	}
}

// TestExecute_FilterCreate covers the filter special case (range mandatory,
// optional --data conditions merge).
func TestExecute_FilterCreate(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"filter_id":"sh1"}`)
	out, err := runShortcutWithStubs(t, FilterCreate, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:F100",
		"--properties", `{"rules":[{"col":"B","filter_type":"multiValue","expected":["x"]}]}`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "manage_filter_object")
	props, _ := input["properties"].(map[string]interface{})
	if props["range"] != "A1:F100" {
		t.Errorf("properties.range = %v", props["range"])
	}
	if props["rules"] == nil {
		t.Errorf("rules missing: %#v", props)
	}
}

// TestExecute_BatchUpdate_Raw covers the raw passthrough including
// continue_on_error.
func TestExecute_BatchUpdate_Raw(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"results":[{"ok":true}]}`)
	_, err := runShortcutWithStubs(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"tool_name":"set_cell_range","input":{"excel_id":"shtcnTestTOK","range":"A1","cells":[[{"value":1}]]}}]`,
		"--continue-on-error",
		"--yes",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "batch_update")
	if input["continue_on_error"] != true {
		t.Errorf("continue_on_error not propagated: %#v", input)
	}
}

// TestExecute_WorkbookCreate covers the legacy POST + optional
// set_cell_range follow-up. Stubs both endpoints.
func TestExecute_WorkbookCreate(t *testing.T) {
	t.Parallel()
	create := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{
					"spreadsheet_token": "shtcnBRAND",
					"title":             "Sales",
				},
			},
		},
	}
	fill := toolOutputStub("shtcnBRAND", "write", `{"updated_cells":4}`)
	out, err := runShortcutWithStubs(t, WorkbookCreate, []string{
		"--title", "Sales",
		"--headers", `["Name","Score"]`,
		"--values", `[["alice",95]]`,
	}, create, fill)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	ss, _ := data["spreadsheet"].(map[string]interface{})
	if ss["spreadsheet_token"] != "shtcnBRAND" {
		t.Errorf("spreadsheet_token = %v", ss["spreadsheet_token"])
	}
	if data["initial_fill"] == nil {
		t.Errorf("initial_fill missing in envelope")
	}
}

// TestExecute_DimMove covers the legacy v2 dimension_range call with
// CLI inclusive → API exclusive end-index conversion.
func TestExecute_DimMove(t *testing.T) {
	t.Parallel()
	move := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/" + testToken + "/dimension_range",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"moved": true},
		},
	}
	_, err := runShortcutWithStubs(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--dimension", "row", "--start", "0", "--end", "2", "--target", "10",
	}, move)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, move.CapturedBody)
	src, _ := body["source"].(map[string]interface{})
	if src["startIndex"].(float64) != 0 || src["endIndex"].(float64) != 3 {
		t.Errorf("indices = (%v,%v), want (0,3)", src["startIndex"], src["endIndex"])
	}
}

// TestExecute_ChartCreate covers the object-CRUD factory's create path.
func TestExecute_ChartCreate(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"chart_id":"chartNEW"}`)
	out, err := runShortcutWithStubs(t, ChartCreate, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--properties", `{"type":"line"}`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	data := decodeEnvelopeData(t, out)
	if data["chart_id"] != "chartNEW" {
		t.Errorf("chart_id = %v", data["chart_id"])
	}
}

// TestExecute_SheetCreate hits the workbook write path with all four
// optional flags so the input builder + callTool wiring is exercised.
func TestExecute_SheetCreate(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"sheet_id":"sh99","sheet_name":"Q4","index":2}`)
	out, err := runShortcutWithStubs(t, SheetCreate, []string{
		"--url", testURL,
		"--title", "Q4",
		"--index", "2",
		"--row-count", "300",
		"--col-count", "12",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "modify_workbook_structure")
	if input["operation"] != "create" || input["sheet_name"] != "Q4" {
		t.Errorf("input shape wrong: %#v", input)
	}
	if input["rows"].(float64) != 300 || input["columns"].(float64) != 12 {
		t.Errorf("dimensions = (%v, %v), want (300, 12)", input["rows"], input["columns"])
	}
}

// TestExecute_RangeSort exercises the sort_conditions JSON parsing
// alongside the boolean has_header.
func TestExecute_RangeSort(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"sorted":true}`)
	_, err := runShortcutWithStubs(t, RangeSort, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:D50",
		"--has-header",
		"--sort-keys", `[{"col":"B","order":"asc"}]`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, stub.CapturedBody)
	input := decodeToolInput(t, body, "transform_range")
	if input["operation"] != "sort" || input["has_header"] != true {
		t.Errorf("input wrong: %#v", input)
	}
	conds, _ := input["sort_conditions"].([]interface{})
	if len(conds) != 1 {
		t.Errorf("sort_conditions len = %d", len(conds))
	}
}

// decodeRawEnvelopeBody parses the raw JSON request body captured by an
// httpmock stub. Used by execute tests to inspect what the CLI sent on
// the wire (vs. dry-run tests that render the body up-front).
func decodeRawEnvelopeBody(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("captured body parse error: %v\nraw=%s", err, string(raw))
	}
	return body
}
