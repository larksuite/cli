// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// ── SetDropdown ─────────────────────────────────────────────────────────────

func TestSetDropdownValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "",
		"range": "s1!A2:A100", "condition-values": `["opt1","opt2"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSetDropdownValidateInvalidConditionValues(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "s1!A2:A100", "condition-values": "not-json",
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--condition-values must be a JSON array") {
		t.Fatalf("expected JSON array error, got: %v", err)
	}
}

func TestSetDropdownValidateNonStringConditionValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
	}{
		{"mixed types", `["ok", 1, null]`},
		{"all numbers", `[1, 2, 3]`},
		{"null literal", `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newSheetsTestRuntime(t, map[string]string{
				"url": "", "spreadsheet-token": "sht1",
				"range": "s1!A2:A100", "condition-values": tc.input,
				"colors": "",
			}, map[string]bool{"multiple": false, "highlight": false})
			err := SheetSetDropdown.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), "--condition-values must be") {
				t.Fatalf("expected validation error for %q, got: %v", tc.input, err)
			}
		})
	}
}

func TestSetDropdownValidateInvalidColors(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "s1!A2:A100", "condition-values": `["opt1","opt2"]`,
		"colors": "bad-json",
	}, map[string]bool{"multiple": false, "highlight": true})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--colors must be a JSON array") {
		t.Fatalf("expected colors JSON error, got: %v", err)
	}
}

func TestSetDropdownValidateRangeMissingSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "A2:A100", "condition-values": `["opt1"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "fully qualified range") {
		t.Fatalf("expected range validation error, got: %v", err)
	}
}

func TestSetDropdownValidateEmptyConditionValues(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "s1!A2:A100", "condition-values": `[]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--condition-values must not be empty") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestSetDropdownValidateColorsMismatchLength(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "s1!A2:A100", "condition-values": `["a","b","c"]`,
		"colors": `["#FF0000"]`,
	}, map[string]bool{"multiple": false, "highlight": true})
	err := SheetSetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--colors length") {
		t.Fatalf("expected length mismatch error, got: %v", err)
	}
}

func TestSetDropdownValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1",
		"range": "s1!A2:A100", "condition-values": `["opt1","opt2"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	if err := SheetSetDropdown.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetDropdownDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test",
		"range": "s1!A2:A100", "condition-values": `["opt1","opt2"]`,
		"colors": "",
	}, map[string]bool{"multiple": true, "highlight": false})
	got := mustMarshalSheetsDryRun(t, SheetSetDropdown.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"POST"`) {
		t.Fatalf("DryRun should use POST: %s", got)
	}
	if !strings.Contains(got, `dataValidation`) {
		t.Fatalf("DryRun missing dataValidation: %s", got)
	}
	if !strings.Contains(got, `"dataValidationType":"list"`) {
		t.Fatalf("DryRun missing dataValidationType: %s", got)
	}
}

func TestSetDropdownExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetSetDropdown, []string{
		"+set-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--range", "s1!A2:A100", "--condition-values", `["opt1","opt2","opt3"]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetDropdownExecuteWithMultipleAndColors(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetSetDropdown, []string{
		"+set-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--range", "s1!A2:A100", "--condition-values", `["a","b"]`,
		"--multiple", "--highlight", "--colors", `["#1FB6C1","#F006C2"]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	dv, _ := body["dataValidation"].(map[string]interface{})
	opts, _ := dv["options"].(map[string]interface{})
	if opts["multipleValues"] != true {
		t.Fatalf("expected multipleValues=true, got: %v", opts["multipleValues"])
	}
	if opts["highlightValidData"] != true {
		t.Fatalf("expected highlightValidData=true, got: %v", opts["highlightValidData"])
	}
}

func TestSetDropdownExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetSetDropdown, []string{
		"+set-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--range", "s1!A2:A100", "--condition-values", `["opt1"]`,
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetDropdownWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v2/spreadsheets/shtFromURL/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetSetDropdown, []string{
		"+set-dropdown", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--range", "s1!A2:A100", "--condition-values", `["opt1"]`,
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── UpdateDropdown ──────────────────────────────────────────────────────────

func TestUpdateDropdownValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1",
		"ranges": `["s1!A1:A100"]`, "condition-values": `["opt1"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetUpdateDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestUpdateDropdownValidateInvalidRanges(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"ranges": "not-json", "condition-values": `["opt1"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetUpdateDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--ranges must be a JSON array") {
		t.Fatalf("expected JSON array error, got: %v", err)
	}
}

func TestUpdateDropdownValidateRangesMissingSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"ranges": `["A1:A100"]`, "condition-values": `["opt1"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetUpdateDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "fully qualified range") {
		t.Fatalf("expected range validation error, got: %v", err)
	}
}

func TestUpdateDropdownValidateEmptyRanges(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"ranges": `[]`, "condition-values": `["opt1"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	err := SheetUpdateDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--ranges must not be empty") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestUpdateDropdownValidateInvalidColors(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"ranges": `["s1!A1:A100"]`, "condition-values": `["opt1"]`,
		"colors": "{not-array}",
	}, map[string]bool{"multiple": false, "highlight": true})
	err := SheetUpdateDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--colors must be a JSON array") {
		t.Fatalf("expected colors JSON error, got: %v", err)
	}
}

func TestUpdateDropdownDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
		"ranges": `["sheet1!A1:A100"]`, "condition-values": `["new1","new2"]`,
		"colors": "",
	}, map[string]bool{"multiple": false, "highlight": false})
	got := mustMarshalSheetsDryRun(t, SheetUpdateDropdown.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"PUT"`) {
		t.Fatalf("DryRun should use PUT: %s", got)
	}
	if !strings.Contains(got, `sheet1`) {
		t.Fatalf("DryRun missing sheet_id: %s", got)
	}
}

func TestUpdateDropdownExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation/sheet1",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"spreadsheetToken": "shtTOKEN", "sheetId": "sheet1",
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateDropdown, []string{
		"+update-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--ranges", `["sheet1!A1:A100"]`,
		"--condition-values", `["new1","new2"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateDropdownWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v2/spreadsheets/shtFromURL/dataValidation/sheet1",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetUpdateDropdown, []string{
		"+update-dropdown", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--ranges", `["sheet1!A1:A100"]`,
		"--condition-values", `["opt1"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── GetDropdown ─────────────────────────────────────────────────────────────

func TestGetDropdownValidateRangeMissingSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "range": "A2:A100",
	}, nil)
	err := SheetGetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "fully qualified range") {
		t.Fatalf("expected range validation error, got: %v", err)
	}
}

func TestGetDropdownValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "range": "s1!A2:A100",
	}, nil)
	err := SheetGetDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestGetDropdownDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "range": "s1!A2:A100",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetGetDropdown.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"GET"`) {
		t.Fatalf("DryRun should use GET: %s", got)
	}
	if !strings.Contains(got, `dataValidation`) {
		t.Fatalf("DryRun missing dataValidation path: %s", got)
	}
}

func TestGetDropdownExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "Success", "data": map[string]interface{}{
			"dataValidations": []interface{}{
				map[string]interface{}{
					"dataValidationType": "list",
					"conditionValues":    []interface{}{"opt1", "opt2"},
					"ranges":             []interface{}{"s1!A2:A100"},
				},
			},
		}},
	})
	err := mountAndRunSheets(t, SheetGetDropdown, []string{
		"+get-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--range", "s1!A2:A100", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "dataValidations") {
		t.Fatalf("stdout missing dataValidations: %s", stdout.String())
	}
}

func TestGetDropdownWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v2/spreadsheets/shtFromURL/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "Success", "data": map[string]interface{}{
			"dataValidations": []interface{}{},
		}},
	})
	err := mountAndRunSheets(t, SheetGetDropdown, []string{
		"+get-dropdown", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--range", "s1!A2:A100", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── DeleteDropdown ──────────────────────────────────────────────────────────

func TestDeleteDropdownValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "ranges": `["s1!A2:A100"]`,
	}, nil)
	err := SheetDeleteDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestDeleteDropdownValidateRangesMissingSheetID(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "ranges": `["B1:B50"]`,
	}, nil)
	err := SheetDeleteDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "fully qualified range") {
		t.Fatalf("expected range validation error, got: %v", err)
	}
}

func TestDeleteDropdownValidateEmptyRanges(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "ranges": `[]`,
	}, nil)
	err := SheetDeleteDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--ranges must not be empty") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestDeleteDropdownValidateInvalidRanges(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "ranges": "bad",
	}, nil)
	err := SheetDeleteDropdown.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--ranges must be a JSON array") {
		t.Fatalf("expected JSON array error, got: %v", err)
	}
}

func TestDeleteDropdownDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "ranges": `["s1!A2:A100","s1!C1:C50"]`,
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetDeleteDropdown.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"DELETE"`) {
		t.Fatalf("DryRun should use DELETE: %s", got)
	}
	if !strings.Contains(got, `dataValidationRanges`) {
		t.Fatalf("DryRun missing dataValidationRanges: %s", got)
	}
}

func TestDeleteDropdownExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"rangeResults": []interface{}{
				map[string]interface{}{"range": "s1!A2:A100", "success": true, "updatedCells": 99},
			},
		}},
	})
	err := mountAndRunSheets(t, SheetDeleteDropdown, []string{
		"+delete-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--ranges", `["s1!A2:A100"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "rangeResults") {
		t.Fatalf("stdout missing rangeResults: %s", stdout.String())
	}
}

func TestDeleteDropdownExecuteMultipleRanges(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetDeleteDropdown, []string{
		"+delete-dropdown", "--spreadsheet-token", "shtTOKEN",
		"--ranges", `["s1!A2:A100","s1!C1:C50"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	dvRanges, _ := body["dataValidationRanges"].([]interface{})
	if len(dvRanges) != 2 {
		t.Fatalf("expected 2 ranges, got: %d", len(dvRanges))
	}
}

func TestDeleteDropdownWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v2/spreadsheets/shtFromURL/dataValidation",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteDropdown, []string{
		"+delete-dropdown", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--ranges", `["s1!A2:A100"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// suppress unused import for bytes in case the test helpers already import it
var _ = (*bytes.Buffer)(nil)
