// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// ── MergeCells ───────────────────────────────────────────────────────────────

func TestSheetMergeCellsValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "range": "sheet1!A1:B2", "sheet-id": "", "merge-type": "MERGE_ALL",
	}, nil)
	err := SheetMergeCells.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetMergeCellsValidateRelativeRangeWithoutSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "A1:B2", "sheet-id": "", "merge-type": "MERGE_ALL",
	}, nil)
	err := SheetMergeCells.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--sheet-id") {
		t.Fatalf("expected sheet-id error, got: %v", err)
	}
}

func TestSheetMergeCellsValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "", "merge-type": "MERGE_ROWS",
	}, nil)
	if err := SheetMergeCells.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetMergeCellsDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "range": "A1:B2", "sheet-id": "sheet1", "merge-type": "MERGE_ALL",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetMergeCells.DryRun(context.Background(), rt))
	if !strings.Contains(got, `merge_cells`) {
		t.Fatalf("DryRun URL missing merge_cells: %s", got)
	}
	if !strings.Contains(got, `"range":"sheet1!A1:B2"`) {
		t.Fatalf("DryRun range not normalized: %s", got)
	}
	if !strings.Contains(got, `"mergeType":"MERGE_ALL"`) {
		t.Fatalf("DryRun missing mergeType: %s", got)
	}
}

func TestSheetMergeCellsExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/merge_cells",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"spreadsheetToken": "shtTOKEN"}},
	})
	err := mountAndRunSheets(t, SheetMergeCells, []string{
		"+merge-cells", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--merge-type", "MERGE_ALL", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "spreadsheetToken") {
		t.Fatalf("stdout missing spreadsheetToken: %s", stdout.String())
	}
}

func TestSheetMergeCellsExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/merge_cells",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetMergeCells, []string{
		"+merge-cells", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--merge-type", "MERGE_ALL", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── UnmergeCells ─────────────────────────────────────────────────────────────

func TestSheetUnmergeCellsValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "range": "sheet1!A1:B2", "sheet-id": "",
	}, nil)
	err := SheetUnmergeCells.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetUnmergeCellsValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
	}, nil)
	if err := SheetUnmergeCells.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetUnmergeCellsDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "range": "sheet1!A1:B2", "sheet-id": "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetUnmergeCells.DryRun(context.Background(), rt))
	if !strings.Contains(got, `unmerge_cells`) {
		t.Fatalf("DryRun URL missing unmerge_cells: %s", got)
	}
	if !strings.Contains(got, `"range":"sheet1!A1:B2"`) {
		t.Fatalf("DryRun missing range: %s", got)
	}
}

func TestSheetUnmergeCellsExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/unmerge_cells",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"spreadsheetToken": "shtTOKEN"}},
	})
	err := mountAndRunSheets(t, SheetUnmergeCells, []string{
		"+unmerge-cells", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetUnmergeCellsExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/unmerge_cells",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetUnmergeCells, []string{
		"+unmerge-cells", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── Replace ──────────────────────────────────────────────────────────────────

func TestSheetReplaceValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1", "find": "a", "replacement": "b", "range": "",
	}, map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	err := SheetReplace.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetReplaceValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "find": "hello", "replacement": "world", "range": "",
	}, map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	if err := SheetReplace.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetReplaceValidateMismatchedRangeSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1", "find": "a", "replacement": "b",
		"range": "sheet2!A1:B2",
	}, map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	err := SheetReplace.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch error, got: %v", err)
	}
}

func TestSheetReplaceValidateMatchingRangeSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1", "find": "a", "replacement": "b",
		"range": "sheet1!A1:B2",
	}, map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	if err := SheetReplace.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetReplaceDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "find": "old", "replacement": "new", "range": "A1:C5",
	}, map[string]bool{"match-case": true, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	got := mustMarshalSheetsDryRun(t, SheetReplace.DryRun(context.Background(), rt))
	if !strings.Contains(got, `replace`) {
		t.Fatalf("DryRun URL missing replace: %s", got)
	}
	if !strings.Contains(got, `"find":"old"`) {
		t.Fatalf("DryRun missing find: %s", got)
	}
	if !strings.Contains(got, `"replacement":"new"`) {
		t.Fatalf("DryRun missing replacement: %s", got)
	}
	if !strings.Contains(got, `"match_case":true`) {
		t.Fatalf("DryRun missing match_case: %s", got)
	}
}

func TestSheetReplaceDryRunNoRange(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "find": "a", "replacement": "b", "range": "",
	}, map[string]bool{"match-case": false, "match-entire-cell": false, "search-by-regex": false, "include-formulas": false})
	got := mustMarshalSheetsDryRun(t, SheetReplace.DryRun(context.Background(), rt))
	// When no range specified, range defaults to sheet-id
	if !strings.Contains(got, `"range":"sheet1"`) {
		t.Fatalf("DryRun range should default to sheet-id: %s", got)
	}
}

func TestSheetReplaceExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/replace",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"replace_result": map[string]interface{}{
				"matched_cells": []interface{}{"A1"}, "rows_count": float64(1),
			},
		}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetReplace, []string{
		"+replace", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--find", "hello", "--replacement", "world", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "matched_cells") {
		t.Fatalf("stdout missing matched_cells: %s", stdout.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["find"] != "hello" || body["replacement"] != "world" {
		t.Fatalf("unexpected body: %#v", body)
	}
}

func TestSheetReplaceExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/replace",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetReplace, []string{
		"+replace", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--find", "a", "--replacement", "b", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── SetStyle ─────────────────────────────────────────────────────────────────

func TestSheetSetStyleValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `{"font":{"bold":true}}`,
	}, nil)
	err := SheetSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetSetStyleValidateInvalidJSON(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `{invalid}`,
	}, nil)
	err := SheetSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--style must be valid JSON") {
		t.Fatalf("expected JSON error, got: %v", err)
	}
}

func TestSheetSetStyleValidateRejectsArray(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `[{"bold":true}]`,
	}, nil)
	err := SheetSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("expected object error, got: %v", err)
	}
}

func TestSheetSetStyleValidateRejectsString(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `"bold"`,
	}, nil)
	err := SheetSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("expected object error, got: %v", err)
	}
}

func TestSheetSetStyleValidateRejectsNull(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `null`,
	}, nil)
	err := SheetSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "JSON object") {
		t.Fatalf("expected object error, got: %v", err)
	}
}

func TestSheetSetStyleValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "sheet1!A1:B2", "sheet-id": "",
		"style": `{"font":{"bold":true},"backColor":"#ff0000"}`,
	}, nil)
	if err := SheetSetStyle.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetSetStyleDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "range": "A1:B2", "sheet-id": "sheet1",
		"style": `{"font":{"bold":true}}`,
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetSetStyle.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"PUT"`) {
		t.Fatalf("DryRun should use PUT: %s", got)
	}
	if !strings.Contains(got, `/style`) {
		t.Fatalf("DryRun URL missing /style: %s", got)
	}
	if !strings.Contains(got, `"range":"sheet1!A1:B2"`) {
		t.Fatalf("DryRun range not normalized: %s", got)
	}
	if !strings.Contains(got, `"bold":true`) {
		t.Fatalf("DryRun missing style: %s", got)
	}
}

func TestSheetSetStyleExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/style",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"updates": map[string]interface{}{"updatedCells": float64(4), "updatedRange": "sheet1!A1:B2"},
		}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetSetStyle, []string{
		"+set-style", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--style", `{"font":{"bold":true}}`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "updatedCells") {
		t.Fatalf("stdout missing updatedCells: %s", stdout.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	appendStyle, _ := body["appendStyle"].(map[string]interface{})
	if appendStyle["range"] != "sheet1!A1:B2" {
		t.Fatalf("unexpected range: %v", appendStyle["range"])
	}
}

func TestSheetSetStyleExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/style",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetSetStyle, []string{
		"+set-style", "--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:B2", "--style", `{"font":{"bold":true}}`, "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── BatchSetStyle ────────────────────────────────────────────────────────────

func TestSheetBatchSetStyleValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "",
		"data": `[{"ranges":["sheet1!A1:B2"],"style":{"font":{"bold":true}}}]`,
	}, nil)
	err := SheetBatchSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetBatchSetStyleValidateInvalidJSON(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "data": `not-json`,
	}, nil)
	err := SheetBatchSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--data must be valid JSON") {
		t.Fatalf("expected JSON error, got: %v", err)
	}
}

func TestSheetBatchSetStyleValidateNotArray(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "data": `{"not":"array"}`,
	}, nil)
	err := SheetBatchSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "non-empty JSON array") {
		t.Fatalf("expected array error, got: %v", err)
	}
}

func TestSheetBatchSetStyleValidateEmptyArray(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "data": `[]`,
	}, nil)
	err := SheetBatchSetStyle.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "non-empty JSON array") {
		t.Fatalf("expected empty array error, got: %v", err)
	}
}

func TestSheetBatchSetStyleValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"data": `[{"ranges":["sheet1!A1:B2"],"style":{"font":{"bold":true}}}]`,
	}, nil)
	if err := SheetBatchSetStyle.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetBatchSetStyleDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test",
		"data": `[{"ranges":["sheet1!A1:B2"],"style":{"backColor":"#ff0000"}}]`,
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetBatchSetStyle.DryRun(context.Background(), rt))
	if !strings.Contains(got, `styles_batch_update`) {
		t.Fatalf("DryRun URL missing styles_batch_update: %s", got)
	}
	if !strings.Contains(got, `"method":"PUT"`) {
		t.Fatalf("DryRun should use PUT: %s", got)
	}
}

func TestSheetBatchSetStyleExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/styles_batch_update",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"totalUpdatedCells": float64(4), "revision": float64(90),
		}},
	})
	err := mountAndRunSheets(t, SheetBatchSetStyle, []string{
		"+batch-set-style", "--spreadsheet-token", "shtTOKEN",
		"--data", `[{"ranges":["sheet1!A1:B2"],"style":{"font":{"bold":true}}}]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "totalUpdatedCells") {
		t.Fatalf("stdout missing totalUpdatedCells: %s", stdout.String())
	}
}

func TestSheetBatchSetStyleExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/styles_batch_update",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetBatchSetStyle, []string{
		"+batch-set-style", "--spreadsheet-token", "shtTOKEN",
		"--data", `[{"ranges":["sheet1!A1:B2"],"style":{}}]`, "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
