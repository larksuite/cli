// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
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
			"code":   1310201,
			"msg":    "spreadsheet not found",
			"log_id": "sheetai-log-1",
			"data":   map[string]interface{}{},
		},
	}
	_, _, err := func() (string, string, error) {
		parent, stdout, stderr, reg := newTestRig(t, WorkbookInfo)
		reg.Register(stub)
		parent.SetArgs([]string{"+workbook-info", "--url", testURL})
		err := parent.Execute()
		return stdout.String(), stderr.String(), err
	}()
	p := requireProblem(t, err, errs.CategoryAPI, errs.SubtypeServerError, "")
	if !strings.Contains(p.Message, "1310201") && !strings.Contains(p.Message, "not found") {
		t.Errorf("expected error code or message in problem; got message=%q", p.Message)
	}
	if p.LogID != "sheetai-log-1" {
		t.Errorf("LogID = %q, want %q (callTool's in-place rewrite must keep the classifier's log_id)", p.LogID, "sheetai-log-1")
	}
}

// TestExecute_ToolError_KnownSubtypePassthrough pins that callTool's rewrite
// only pins SubtypeUnknown to server_error: a code the table classifies
// (rate_limit, invalid_parameters, …) keeps its subtype — an agent routes on
// it, and stamping server_error would misread rate limiting or a bad
// parameter as a backend fault. The Message still gains the tool context.
func TestExecute_ToolError_KnownSubtypePassthrough(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		code          int
		msg           string
		wantSubtype   errs.Subtype
		wantRetryable bool
	}{
		{name: "rate limit", code: 99991400, msg: "rate limited", wantSubtype: errs.SubtypeRateLimit, wantRetryable: true},
		{name: "invalid parameters", code: 1310246, msg: "bad range", wantSubtype: errs.SubtypeInvalidParameters},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stub := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/sheet_ai/v2/spreadsheets/" + testToken + "/tools/invoke_read",
				Body: map[string]interface{}{
					"code": tc.code,
					"msg":  tc.msg,
					"data": map[string]interface{}{},
				},
			}
			parent, _, _, reg := newTestRig(t, WorkbookInfo)
			reg.Register(stub)
			parent.SetArgs([]string{"+workbook-info", "--url", testURL})
			err := parent.Execute()
			p := requireProblem(t, err, errs.CategoryAPI, tc.wantSubtype, tc.msg)
			if !strings.Contains(p.Message, "workbook_info") && !strings.Contains(p.Message, "tool ") {
				t.Errorf("message should carry the tool context, got %q", p.Message)
			}
			if p.Retryable != tc.wantRetryable {
				t.Errorf("Retryable = %v, want %v (classifier metadata must survive the rewrite)", p.Retryable, tc.wantRetryable)
			}
		})
	}
}

