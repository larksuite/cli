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

// ── CreateFilterView ─────────────────────────────────────────────────────────

func TestCreateFilterViewValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1", "range": "s1!A1:H14",
		"filter-view-name": "", "filter-view-id": "",
	}, nil)
	err := SheetCreateFilterView.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestValidateFilterViewTokenRejectsURLAndTokenTogether(t *testing.T) {
	t.Parallel()

	rt := newSheetsTestRuntime(t, map[string]string{
		"url":               "https://example.feishu.cn/sheets/shtFromURL",
		"spreadsheet-token": "shtTOKEN",
		"sheet-id":          "s1",
		"range":             "s1!A1:H14",
		"filter-view-name":  "",
		"filter-view-id":    "",
	}, nil)
	_, err := validateFilterViewToken(rt)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusivity error, got: %v", err)
	}
}

func TestCreateFilterViewValidateRejectsEmptyRange(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "range": "",
		"filter-view-name": "", "filter-view-id": "",
	}, nil)
	err := SheetCreateFilterView.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--range must not be empty") {
		t.Fatalf("expected empty range error, got: %v", err)
	}
}

func TestCreateFilterViewValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "range": "s1!A1:H14",
		"filter-view-name": "", "filter-view-id": "",
	}, nil)
	if err := SheetCreateFilterView.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateFilterViewDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "range": "sheet1!A1:H14",
		"filter-view-name": "my view", "filter-view-id": "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetCreateFilterView.DryRun(context.Background(), rt))
	if !strings.Contains(got, `filter_views`) {
		t.Fatalf("DryRun URL missing filter_views: %s", got)
	}
	if !strings.Contains(got, `"filter_view_name":"my view"`) {
		t.Fatalf("DryRun missing name: %s", got)
	}
}

func TestCreateFilterViewExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "pH9hbVcCXA", "range": "sheet1!A1:H14"},
		}},
	})
	err := mountAndRunSheets(t, SheetCreateFilterView, []string{
		"+create-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--range", "sheet1!A1:H14", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "filter_view_id") {
		t.Fatalf("stdout missing filter_view_id: %s", stdout.String())
	}
}

func TestCreateFilterViewExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetCreateFilterView, []string{
		"+create-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--range", "sheet1!A1:H14", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── UpdateFilterView ─────────────────────────────────────────────────────────

func TestUpdateFilterViewDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
		"filter-view-id": "pH9hbVcCXA", "range": "sheet1!A1:J20", "filter-view-name": "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetUpdateFilterView.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"PATCH"`) {
		t.Fatalf("DryRun should use PATCH: %s", got)
	}
	if !strings.Contains(got, `pH9hbVcCXA`) {
		t.Fatalf("DryRun missing filter_view_id: %s", got)
	}
}

func TestUpdateFilterViewExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "fv123", "range": "sheet1!A1:J20"},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFilterView, []string{
		"+update-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv123", "--range", "sheet1!A1:J20", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFilterViewRejectsNoFields(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetUpdateFilterView, []string{
		"+update-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error when no update fields provided, got nil")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestUpdateFilterViewRejectsBlankFieldsOnly(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetUpdateFilterView, []string{
		"+update-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1",
		"--range", "", "--filter-view-name", "",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error when only blank update fields are provided, got nil")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ── ListFilterViews ──────────────────────────────────────────────────────────

func TestListFilterViewsDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetListFilterViews.DryRun(context.Background(), rt))
	if !strings.Contains(got, `filter_views/query`) {
		t.Fatalf("DryRun URL missing query: %s", got)
	}
}

func TestListFilterViewsExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"items": []interface{}{map[string]interface{}{"filter_view_id": "fv1"}},
		}},
	})
	err := mountAndRunSheets(t, SheetListFilterViews, []string{
		"+list-filter-views", "--spreadsheet-token", "shtTOKEN", "--sheet-id", "sheet1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "fv1") {
		t.Fatalf("stdout missing fv1: %s", stdout.String())
	}
}

// ── GetFilterView ────────────────────────────────────────────────────────────

func TestGetFilterViewDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "filter-view-id": "fv123",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetGetFilterView.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"GET"`) {
		t.Fatalf("DryRun should use GET: %s", got)
	}
	if !strings.Contains(got, `fv123`) {
		t.Fatalf("DryRun missing filter_view_id: %s", got)
	}
}

func TestGetFilterViewExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "fv123"},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFilterView, []string{
		"+get-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── DeleteFilterView ─────────────────────────────────────────────────────────

func TestDeleteFilterViewDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "filter-view-id": "fv123",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetDeleteFilterView.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"DELETE"`) {
		t.Fatalf("DryRun should use DELETE: %s", got)
	}
}

func TestDeleteFilterViewExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFilterView, []string{
		"+delete-filter-view", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── CreateFilterViewCondition ────────────────────────────────────────────────

func TestCreateFilterViewConditionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1", "filter-view-id": "fv1",
		"condition-id": "E", "filter-type": "number", "compare-type": "less", "expected": `["6"]`,
	}, nil)
	err := SheetCreateFilterViewCondition.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestCreateFilterViewConditionDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "filter-view-id": "fv1",
		"condition-id": "E", "filter-type": "number", "compare-type": "less", "expected": `["6"]`,
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetCreateFilterViewCondition.DryRun(context.Background(), rt))
	if !strings.Contains(got, `conditions`) {
		t.Fatalf("DryRun URL missing conditions: %s", got)
	}
	if !strings.Contains(got, `"condition_id":"E"`) {
		t.Fatalf("DryRun missing condition_id: %s", got)
	}
	if !strings.Contains(got, `"filter_type":"number"`) {
		t.Fatalf("DryRun missing filter_type: %s", got)
	}
}

func TestCreateFilterViewConditionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv1/conditions",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E", "filter_type": "number"},
		}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetCreateFilterViewCondition, []string{
		"+create-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1",
		"--condition-id", "E", "--filter-type", "number", "--compare-type", "less",
		"--expected", `["6"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["condition_id"] != "E" {
		t.Fatalf("unexpected condition_id: %v", body["condition_id"])
	}
}

// ── UpdateFilterViewCondition ────────────────────────────────────────────────

func TestUpdateFilterViewConditionDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "filter-view-id": "fv1",
		"condition-id": "E", "filter-type": "number", "compare-type": "between", "expected": `["2","10"]`,
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetUpdateFilterViewCondition.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"PUT"`) {
		t.Fatalf("DryRun should use PUT: %s", got)
	}
	if !strings.Contains(got, `"compare_type":"between"`) {
		t.Fatalf("DryRun missing compare_type: %s", got)
	}
}

func TestUpdateFilterViewConditionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E"},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFilterViewCondition, []string{
		"+update-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E",
		"--filter-type", "number", "--compare-type", "between", "--expected", `["2","10"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFilterViewConditionRejectsNoFields(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetUpdateFilterViewCondition, []string{
		"+update-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E",
		"--as", "user",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error when no update fields provided, got nil")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

// ── ListFilterViewConditions ─────────────────────────────────────────────────

func TestListFilterViewConditionsDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "filter-view-id": "fv1",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetListFilterViewConditions.DryRun(context.Background(), rt))
	if !strings.Contains(got, `conditions/query`) {
		t.Fatalf("DryRun URL missing conditions/query: %s", got)
	}
}

func TestListFilterViewConditionsExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv1/conditions/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"items": []interface{}{map[string]interface{}{"condition_id": "E"}},
		}},
	})
	err := mountAndRunSheets(t, SheetListFilterViewConditions, []string{
		"+list-filter-view-conditions", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── GetFilterViewCondition ───────────────────────────────────────────────────

func TestGetFilterViewConditionDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
		"filter-view-id": "fv1", "condition-id": "E",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetGetFilterViewCondition.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"GET"`) {
		t.Fatalf("DryRun should use GET: %s", got)
	}
}

func TestGetFilterViewConditionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E", "filter_type": "number"},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFilterViewCondition, []string{
		"+get-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── DeleteFilterViewCondition ────────────────────────────────────────────────

func TestDeleteFilterViewConditionDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
		"filter-view-id": "fv1", "condition-id": "E",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetDeleteFilterViewCondition.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"DELETE"`) {
		t.Fatalf("DryRun should use DELETE: %s", got)
	}
}

func TestDeleteFilterViewConditionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFilterViewCondition, []string{
		"+delete-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── URL flag coverage ────────────────────────────────────────────────────────

func TestCreateFilterViewWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "fv1"},
		}},
	})
	err := mountAndRunSheets(t, SheetCreateFilterView, []string{
		"+create-filter-view", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--range", "sheet1!A1:H14", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListFilterViewsWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"items": []interface{}{}}},
	})
	err := mountAndRunSheets(t, SheetListFilterViews, []string{
		"+list-filter-views", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFilterViewWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "fv1"},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFilterView, []string{
		"+get-filter-view", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFilterViewWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"filter_view": map[string]interface{}{"filter_view_id": "fv1"},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFilterView, []string{
		"+update-filter-view", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--range", "sheet1!A1:J20", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFilterViewWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFilterView, []string{
		"+delete-filter-view", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateFilterViewConditionWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1/conditions",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E"},
		}},
	})
	err := mountAndRunSheets(t, SheetCreateFilterViewCondition, []string{
		"+create-filter-view-condition", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1",
		"--condition-id", "E", "--filter-type", "number", "--compare-type", "less",
		"--expected", `["6"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFilterViewConditionWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E"},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFilterViewCondition, []string{
		"+update-filter-view-condition", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E",
		"--filter-type", "number", "--expected", `["5"]`, "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListFilterViewConditionsWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1/conditions/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"items": []interface{}{}}},
	})
	err := mountAndRunSheets(t, SheetListFilterViewConditions, []string{
		"+list-filter-view-conditions", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFilterViewConditionWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"condition": map[string]interface{}{"condition_id": "E"},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFilterViewCondition, []string{
		"+get-filter-view-condition", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFilterViewConditionWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/filter_views/fv1/conditions/E",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFilterViewCondition, []string{
		"+delete-filter-view-condition", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--filter-view-id", "fv1", "--condition-id", "E", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── --expected validation rejects non-array input ────────────────────────────

func TestCreateFilterViewConditionRejectsNonArrayExpected(t *testing.T) {
	cases := []struct {
		name     string
		expected string
	}{
		{"plain string", "hello"},
		{"JSON object", `{"key":"val"}`},
		{"JSON number", "42"},
		{"JSON string", `"hello"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
			err := mountAndRunSheets(t, SheetCreateFilterViewCondition, []string{
				"+create-filter-view-condition", "--spreadsheet-token", "shtTOKEN",
				"--sheet-id", "sheet1", "--filter-view-id", "fv1",
				"--condition-id", "A", "--filter-type", "text", "--compare-type", "contains",
				"--expected", tc.expected, "--as", "user",
			}, f, stdout)
			if err == nil {
				t.Fatalf("expected validation error for --expected=%q, got nil", tc.expected)
			}
			if !strings.Contains(err.Error(), "--expected must be a JSON array") {
				t.Fatalf("unexpected error message: %v", err)
			}
		})
	}
}
