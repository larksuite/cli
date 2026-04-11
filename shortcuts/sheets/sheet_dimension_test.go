// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// newDimTestRuntime creates a RuntimeContext with string, int, and bool flags.
func newDimTestRuntime(t *testing.T, strFlags map[string]string, intFlags map[string]int, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	for name := range strFlags {
		cmd.Flags().String(name, "", "")
	}
	for name := range intFlags {
		cmd.Flags().Int(name, 0, "")
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, value := range strFlags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range intFlags {
		if err := cmd.Flags().Set(name, strconv.Itoa(value)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range boolFlags {
		if err := cmd.Flags().Set(name, strconv.FormatBool(value)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func marshalDryRun(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(b)
}

// ── AddDimension ─────────────────────────────────────────────────────────────

func TestSheetAddDimensionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"length": 10}, nil)
	err := SheetAddDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetAddDimensionValidateLengthOutOfRange(t *testing.T) {
	t.Parallel()
	for _, length := range []int{0, -1, 5001} {
		rt := newDimTestRuntime(t,
			map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
			map[string]int{"length": length}, nil)
		err := SheetAddDimension.Validate(context.Background(), rt)
		if err == nil || !strings.Contains(err.Error(), "--length") {
			t.Fatalf("length=%d: expected length error, got: %v", length, err)
		}
	}
}

func TestSheetAddDimensionValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"length": 100}, nil)
	if err := SheetAddDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetAddDimensionValidateWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtABC", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"length": 5}, nil)
	if err := SheetAddDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetAddDimensionDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS"},
		map[string]int{"length": 8}, nil)
	got := marshalDryRun(t, SheetAddDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `dimension_range`) {
		t.Fatalf("DryRun URL missing dimension_range: %s", got)
	}
	if !strings.Contains(got, `"sheetId":"sheet1"`) {
		t.Fatalf("DryRun missing sheetId: %s", got)
	}
	if !strings.Contains(got, `"majorDimension":"ROWS"`) {
		t.Fatalf("DryRun missing majorDimension: %s", got)
	}
	if !strings.Contains(got, `"length":8`) {
		t.Fatalf("DryRun missing length: %s", got)
	}
}

func TestSheetAddDimensionDryRunWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"length": 3}, nil)
	got := marshalDryRun(t, SheetAddDimension.DryRun(context.Background(), rt))
	if !strings.Contains(got, "shtFromURL") {
		t.Fatalf("DryRun should extract token from URL: %s", got)
	}
}

func TestSheetAddDimensionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Body: map[string]interface{}{
			"code": 0, "msg": "Success",
			"data": map[string]interface{}{"addCount": float64(8), "majorDimension": "ROWS"},
		},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetAddDimension, []string{
		"+add-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--length", "8",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"addCount"`) {
		t.Fatalf("stdout missing addCount: %s", stdout.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	dim, _ := body["dimension"].(map[string]interface{})
	if dim["sheetId"] != "sheet1" || dim["majorDimension"] != "ROWS" {
		t.Fatalf("unexpected request body: %#v", body)
	}
}

func TestSheetAddDimensionExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Status: 400,
		Body:   map[string]interface{}{"code": 90001, "msg": "invalid request"},
	})

	err := mountAndRunSheets(t, SheetAddDimension, []string{
		"+add-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--length", "8",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ── InsertDimension ──────────────────────────────────────────────────────────

func TestSheetInsertDimensionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS", "inherit-style": ""},
		map[string]int{"start-index": 0, "end-index": 3}, nil)
	err := SheetInsertDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetInsertDimensionValidateNegativeStartIndex(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS", "inherit-style": ""},
		map[string]int{"start-index": -1, "end-index": 3}, nil)
	err := SheetInsertDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--start-index") {
		t.Fatalf("expected start-index error, got: %v", err)
	}
}

func TestSheetInsertDimensionValidateEndNotGreaterThanStart(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS", "inherit-style": ""},
		map[string]int{"start-index": 5, "end-index": 5}, nil)
	err := SheetInsertDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--end-index") {
		t.Fatalf("expected end-index error, got: %v", err)
	}
}

func TestSheetInsertDimensionValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "COLUMNS", "inherit-style": ""},
		map[string]int{"start-index": 0, "end-index": 4}, nil)
	if err := SheetInsertDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetInsertDimensionDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS", "inherit-style": "BEFORE"},
		map[string]int{"start-index": 3, "end-index": 7}, nil)
	got := marshalDryRun(t, SheetInsertDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `insert_dimension_range`) {
		t.Fatalf("DryRun URL missing insert_dimension_range: %s", got)
	}
	if !strings.Contains(got, `"startIndex":3`) {
		t.Fatalf("DryRun missing startIndex: %s", got)
	}
	if !strings.Contains(got, `"endIndex":7`) {
		t.Fatalf("DryRun missing endIndex: %s", got)
	}
	if !strings.Contains(got, `"inheritStyle":"BEFORE"`) {
		t.Fatalf("DryRun missing inheritStyle: %s", got)
	}
}

func TestSheetInsertDimensionDryRunNoInheritStyle(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "COLUMNS", "inherit-style": ""},
		map[string]int{"start-index": 0, "end-index": 2}, nil)
	got := marshalDryRun(t, SheetInsertDimension.DryRun(context.Background(), rt))

	if strings.Contains(got, `inheritStyle`) {
		t.Fatalf("DryRun should omit inheritStyle when empty: %s", got)
	}
}

func TestSheetInsertDimensionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/insert_dimension_range",
		Body:   map[string]interface{}{"code": 0, "msg": "Success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetInsertDimension, []string{
		"+insert-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "3",
		"--end-index", "7",
		"--inherit-style", "AFTER",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	dim, _ := body["dimension"].(map[string]interface{})
	if dim["sheetId"] != "sheet1" || dim["majorDimension"] != "ROWS" {
		t.Fatalf("unexpected dimension: %#v", dim)
	}
	if body["inheritStyle"] != "AFTER" {
		t.Fatalf("unexpected inheritStyle: %v", body["inheritStyle"])
	}
}

func TestSheetInsertDimensionExecuteWithoutInheritStyle(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/insert_dimension_range",
		Body:   map[string]interface{}{"code": 0, "msg": "Success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetInsertDimension, []string{
		"+insert-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "COLUMNS",
		"--start-index", "0",
		"--end-index", "2",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	if _, ok := body["inheritStyle"]; ok {
		t.Fatalf("inheritStyle should be absent when not specified: %#v", body)
	}
}

func TestSheetInsertDimensionExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/insert_dimension_range",
		Status: 400,
		Body:   map[string]interface{}{"code": 90001, "msg": "invalid request"},
	})

	err := mountAndRunSheets(t, SheetInsertDimension, []string{
		"+insert-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "0",
		"--end-index", "3",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ── UpdateDimension ──────────────────────────────────────────────────────────

func TestSheetUpdateDimensionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3, "fixed-size": 50},
		map[string]bool{"visible": true})
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateStartIndexLessThan1(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 3, "fixed-size": 50},
		map[string]bool{"visible": true})
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--start-index") {
		t.Fatalf("expected start-index error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateEndLessThanStart(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 5, "end-index": 3, "fixed-size": 50},
		map[string]bool{"visible": true})
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--end-index") {
		t.Fatalf("expected end-index error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateNoProperties(t *testing.T) {
	t.Parallel()
	// Neither --visible nor --fixed-size is set (Changed returns false)
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3}, nil)
	// Register the flags but don't set them so Changed() returns false
	rt.Cmd.Flags().Bool("visible", false, "")
	rt.Cmd.Flags().Int("fixed-size", 0, "")
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--visible or --fixed-size") {
		t.Fatalf("expected properties error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateSuccessWithVisible(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3},
		map[string]bool{"visible": true})
	// Ensure fixed-size flag exists but is not set
	rt.Cmd.Flags().Int("fixed-size", 0, "")
	if err := SheetUpdateDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetUpdateDimensionValidateFixedSizeZero(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3, "fixed-size": 0}, nil)
	rt.Cmd.Flags().Bool("visible", false, "")
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--fixed-size must be >= 1") {
		t.Fatalf("expected fixed-size error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateFixedSizeNegative(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3, "fixed-size": -10}, nil)
	rt.Cmd.Flags().Bool("visible", false, "")
	err := SheetUpdateDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--fixed-size must be >= 1") {
		t.Fatalf("expected fixed-size error, got: %v", err)
	}
}

func TestSheetUpdateDimensionValidateSuccessWithFixedSize(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 1, "end-index": 5, "fixed-size": 120}, nil)
	// Ensure visible flag exists but is not set
	rt.Cmd.Flags().Bool("visible", false, "")
	if err := SheetUpdateDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetUpdateDimensionDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3, "fixed-size": 50},
		map[string]bool{"visible": true})
	got := marshalDryRun(t, SheetUpdateDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `"method":"PUT"`) {
		t.Fatalf("DryRun should use PUT: %s", got)
	}
	if !strings.Contains(got, `dimension_range`) {
		t.Fatalf("DryRun URL missing dimension_range: %s", got)
	}
	if !strings.Contains(got, `"visible":true`) {
		t.Fatalf("DryRun missing visible: %s", got)
	}
	if !strings.Contains(got, `"fixedSize":50`) {
		t.Fatalf("DryRun missing fixedSize: %s", got)
	}
}

func TestSheetUpdateDimensionDryRunOnlyVisible(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3},
		map[string]bool{"visible": false})
	// Add fixed-size flag but don't set it
	rt.Cmd.Flags().Int("fixed-size", 0, "")
	got := marshalDryRun(t, SheetUpdateDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `"visible":false`) {
		t.Fatalf("DryRun missing visible: %s", got)
	}
	if strings.Contains(got, `fixedSize`) {
		t.Fatalf("DryRun should omit fixedSize when not set: %s", got)
	}
}

func TestSheetUpdateDimensionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Body:   map[string]interface{}{"code": 0, "msg": "Success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetUpdateDimension, []string{
		"+update-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "1",
		"--end-index", "3",
		"--visible=true",
		"--fixed-size", "50",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	props, _ := body["dimensionProperties"].(map[string]interface{})
	if props["visible"] != true {
		t.Fatalf("expected visible=true, got: %#v", props)
	}
	if props["fixedSize"] != float64(50) {
		t.Fatalf("expected fixedSize=50, got: %#v", props)
	}
}

func TestSheetUpdateDimensionExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Status: 400,
		Body:   map[string]interface{}{"code": 90001, "msg": "invalid request"},
	})

	err := mountAndRunSheets(t, SheetUpdateDimension, []string{
		"+update-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "1",
		"--end-index", "3",
		"--visible=true",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ── MoveDimension ────────────────────────────────────────────────────────────

func TestSheetMoveDimensionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 1, "destination-index": 4}, nil)
	err := SheetMoveDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetMoveDimensionValidateNegativeStartIndex(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": -1, "end-index": 1, "destination-index": 4}, nil)
	err := SheetMoveDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--start-index") {
		t.Fatalf("expected start-index error, got: %v", err)
	}
}

func TestSheetMoveDimensionValidateEndLessThanStart(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 5, "end-index": 3, "destination-index": 0}, nil)
	err := SheetMoveDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--end-index") {
		t.Fatalf("expected end-index error, got: %v", err)
	}
}

func TestSheetMoveDimensionValidateNegativeDestinationIndex(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 1, "destination-index": -1}, nil)
	err := SheetMoveDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--destination-index") {
		t.Fatalf("expected destination-index error, got: %v", err)
	}
}

func TestSheetMoveDimensionValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 0, "end-index": 2, "destination-index": 5}, nil)
	if err := SheetMoveDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetMoveDimensionValidateWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtABC", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 1, "destination-index": 4}, nil)
	if err := SheetMoveDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetMoveDimensionDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 1, "destination-index": 4}, nil)
	got := marshalDryRun(t, SheetMoveDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `move_dimension`) {
		t.Fatalf("DryRun URL missing move_dimension: %s", got)
	}
	if !strings.Contains(got, `"major_dimension":"ROWS"`) {
		t.Fatalf("DryRun missing major_dimension: %s", got)
	}
	if !strings.Contains(got, `"start_index":0`) {
		t.Fatalf("DryRun missing start_index: %s", got)
	}
	if !strings.Contains(got, `"destination_index":4`) {
		t.Fatalf("DryRun missing destination_index: %s", got)
	}
}

func TestSheetMoveDimensionDryRunWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "", "sheet-id": "sheet1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 1, "end-index": 3, "destination-index": 0}, nil)
	got := marshalDryRun(t, SheetMoveDimension.DryRun(context.Background(), rt))
	if !strings.Contains(got, "shtFromURL") {
		t.Fatalf("DryRun should extract token from URL: %s", got)
	}
}

func TestSheetMoveDimensionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/move_dimension",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetMoveDimension, []string{
		"+move-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "0",
		"--end-index", "1",
		"--destination-index", "4",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	source, _ := body["source"].(map[string]interface{})
	if source["major_dimension"] != "ROWS" {
		t.Fatalf("unexpected major_dimension: %v", source["major_dimension"])
	}
	if body["destination_index"] != float64(4) {
		t.Fatalf("unexpected destination_index: %v", body["destination_index"])
	}
}

func TestSheetMoveDimensionExecuteWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/move_dimension",
		Body:   map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})

	err := mountAndRunSheets(t, SheetMoveDimension, []string{
		"+move-dimension",
		"--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1",
		"--dimension", "COLUMNS",
		"--start-index", "1",
		"--end-index", "2",
		"--destination-index", "0",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetMoveDimensionExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/move_dimension",
		Status: 400,
		Body:   map[string]interface{}{"code": 1310211, "msg": "wrong sheet id"},
	})

	err := mountAndRunSheets(t, SheetMoveDimension, []string{
		"+move-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "0",
		"--end-index", "1",
		"--destination-index", "4",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

// ── DeleteDimension ──────────────────────────────────────────────────────────

func TestSheetDeleteDimensionValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 1, "end-index": 3}, nil)
	err := SheetDeleteDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetDeleteDimensionValidateStartIndexLessThan1(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 0, "end-index": 3}, nil)
	err := SheetDeleteDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--start-index") {
		t.Fatalf("expected start-index error, got: %v", err)
	}
}

func TestSheetDeleteDimensionValidateEndLessThanStart(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 5, "end-index": 3}, nil)
	err := SheetDeleteDimension.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--end-index") {
		t.Fatalf("expected end-index error, got: %v", err)
	}
}

func TestSheetDeleteDimensionValidateSuccess(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1", "dimension": "ROWS"},
		map[string]int{"start-index": 3, "end-index": 7}, nil)
	if err := SheetDeleteDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetDeleteDimensionValidateWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtABC", "spreadsheet-token": "", "sheet-id": "s1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 1, "end-index": 2}, nil)
	if err := SheetDeleteDimension.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetDeleteDimensionDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "dimension": "ROWS"},
		map[string]int{"start-index": 3, "end-index": 7}, nil)
	got := marshalDryRun(t, SheetDeleteDimension.DryRun(context.Background(), rt))

	if !strings.Contains(got, `"method":"DELETE"`) {
		t.Fatalf("DryRun should use DELETE: %s", got)
	}
	if !strings.Contains(got, `dimension_range`) {
		t.Fatalf("DryRun URL missing dimension_range: %s", got)
	}
	if !strings.Contains(got, `"startIndex":3`) {
		t.Fatalf("DryRun missing startIndex: %s", got)
	}
	if !strings.Contains(got, `"endIndex":7`) {
		t.Fatalf("DryRun missing endIndex: %s", got)
	}
}

func TestSheetDeleteDimensionDryRunWithURL(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{"url": "https://example.feishu.cn/sheets/shtFromURL", "spreadsheet-token": "", "sheet-id": "sheet1", "dimension": "COLUMNS"},
		map[string]int{"start-index": 1, "end-index": 5}, nil)
	got := marshalDryRun(t, SheetDeleteDimension.DryRun(context.Background(), rt))
	if !strings.Contains(got, "shtFromURL") {
		t.Fatalf("DryRun should extract token from URL: %s", got)
	}
}

func TestSheetDeleteDimensionExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"delCount": float64(5), "majorDimension": "ROWS"},
		},
	}
	reg.Register(stub)

	err := mountAndRunSheets(t, SheetDeleteDimension, []string{
		"+delete-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "3",
		"--end-index", "7",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"delCount"`) {
		t.Fatalf("stdout missing delCount: %s", stdout.String())
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	dim, _ := body["dimension"].(map[string]interface{})
	if dim["sheetId"] != "sheet1" || dim["majorDimension"] != "ROWS" {
		t.Fatalf("unexpected dimension: %#v", dim)
	}
}

func TestSheetDeleteDimensionExecuteWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtFromURL/dimension_range",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"delCount": float64(2), "majorDimension": "COLUMNS"},
		},
	})

	err := mountAndRunSheets(t, SheetDeleteDimension, []string{
		"+delete-dimension",
		"--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1",
		"--dimension", "COLUMNS",
		"--start-index", "1",
		"--end-index", "2",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetDeleteDimensionExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/dimension_range",
		Status: 400,
		Body:   map[string]interface{}{"code": 90001, "msg": "invalid request"},
	})

	err := mountAndRunSheets(t, SheetDeleteDimension, []string{
		"+delete-dimension",
		"--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1",
		"--dimension", "ROWS",
		"--start-index", "3",
		"--end-index", "7",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}