// TestExecute_WikiURLResolvesToSheet covers the two-step wiki path: a /wiki/
// URL is resolved via get_node to its spreadsheet obj_token, which then feeds
// the tool invoke. The tool stub is keyed on the resolved obj_token, so the
// test would fail if the node_token were used unresolved.
func TestExecute_WikiURLResolvesToSheet(t *testing.T) {
	t.Parallel()
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": testToken,
				},
			},
		},
	}
	tool := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"sh1","title":"Sheet1","index":0}]}`)
	out, err := runShortcutWithStubs(t, WorkbookInfo,
		[]string{"--url", "https://example.feishu.cn/wiki/wikTestNODE"}, getNode, tool)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	if sheets, _ := data["sheets"].([]interface{}); len(sheets) != 1 {
		t.Fatalf("sheets len = %d, want 1; out=%s", len(sheets), out)
	}
}

// TestExecute_RevisionGet_WikiURL guards RevisionGet's custom Execute hook:
// the wiki node token must be resolved before get_workbook_structure runs.
func TestExecute_RevisionGet_WikiURL(t *testing.T) {
	t.Parallel()
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": testToken,
				},
			},
		},
	}
	tool := toolOutputStub(testToken, "read", `{"revision":60}`)
	out, err := runShortcutWithStubs(t, RevisionGet,
		[]string{"--url", "https://example.feishu.cn/wiki/wikTestNODE"}, getNode, tool)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	if data["revision"] != float64(60) {
		t.Fatalf("revision = %v, want 60; out=%s", data["revision"], out)
	}
}

// TestExecute_WikiURLWrongObjType rejects a wiki node that resolves to a
// non-spreadsheet obj_type before any tool invoke.
func TestExecute_WikiURLWrongObjType(t *testing.T) {
	t.Parallel()
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "docx",
					"obj_token": "docABC",
				},
			},
		},
	}
	_, err := runShortcutWithStubs(t, WorkbookInfo,
		[]string{"--url", "https://example.feishu.cn/wiki/wikTestNODE"}, getNode)
	requireValidation(t, err, "obj_type")
}

// TestExecute_WikiURLIncompleteNode treats an incomplete get_node response
// (missing obj_type/obj_token) as an internal/server error, not a user --url
// validation error.
func TestExecute_WikiURLIncompleteNode(t *testing.T) {
	t.Parallel()
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{},
			},
		},
	}
	_, err := runShortcutWithStubs(t, WorkbookInfo,
		[]string{"--url", "https://example.feishu.cn/wiki/wikTestNODE"}, getNode)
	if err == nil {
		t.Fatal("want error for incomplete get_node node data")
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) {
		t.Fatalf("incomplete-data error classified as validation (%v); want internal", err)
	}
}

// TestExecute_RangeMove_WikiURL guards the transformExecuteFn path: +range-move
// and +range-copy use a named Execute helper (not an inline func), so they must
// still resolve a /wiki/ URL to the backing spreadsheet token before calling
// transform_range. The tool stub is keyed on the resolved obj_token, so an
// unresolved node_token would miss it and fail this test.
func TestExecute_RangeMove_WikiURL(t *testing.T) {
	t.Parallel()
	getNode := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"obj_type":  "sheet",
					"obj_token": testToken,
				},
			},
		},
	}
	tool := toolOutputStub(testToken, "write", `{"updated_range":"A10:B11"}`)
	out, err := runShortcutWithStubs(t, RangeMove,
		[]string{
			"--url", "https://example.feishu.cn/wiki/wikTestNODE",
			"--sheet-id", testSheetID,
			"--source-range", "A1:B2",
			"--target-range", "A10",
		}, getNode, tool)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
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

// TestExecute_SheetMove_LookupsIndexByTitle covers the same lookup path as
// above but with get_workbook_structure exposing the display name as "title"
// (the field the real tool returns) instead of "sheet_name". lookupSheetIndex
// must resolve --sheet-name against either key.
func TestExecute_SheetMove_LookupsIndexByTitle(t *testing.T) {
	t.Parallel()
	lookup := toolOutputStub(testToken, "read", `{"sheets":[{"sheet_id":"sh1","title":"汇总","index":3}]}`)
	move := toolOutputStub(testToken, "write", `{"sheet_id":"sh1"}`)
	out, err := runShortcutWithStubs(t, SheetMove,
		[]string{"--url", testURL, "--sheet-name", "汇总", "--index", "0"},
		lookup, move,
	)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if move.CapturedBody == nil {
		t.Fatal("move stub didn't capture a body")
	}
	body := decodeRawEnvelopeBody(t, move.CapturedBody)
	input := decodeToolInput(t, body, "modify_workbook_structure")
	if input["sheet_id"] != "sh1" {
		t.Errorf("sheet_id = %v, want sh1 (resolved from --sheet-name via title)", input["sheet_id"])
	}
	if input["source_index"].(float64) != 3 {
		t.Errorf("source_index = %v, want 3 (from lookup)", input["source_index"])
	}
}

// TestExecute_CellsGet covers a multi-range read end-to-end.
func TestExecute_CellsGet(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "read", `{"ranges":[{"range":"A1:B2","cells":[[{"value":1}]]}]}`)
	out, err := runShortcutWithStubs(t, CellsGet,
		[]string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:B2"}, stub)
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
		"--properties", `{"rules":[{"column_index":"B","conditions":[{"type":"multiValue","compare_type":"equal","values":["x"]}]}]}`,
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

// TestExecute_BatchUpdate_Translated covers the CLI-shape → MCP-shape
// translation: user passes {shortcut, input}, batchOpDispatch maps it to
// {tool_name, input(+operation, +excel_id)} before the tool call. Also
// verifies --continue-on-error.
func TestExecute_BatchUpdate_Translated(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"results":[{"ok":true}]}`)
	_, err := runShortcutWithStubs(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+cells-set","input":{"sheet-id":"sh1","range":"A1","cells":[[{"value":1}]]}}]`,
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
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("operations length = %d, want 1", len(ops))
	}
	op := ops[0].(map[string]interface{})
	if op["tool_name"] != "set_cell_range" {
		t.Errorf("op.tool_name = %v, want set_cell_range (translated from +cells-set)", op["tool_name"])
	}
	subInput, _ := op["input"].(map[string]interface{})
	if subInput["excel_id"] != testToken {
		t.Errorf("op.input.excel_id = %v, want %s (translator should inject)", subInput["excel_id"], testToken)
	}
	if _, has := subInput["operation"]; has {
		t.Errorf("op.input.operation present but +cells-set should not inject one: %#v", subInput)
	}
}

