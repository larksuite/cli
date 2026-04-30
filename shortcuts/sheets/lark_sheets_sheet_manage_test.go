// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/tidwall/gjson"
)

func TestSheetCreateSheetValidateMissingToken(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "title": "Sheet 2"},
		nil, nil)
	err := SheetCreateSheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetManageValidateRejectsURLAndTokenTogether(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     map[string]string
	}{
		{
			name:     "create-sheet",
			shortcut: SheetCreateSheet,
			args: map[string]string{
				"url":               "https://example.feishu.cn/sheets/shtFromURL",
				"spreadsheet-token": "shtTOKEN",
				"title":             "Data",
			},
		},
		{
			name:     "copy-sheet",
			shortcut: SheetCopySheet,
			args: map[string]string{
				"url":               "https://example.feishu.cn/sheets/shtFromURL",
				"spreadsheet-token": "shtTOKEN",
				"sheet-id":          "sheet1",
				"title":             "Copy",
			},
		},
		{
			name:     "delete-sheet",
			shortcut: SheetDeleteSheet,
			args: map[string]string{
				"url":               "https://example.feishu.cn/sheets/shtFromURL",
				"spreadsheet-token": "shtTOKEN",
				"sheet-id":          "sheet1",
			},
		},
		{
			name:     "update-sheet",
			shortcut: SheetUpdateSheet,
			args: map[string]string{
				"url":               "https://example.feishu.cn/sheets/shtFromURL",
				"spreadsheet-token": "shtTOKEN",
				"sheet-id":          "sheet1",
				"title":             "Renamed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t, tt.args, nil, nil)
			err := tt.shortcut.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
				t.Fatalf("expected mutual exclusivity error, got: %v", err)
			}
		})
	}
}

func TestSheetCreateSheetValidateRejectsInvalidTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		title     string
		wantSubst string
	}{
		{name: "special chars", title: "bad/title", wantSubst: "must not contain"},
		{name: "empty", title: "", wantSubst: "must not be empty"},
		{name: "tab", title: "bad\ttitle", wantSubst: "tabs or line breaks"},
		{name: "newline", title: "bad\ntitle", wantSubst: "tabs or line breaks"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t,
				map[string]string{"spreadsheet-token": "sht1", "title": tt.title},
				nil, nil)
			err := SheetCreateSheet.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubst) {
				t.Fatalf("expected title error containing %q, got: %v", tt.wantSubst, err)
			}
		})
	}
}

func TestSheetCreateSheetValidateRejectsNegativeIndexWhenTitleProvided(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "sht1", "title": "Data"},
		map[string]int{"index": -1}, nil)
	err := SheetCreateSheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--index must be >= 0") {
		t.Fatalf("expected index validation error, got: %v", err)
	}
}

func TestSheetCopySheetValidateRejectsInvalidTitle(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "sht1", "sheet-id": "sheet1", "title": "bad\ttitle"},
		nil, nil)
	err := SheetCopySheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "tabs or line breaks") {
		t.Fatalf("expected title error, got: %v", err)
	}
}

func TestSheetCopySheetValidateRejectsNegativeIndexWhenTitleProvided(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "sht1", "sheet-id": "sheet1", "title": "Copy"},
		map[string]int{"index": -1}, nil)
	err := SheetCopySheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--index must be >= 0") {
		t.Fatalf("expected index validation error, got: %v", err)
	}
}

func TestSheetUpdateSheetValidateRejectsEmptyTitle(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "sht1", "sheet-id": "sheet1", "title": ""},
		nil, nil)
	err := SheetUpdateSheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("expected empty-title error, got: %v", err)
	}
}

func TestSheetCreateSheetDryRun(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "shtTOKEN", "title": "Data"},
		map[string]int{"index": 0}, nil)
	got := mustMarshalSheetsDryRun(t, SheetCreateSheet.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update"`) {
		t.Fatalf("DryRun URL mismatch: %s", got)
	}
	if !strings.Contains(got, `"addSheet"`) || !strings.Contains(got, `"title":"Data"`) || !strings.Contains(got, `"index":0`) {
		t.Fatalf("DryRun body mismatch: %s", got)
	}
}

func TestSheetCreateSheetExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"addSheet": map[string]interface{}{
							"properties": map[string]interface{}{
								"sheetId": "sheet_new",
								"title":   "Data",
								"index":   0,
							},
						},
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetCreateSheet, []string{
		"+create-sheet",
		"--spreadsheet-token", "shtTOKEN",
		"--title", "Data",
		"--index", "0",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gjson.Get(stdout.String(), "data.sheet_id").String() != "sheet_new" {
		t.Fatalf("stdout missing sheet_id: %s", stdout.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	requests, _ := body["requests"].([]interface{})
	if len(requests) != 1 {
		t.Fatalf("unexpected body: %#v", body)
	}
	req0, _ := requests[0].(map[string]interface{})
	addSheet, _ := req0["addSheet"].(map[string]interface{})
	props, _ := addSheet["properties"].(map[string]interface{})
	if props["title"] != "Data" {
		t.Fatalf("request title = %#v", props["title"])
	}
	if idx, ok := props["index"].(float64); !ok || idx != 0 {
		t.Fatalf("request index = %#v", props["index"])
	}
}

func TestSheetCopySheetValidateMissingSheetID(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "sht1", "sheet-id": ""},
		nil, nil)
	err := SheetCopySheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--sheet-id") {
		t.Fatalf("expected sheet-id error, got: %v", err)
	}
}

func TestSheetCopySheetDryRun(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1", "title": "Copy"},
		map[string]int{"index": 2}, nil)
	got := mustMarshalSheetsDryRun(t, SheetCopySheet.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update"`) {
		t.Fatalf("DryRun URL mismatch: %s", got)
	}
	if !strings.Contains(got, `"copySheet"`) || !strings.Contains(got, `"sheetId":"sheet1"`) || !strings.Contains(got, `"title":"Copy"`) {
		t.Fatalf("DryRun body mismatch: %s", got)
	}
	if !strings.Contains(got, `"[2] Move copied sheet to requested index"`) || !strings.Contains(got, `\u003ccopied_sheet_id\u003e`) || !strings.Contains(got, `"index":2`) {
		t.Fatalf("DryRun should describe follow-up move: %s", got)
	}
}

func TestSheetCopySheetExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	copyStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"copySheet": map[string]interface{}{
							"properties": map[string]interface{}{
								"sheetId": "sheet_copy",
								"title":   "Copy",
								"index":   1,
							},
						},
					},
				},
			},
		},
	}
	moveStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"updateSheet": map[string]interface{}{
							"properties": map[string]interface{}{
								"sheetId": "sheet_copy",
								"index":   2,
							},
						},
					},
				},
			},
		},
	}
	reg.Register(copyStub)
	reg.Register(moveStub)

	err := mountAndRunSheets(t, SheetCopySheet, []string{
		"+copy-sheet",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--title", "Copy",
		"--index", "2",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gjson.Get(stdout.String(), "data.sheet_id").String() != "sheet_copy" {
		t.Fatalf("stdout missing copied sheet id: %s", stdout.String())
	}
	if gjson.Get(stdout.String(), "data.sheet.index").Int() != 2 {
		t.Fatalf("stdout missing moved index: %s", stdout.String())
	}

	var copyBody map[string]interface{}
	if err := json.Unmarshal(copyStub.CapturedBody, &copyBody); err != nil {
		t.Fatalf("parse copy body: %v", err)
	}
	if !strings.Contains(string(copyStub.CapturedBody), `"copySheet"`) {
		t.Fatalf("copy request missing copySheet: %s", string(copyStub.CapturedBody))
	}
	if !strings.Contains(string(moveStub.CapturedBody), `"updateSheet"`) || !strings.Contains(string(moveStub.CapturedBody), `"index":2`) {
		t.Fatalf("move request mismatch: %s", string(moveStub.CapturedBody))
	}
}

func TestSheetCopySheetExecuteMoveFailureIncludesCopiedSheetRecovery(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"copySheet": map[string]interface{}{
							"properties": map[string]interface{}{
								"sheetId": "sheet_copy",
								"title":   "Copy",
								"index":   1,
							},
						},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1310211,
			"msg":  "wrong sheet id",
			"error": map[string]interface{}{
				"log_id": "log-move-failed",
			},
		},
	})

	err := mountAndRunSheets(t, SheetCopySheet, []string{
		"+copy-sheet",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--title", "Copy",
		"--index", "2",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected move failure, got nil")
	}

	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected *output.ExitError with detail, got %T: %v", err, err)
	}
	if exitErr.Detail.Code != 1310211 {
		t.Fatalf("error code = %d, want 1310211", exitErr.Detail.Code)
	}
	if !strings.Contains(exitErr.Detail.Message, `sheet copied successfully as "sheet_copy"`) {
		t.Fatalf("message missing copied sheet id: %q", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, "do not retry +copy-sheet") {
		t.Fatalf("hint missing retry guard: %q", exitErr.Detail.Hint)
	}
	if !strings.Contains(exitErr.Detail.Hint, "+update-sheet --spreadsheet-token shtTOKEN --sheet-id sheet_copy --index 2") {
		t.Fatalf("hint missing recovery command: %q", exitErr.Detail.Hint)
	}

	detail, _ := exitErr.Detail.Detail.(map[string]interface{})
	if detail["partial_success"] != true {
		t.Fatalf("partial_success = %#v, want true", detail["partial_success"])
	}
	if detail["sheet_id"] != "sheet_copy" {
		t.Fatalf("sheet_id = %#v, want %q", detail["sheet_id"], "sheet_copy")
	}
	if detail["requested_index"] != 2 {
		t.Fatalf("requested_index = %#v, want 2", detail["requested_index"])
	}
	if detail["retry_command"] != "lark-cli sheets +update-sheet --spreadsheet-token shtTOKEN --sheet-id sheet_copy --index 2" {
		t.Fatalf("retry_command = %#v", detail["retry_command"])
	}
	if detail["log_id"] != "log-move-failed" {
		t.Fatalf("log_id = %#v, want %q", detail["log_id"], "log-move-failed")
	}
}