func TestExecute_BatchChartCreate_ContinueOnErrorKeepsLocallyValidOperations(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{
		"total":1,
		"succeeded":1,
		"failed":0,
		"results":[{"index":0,"tool_name":"manage_chart_object","success":true}]
	}`)
	out, err := runShortcutWithStubs(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[
			{"sheet-id":"sh1","chart-type":"donut","data-range":"A1:C10"},
			{"sheet-id":"sh1","chart-type":"line","data-range":"E1:G10","title":"Trend"}
		]`,
		"--continue-on-error",
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	input := decodeToolInput(t, decodeRawEnvelopeBody(t, stub.CapturedBody), "batch_update")
	ops, _ := input["operations"].([]interface{})
	if len(ops) != 1 {
		t.Fatalf("server should receive only the locally valid operation, got %d", len(ops))
	}
	for _, want := range []string{
		`"total": 2`,
		`"succeeded": 1`,
		`"failed": 1`,
		`"index": 0`,
		`"index": 1`,
		`"stage": "cli_validation"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("merged partial result should contain %q, got:\n%s", want, out)
		}
	}
}

func TestExecute_BatchChartCreate_StrictModeRejectsBeforeWrite(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, BatchChartCreate, []string{
		"--url", testURL,
		"--operations", `[
			{"sheet-id":"sh1","chart-type":"donut","data-range":"A1:C10"},
			{"sheet-id":"sh1","chart-type":"line","data-range":"E1:G10","title":"Trend"}
		]`,
		"--continue-on-error=false",
	})
	requireValidation(t, err, "invalid value \"donut\" for --chart-type")
}

func TestExecute_BatchChartUpdate_PreflightsSnapshots(t *testing.T) {
	t.Parallel()
	read := toolOutputStub(testToken, "read", `{
		"sheets":[{
			"sheet_id":"shtSubA",
			"charts":[{
				"chart_id":"chart-1",
				"details":{"snapshot":{
					"title":{"text":"Old"},
					"plotArea":{"plot":{"type":"line"}}
				}}
			}]
		}]
	}`)
	write := toolOutputStub(testToken, "write", `{
		"total":1,
		"succeeded":1,
		"failed":0,
		"results":[{"index":0,"tool_name":"manage_chart_object","success":true}]
	}`)
	out, err := runShortcutWithStubs(t, BatchChartUpdate, []string{
		"--url", testURL,
		"--operations", `[{
			"shortcut":"+chart-config-update",
			"input":{"sheetId":"shtSubA","chartId":"chart-1","title":"New"}
		}]`,
	}, read, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, write.CapturedBody), "batch_update")
	ops := input["operations"].([]interface{})
	chartInput := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	snapshot := chartDryRunSnapshot(t, chartInput)
	if snapshot["title"].(map[string]interface{})["text"] != "New" {
		t.Fatalf("batch partial title = %#v", snapshot["title"])
	}
}

func TestExecute_ChartBatches_RejectDuplicateTargetByIDAndName(t *testing.T) {
	t.Parallel()
	operations := `[
			{"shortcut":"+chart-config-update","input":{"sheet_id":"shtSubA","chart_id":"chart-1","title":"New"}},
			{"shortcut":"+chart-data-update","input":{"sheet_name":"Data","chart_id":"chart-1","data_range":"A1:C10"}}
		]`
	for _, tc := range []struct {
		name     string
		shortcut common.Shortcut
		extra    []string
	}{
		{name: "dedicated chart batch", shortcut: BatchChartUpdate},
		{name: "general batch", shortcut: BatchUpdate, extra: []string{"--yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			structure := toolOutputStub(testToken, "read", `{
				"sheets":[{"sheet_id":"shtSubA","title":"Data","index":0}]
			}`)
			args := []string{"--url", testURL, "--operations", operations}
			_, err := runShortcutWithStubs(t, tc.shortcut, append(args, tc.extra...), structure)
			requireValidation(t, err, "both target chart \"chart-1\"")
		})
	}
}

func TestExecute_BatchUpdate_MixesCellsAndSemanticChartUpdate(t *testing.T) {
	t.Parallel()
	read := toolOutputStub(testToken, "read", `{
		"sheets":[{
			"sheet_id":"shtSubA",
			"charts":[{
				"chart_id":"chart-1",
				"details":{"snapshot":{
					"title":{"text":"Old"},
					"plotArea":{"plot":{"type":"line"}}
				}}
			}]
		}]
	}`)
	write := toolOutputStub(testToken, "write", `{
		"total":2,
		"succeeded":2,
		"failed":0,
		"results":[
			{"index":0,"tool_name":"set_cell_range","success":true},
			{"index":1,"tool_name":"manage_chart_object","success":true}
		]
	}`)
	out, err := runShortcutWithStubs(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[
			{"shortcut":"+cells-set","input":{"sheet-id":"shtSubA","range":"A1","cells":[[{"value":1}]]}},
			{"shortcut":"+chart-config-update","input":{"sheetId":"shtSubA","chartId":"chart-1","title":"New"}}
		]`,
		"--yes",
	}, read, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	input := decodeToolInput(t, decodeRawEnvelopeBody(t, write.CapturedBody), "batch_update")
	ops := input["operations"].([]interface{})
	if len(ops) != 2 || ops[0].(map[string]interface{})["tool_name"] != "set_cell_range" {
		t.Fatalf("mixed operations = %#v", ops)
	}
	chartInput := ops[1].(map[string]interface{})["input"].(map[string]interface{})
	snapshot := chartDryRunSnapshot(t, chartInput)
	if snapshot["title"].(map[string]interface{})["text"] != "New" {
		t.Fatalf("generic batch partial title = %#v", snapshot["title"])
	}
}

func TestExecute_BatchUpdate_CompactsChartCreateSnapshot(t *testing.T) {
	t.Parallel()
	write := toolOutputStub(testToken, "write", `{
		"total":1,
		"succeeded":1,
		"failed":0,
		"results":[{
			"index":0,
			"tool_name":"manage_chart_object",
			"success":true,
			"data":{"chart_id":"chart-1","snapshot":{"title":{"text":"Large"}}}
		}]
	}`)
	out, err := runShortcutWithStubs(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{
			"shortcut":"+chart-create-basic",
			"input":{"sheet-id":"shtSubA","chart-type":"line","data-range":"A1:C10"}
		}]`,
		"--yes",
	}, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	if strings.Contains(out, `"snapshot"`) {
		t.Fatalf("generic batch create must omit the full chart snapshot: %s", out)
	}
	if !strings.Contains(out, `"chart_id": "chart-1"`) {
		t.Fatalf("generic batch create must retain chart_id: %s", out)
	}
}

// TestExecute_BatchUpdate_ContinueOnErrorPrecedence locks the flag-vs-envelope
// precedence: an explicit --continue-on-error=false must keep the strict
// transaction even when the --operations envelope carries continue_on_error:true,
// while an envelope value still applies when the flag is absent. Guards against
// the regression where the flag was read by value (runtime.Bool) rather than by
// Changed().
func TestExecute_BatchUpdate_ContinueOnErrorPrecedence(t *testing.T) {
	t.Parallel()
	envelope := `{"operations":[{"shortcut":"+cells-set","input":{"sheet-id":"sh1","range":"A1","cells":[[{"value":1}]]}}],"continue_on_error":true}`

	t.Run("explicit false overrides envelope", func(t *testing.T) {
		t.Parallel()
		stub := toolOutputStub(testToken, "write", `{"results":[{"ok":true}]}`)
		_, err := runShortcutWithStubs(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", envelope,
			"--continue-on-error=false",
			"--yes",
		}, stub)
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		input := decodeToolInput(t, decodeRawEnvelopeBody(t, stub.CapturedBody), "batch_update")
		if input["continue_on_error"] == true {
			t.Errorf("explicit --continue-on-error=false must win over envelope; got continue_on_error=%#v", input["continue_on_error"])
		}
	})

	t.Run("envelope applies when flag absent", func(t *testing.T) {
		t.Parallel()
		stub := toolOutputStub(testToken, "write", `{"results":[{"ok":true}]}`)
		_, err := runShortcutWithStubs(t, BatchUpdate, []string{
			"--url", testURL,
			"--operations", envelope,
			"--yes",
		}, stub)
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}
		input := decodeToolInput(t, decodeRawEnvelopeBody(t, stub.CapturedBody), "batch_update")
		if input["continue_on_error"] != true {
			t.Errorf("envelope continue_on_error:true should apply when --continue-on-error absent; got %#v", input["continue_on_error"])
		}
	})
}

// TestExecute_WorkbookCreate covers the create POST + first-sheet lookup +
// set_cell_range follow-up. Stubs all three endpoints.
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
	// The write reads the workbook structure to resolve the default sheet's id
	// (the create response doesn't echo it). lookupFirstSheetID and
	// writeTypedSheets' listSheetIDsByName both read it — one reusable stub serves
	// both. The synthesized sheet is named "Sheet1", matching the default sheet,
	// so it's adopted in place (no rename).
	structure := toolOutputStub("shtcnBRAND", "read", `{"sheets":[{"sheet_id":"shtFirst","sheet_name":"Sheet1","index":0}]}`)
	structure.Reusable = true
	fill := toolOutputStub("shtcnBRAND", "write", `{"updated_cells":4}`)
	out, err := runShortcutWithStubs(t, WorkbookCreate, []string{
		"--title", "Sales",
		"--values", `[["Name","Score"],["alice",95]]`,
	}, create, structure, fill)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}
	data := decodeEnvelopeData(t, out)
	ss, _ := data["spreadsheet"].(map[string]interface{})
	if ss["spreadsheet_token"] != "shtcnBRAND" {
		t.Errorf("spreadsheet_token = %v", ss["spreadsheet_token"])
	}
	if sheets, _ := data["sheets"].([]interface{}); len(sheets) != 1 {
		t.Errorf("sheets summary missing in envelope; got %#v", data["sheets"])
	}
	// The fill must target the resolved first sheet, not an empty selector.
	fillInput := decodeToolInput(t, decodeRawEnvelopeBody(t, fill.CapturedBody), "set_cell_range")
	if fillInput["sheet_id"] != "shtFirst" {
		t.Errorf("fill sheet_id = %v, want shtFirst (resolved from workbook structure)", fillInput["sheet_id"])
	}
}

// TestExecute_WorkbookCreate_EmptyArraysSkipFill locks the fix for the nil-map
// panic / illegal-range bug: --values '[]' must short-circuit the initial fill
// (no structure/fill calls fire) and finish with the spreadsheet created but no
// sheets summary — never panic on a nil payload.
func TestExecute_WorkbookCreate_EmptyArraysSkipFill(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, flag, val string }{
		{"empty values", "--values", "[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			create := &httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/sheets/v3/spreadsheets",
				Body: map[string]interface{}{
					"code": 0, "msg": "success",
					"data": map[string]interface{}{
						"spreadsheet": map[string]interface{}{"spreadsheet_token": "shtNEW", "title": "X"},
					},
				},
			}
			// Only the create stub is provided: an empty array must skip the fill
			// entirely, so no structure/fill call fires (and no nil-map panic).
			out, err := runShortcutWithStubs(t, WorkbookCreate, []string{"--title", "X", tc.flag, tc.val}, create)
			if err != nil {
				t.Fatalf("execute failed: %v\nout=%s", err, out)
			}
			data := decodeEnvelopeData(t, out)
			if data["sheets"] != nil {
				t.Errorf("sheets should be absent for %s %s; got %#v", tc.flag, tc.val, data["sheets"])
			}
			if ss, _ := data["spreadsheet"].(map[string]interface{}); ss["spreadsheet_token"] != "shtNEW" {
				t.Errorf("spreadsheet_token = %v, want shtNEW", ss["spreadsheet_token"])
			}
		})
	}
}

// TestExecute_WorkbookCreate_FillFailureKeepsToken locks the partial-state
// contract: when the spreadsheet is created but the follow-up fill can't resolve
// its first sheet, the result lands on stdout as an ok:false envelope carrying
// spreadsheet_token + reason + a structured cause field, and the process exits
// with the bare partial-failure signal — matching +table-put's tablePutPartial
// shape so agents see one consistent "side effect landed but follow-up didn't"
// contract across the sheets domain (instead of the old failed_precondition
// stderr envelope).
func TestExecute_WorkbookCreate_FillFailureKeepsToken(t *testing.T) {
	t.Parallel()
	create := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"spreadsheet": map[string]interface{}{"spreadsheet_token": "shtNEW", "title": "X"},
			},
		},
	}
	// Structure comes back with no sheets, so lookupFirstSheetID fails AFTER the
	// spreadsheet already exists — exercising the partial-state path.
	structure := toolOutputStub("shtNEW", "read", `{"sheets":[]}`)
	out, err := runShortcutWithStubs(t, WorkbookCreate, []string{"--title", "X", "--values", `[["a"]]`}, create, structure)
	if err == nil {
		t.Fatalf("expected partial-failure exit signal; got nil. out=%s", out)
	}
	var pfErr *output.PartialFailureError
	if !errors.As(err, &pfErr) {
		t.Fatalf("expected *output.PartialFailureError exit signal; got %T %v", err, err)
	}

	var env map[string]interface{}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", jerr, out)
	}
	if ok, _ := env["ok"].(bool); ok {
		t.Errorf("partial-state envelope must be ok:false; got out=%s", out)
	}
	data, _ := env["data"].(map[string]interface{})
	if got := data["spreadsheet_token"]; got != "shtNEW" {
		t.Errorf("spreadsheet_token = %v, want shtNEW (recovery requires the token to be in the envelope)", got)
	}
	reason, _ := data["reason"].(string)
	if !strings.Contains(reason, "shtNEW") {
		t.Errorf("reason = %q, want the spreadsheet token named for recovery", reason)
	}
	hint, _ := data["hint"].(string)
	if !strings.Contains(hint, "spreadsheet_token") {
		t.Errorf("hint = %q, want recovery guidance naming spreadsheet_token", hint)
	}
	// The underlying fill failure's typed shape is flattened into the cause
	// field so the inner subtype stays diagnosable from the JSON envelope alone.
	cause, _ := data["cause"].(map[string]interface{})
	if got := cause["subtype"]; got != string(errs.SubtypeInvalidResponse) {
		t.Errorf("cause.subtype = %v, want the underlying invalid_response subtype", got)
	}
}

// TestExecute_DimMove covers the native v3 move_dimension call. CLI's
// --source-range "1:3" (1-based inclusive) is parsed into v3's
// source.{start_index=0,end_index=2} (0-based inclusive); --target "11" is
// parsed into destination_index=10.
func TestExecute_DimMove(t *testing.T) {
	t.Parallel()
	move := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets/" + testToken + "/sheets/" + testSheetID + "/move_dimension",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{"moved": true},
		},
	}
	_, err := runShortcutWithStubs(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--source-range", "1:3", "--target", "11",
	}, move)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	body := decodeRawEnvelopeBody(t, move.CapturedBody)
	src, _ := body["source"].(map[string]interface{})
	if src["start_index"].(float64) != 0 || src["end_index"].(float64) != 2 {
		t.Errorf("indices = (%v,%v), want (0,2) — 0-based inclusive", src["start_index"], src["end_index"])
	}
	if body["destination_index"].(float64) != 10 {
		t.Errorf("destination_index = %v, want 10", body["destination_index"])
	}
}

// TestExecute_ChartCreate covers the object-CRUD factory's create path.
func TestExecute_ChartCreate(t *testing.T) {
	t.Parallel()
	stub := toolOutputStub(testToken, "write", `{"chart_id":"chartNEW"}`)
	out, err := runShortcutWithStubs(t, ChartCreate, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--properties", `{"type":"line","position":{"row":0,"col":"A"},"size":{"width":400,"height":300}}`,
	}, stub)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	data := decodeEnvelopeData(t, out)
	if data["chart_id"] != "chartNEW" {
		t.Errorf("chart_id = %v", data["chart_id"])
	}
}

func TestExecute_ChartConfigUpdate_ReadsSnapshotAndWritesPartialPatch(t *testing.T) {
	t.Parallel()
	readBefore := toolOutputStub(testToken, "read", `{
		"sheets":[{
			"sheet_id":"shtSubA",
			"charts":[{
				"chart_id":"chart-1",
				"details":{"snapshot":{
					"title":{"text":"Old"},
					"plotArea":{
						"axes":[
							{"type":"x","position":"bottom","title":{"text":"Month"}},
							{"type":"y","position":"left","title":{"text":"Amount"}}
						],
						"plot":{"type":"line","extra":{"smooth":false}}
					},
					"data":{"direction":"column"}
				}}
			}]
		}]
	}`)
	write := toolOutputStub(testToken, "write", `{"chart_id":"chart-1"}`)
	readAfter := toolOutputStub(testToken, "read", `{
		"sheets":[{
			"sheet_id":"shtSubA",
			"charts":[{
				"chart_id":"chart-1",
				"details":{"snapshot":{
					"title":{"text":"New"},
					"plotArea":{
						"axes":[
							{"type":"x","position":"bottom","title":{"text":"Month"}},
							{"type":"y","position":"left","title":{"text":"Revenue"}}
						],
						"plot":{"type":"line","series":[{"index":1,"points":{"point":[{"index":4,"labels":{"value":true}}]}}]}
					},
					"data":{"direction":"column"}
				}}
			}]
		}]
	}`)
	out, err := runShortcutWithStubs(t, ChartConfigUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--title", "New",
		"--y-axis-title", "Revenue",
		"--last-point-label=true",
	}, readBefore, write, readAfter)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	readInput := decodeToolInput(t, decodeRawEnvelopeBody(t, readBefore.CapturedBody), "get_chart_objects")
	if readInput["chart_id"] != "chart-1" {
		t.Fatalf("read chart_id = %#v", readInput["chart_id"])
	}
	writeInput := decodeToolInput(t, decodeRawEnvelopeBody(t, write.CapturedBody), "manage_chart_object")
	if _, ok := writeInput["last_point_label"]; ok {
		t.Fatalf("last_point_label must not be written at the tool input root: %#v", writeInput)
	}
	writeProperties := writeInput["properties"].(map[string]interface{})
	if writeProperties["last_point_label"] != true {
		t.Fatalf("last_point_label = %#v, want true", writeProperties["last_point_label"])
	}
	snapshot := chartDryRunSnapshot(t, writeInput)
	if snapshot["title"].(map[string]interface{})["text"] != "New" {
		t.Fatalf("partial title = %#v", snapshot["title"])
	}
	axes := snapshot["plotArea"].(map[string]interface{})["axes"].([]interface{})
	if len(axes) != 2 || axes[0].(map[string]interface{})["title"].(map[string]interface{})["text"] != "Month" ||
		axes[1].(map[string]interface{})["title"].(map[string]interface{})["text"] != "Revenue" {
		t.Fatalf("partial axes = %#v", axes)
	}
	data := decodeEnvelopeData(t, out)
	viewModel := data["viewModel"].(map[string]interface{})
	if _, ok := viewModel["data"]; ok {
		t.Fatal("config shortcut output viewModel must not include data")
	}
	plot := viewModel["plotArea"].(map[string]interface{})["plot"].(map[string]interface{})
	series := plot["series"].([]interface{})
	point := series[0].(map[string]interface{})["points"].(map[string]interface{})["point"].([]interface{})[0].(map[string]interface{})
	if point["labels"].(map[string]interface{})["value"] != true {
		t.Fatalf("viewModel must come from the post-update readback: %#v", viewModel)
	}
}

func TestExecute_ChartDataUpdate_ReadsSnapshotAndReturnsData(t *testing.T) {
	t.Parallel()
	read := toolOutputStub(testToken, "read", `{
		"sheets":[{
			"sheet_id":"shtSubA",
			"charts":[{
				"chart_id":"chart-1",
				"details":{"snapshot":{
					"plotArea":{"plot":{"type":"line"}},
					"data":{
						"isStaticData":false,
						"direction":"column",
						"refs":[{"value":"A1:C10"}],
						"dim1":{"serie":{"index":1}},
						"dim2":{"series":[{"index":2},{"index":3}]}
					}
				}}
			}]
		}]
	}`)
	write := toolOutputStub(testToken, "write", `{"chart_id":"chart-1"}`)
	out, err := runShortcutWithStubs(t, ChartDataUpdate, []string{
		"--url", testURL,
		"--sheet-id", testSheetID,
		"--chart-id", "chart-1",
		"--data-range", "A1:D10",
		"--dim1-index", "1",
		"--dim2-indexes", "2,4",
	}, read, write)
	if err != nil {
		t.Fatalf("execute failed: %v\nout=%s", err, out)
	}

	writeInput := decodeToolInput(t, decodeRawEnvelopeBody(t, write.CapturedBody), "manage_chart_object")
	patchData := chartDryRunSnapshot(t, writeInput)["data"].(map[string]interface{})
	series := patchData["dim2"].(map[string]interface{})["series"].([]interface{})
	if len(series) != 2 || series[0].(map[string]interface{})["index"] != float64(2) ||
		series[1].(map[string]interface{})["index"] != float64(4) {
		t.Fatalf("partial data series = %#v", series)
	}
	data := decodeEnvelopeData(t, out)
	returned := data["data"].(map[string]interface{})
	if returned["direction"] != "column" {
		t.Fatalf("returned data = %#v", returned)
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
		"--sort-keys", `[{"column":"B","ascending":true}]`,
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