func TestSheetDeleteSheetDryRun(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1"},
		nil, nil)
	got := mustMarshalSheetsDryRun(t, SheetDeleteSheet.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"POST"`) {
		t.Fatalf("DryRun should use POST: %s", got)
	}
	if !strings.Contains(got, `"/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update"`) {
		t.Fatalf("DryRun URL mismatch: %s", got)
	}
	if !strings.Contains(got, `"deleteSheet"`) || !strings.Contains(got, `"sheetId":"sheet1"`) {
		t.Fatalf("DryRun body mismatch: %s", got)
	}
}

func TestSheetDeleteSheetExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"deleteSheet": map[string]interface{}{
							"result":  true,
							"sheetId": "sheet1",
						},
					},
				},
			},
		},
	})

	err := mountAndRunSheets(t, SheetDeleteSheet, []string{
		"+delete-sheet",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--yes",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !gjson.Get(stdout.String(), "data.deleted").Bool() {
		t.Fatalf("stdout missing deleted=true: %s", stdout.String())
	}
	if gjson.Get(stdout.String(), "data.sheet_id").String() != "sheet1" {
		t.Fatalf("stdout missing sheet_id: %s", stdout.String())
	}
}

func TestSheetUpdateSheetValidateRequiresMutation(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1"},
		nil, nil)
	err := SheetUpdateSheet.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "specify at least one") {
		t.Fatalf("expected mutation error, got: %v", err)
	}
}

func TestSheetUpdateSheetValidateRejectsBadProtectionConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		strFlags  map[string]string
		intFlags  map[string]int
		wantSubst string
	}{
		{
			name: "lock-info requires lock",
			strFlags: map[string]string{
				"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1", "lock-info": "private",
			},
			wantSubst: "--lock when updating protection settings",
		},
		{
			name: "user-ids requires user-id-type",
			strFlags: map[string]string{
				"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1", "lock": "LOCK",
				"user-ids": `["ou_1"]`,
			},
			wantSubst: "--user-ids requires --user-id-type",
		},
		{
			name: "negative frozen rows rejected",
			strFlags: map[string]string{
				"spreadsheet-token": "shtTOKEN", "sheet-id": "sheet1",
			},
			intFlags:  map[string]int{"frozen-row-count": -1},
			wantSubst: "--frozen-row-count must be >= 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t, tt.strFlags, tt.intFlags, nil)
			err := SheetUpdateSheet.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubst) {
				t.Fatalf("want error containing %q, got: %v", tt.wantSubst, err)
			}
		})
	}
}

func TestSheetUpdateSheetDryRun(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t,
		map[string]string{
			"spreadsheet-token": "shtTOKEN",
			"sheet-id":          "sheet1",
			"title":             "Hidden Sheet",
			"lock":              "LOCK",
			"lock-info":         "private",
			"user-ids":          `["ou_1"]`,
			"user-id-type":      "open_id",
		},
		map[string]int{
			"index":            3,
			"frozen-row-count": 2,
			"frozen-col-count": 1,
		},
		map[string]bool{"hidden": false},
	)
	got := mustMarshalSheetsDryRun(t, SheetUpdateSheet.DryRun(context.Background(), rt))
	for _, want := range []string{
		`"/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update"`,
		`"user_id_type":"open_id"`,
		`"sheetId":"sheet1"`,
		`"title":"Hidden Sheet"`,
		`"index":3`,
		`"hidden":false`,
		`"frozenRowCount":2`,
		`"frozenColCount":1`,
		`"lock":"LOCK"`,
		`"lockInfo":"private"`,
		`"userIDs":["ou_1"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DryRun missing %s: %s", want, got)
		}
	}
}

func TestSheetUpdateSheetExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/sheets_batch_update?user_id_type=open_id",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"replies": []interface{}{
					map[string]interface{}{
						"updateSheet": map[string]interface{}{
							"properties": map[string]interface{}{
								"sheetId":        "sheet1",
								"title":          "Renamed",
								"index":          1,
								"hidden":         true,
								"frozenRowCount": 2,
								"frozenColCount": 1,
								"protect": map[string]interface{}{
									"lock":     "LOCK",
									"lockInfo": "private",
									"userIDs":  []interface{}{"ou_1"},
								},
							},
						},
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetUpdateSheet, []string{
		"+update-sheet",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--title", "Renamed",
		"--index", "1",
		"--hidden=true",
		"--frozen-row-count", "2",
		"--frozen-col-count", "1",
		"--lock", "LOCK",
		"--lock-info", "private",
		"--user-ids", `["ou_1"]`,
		"--user-id-type", "open_id",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gjson.Get(stdout.String(), "data.sheet_id").String() != "sheet1" {
		t.Fatalf("stdout missing sheet_id: %s", stdout.String())
	}
	if gjson.Get(stdout.String(), "data.sheet.title").String() != "Renamed" {
		t.Fatalf("stdout missing title: %s", stdout.String())
	}
	if gjson.Get(stdout.String(), "data.sheet.grid_properties.frozen_row_count").Int() != 2 {
		t.Fatalf("stdout missing frozen_row_count: %s", stdout.String())
	}
	if gjson.Get(stdout.String(), "data.sheet.protect.lock_info").String() != "private" {
		t.Fatalf("stdout missing lock_info: %s", stdout.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	requests, ok := body["requests"].([]interface{})
	if !ok || len(requests) != 1 {
		t.Fatalf("unexpected requests body: %#v", body)
	}
	req0, _ := requests[0].(map[string]interface{})
	updateSheet, _ := req0["updateSheet"].(map[string]interface{})
	props, _ := updateSheet["properties"].(map[string]interface{})
	if props["sheetId"] != "sheet1" || props["title"] != "Renamed" {
		t.Fatalf("unexpected properties: %#v", props)
	}
}

func TestBuildUpdateSheetOutputOmitsBlankTitleWhenTitleNotChanged(t *testing.T) {
	t.Parallel()

	out, ok := buildUpdateSheetOutput("shtTOKEN", map[string]interface{}{
		"replies": []interface{}{
			map[string]interface{}{
				"updateSheet": map[string]interface{}{
					"properties": map[string]interface{}{
						"sheetId":        "sheet1",
						"title":          "",
						"hidden":         false,
						"frozenRowCount": 0,
					},
				},
			},
		},
	}, false)
	if !ok {
		t.Fatal("expected output")
	}
	sheet, _ := out["sheet"].(map[string]interface{})
	if _, exists := sheet["title"]; exists {
		t.Fatalf("blank title should be omitted when title is unchanged: %#v", sheet)
	}
	if sheet["sheet_id"] != "sheet1" {
		t.Fatalf("unexpected sheet output: %#v", sheet)
	}
}
