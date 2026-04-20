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

// ── CreateFloatImage ────────────────────────────────────────────────────────

func TestCreateFloatImageValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1",
		"float-image-token": "boxToken", "range": "s1!A1:A1",
		"width": "", "height": "", "offset-x": "", "offset-y": "", "float-image-id": "",
	}, nil)
	err := SheetCreateFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestCreateFloatImageValidateSuccess(t *testing.T) {
	t.Parallel()
	// Pixel flags are int-typed by the shortcut; leave them unset (empty
	// intFlags map) so Cmd.Flags().Changed(...) returns false and
	// validateFloatImageDims doesn't try to read non-existent ints.
	rt := newDimTestRuntime(t,
		map[string]string{
			"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
			"float-image-token": "boxToken", "range": "s1!A1:A1", "float-image-id": "",
		}, nil, nil)
	if err := SheetCreateFloatImage.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateFloatImageValidateRejectsMultiCellRange(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"float-image-token": "boxToken", "range": "s1!A1:B2",
		"width": "", "height": "", "offset-x": "", "offset-y": "", "float-image-id": "",
	}, nil)
	err := SheetCreateFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "single cell") {
		t.Fatalf("expected single-cell error, got: %v", err)
	}
}

func TestCreateFloatImageValidateRejectsSheetIDMismatch(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
		"float-image-token": "boxToken", "range": "other!A1:A1",
		"width": "", "height": "", "offset-x": "", "offset-y": "", "float-image-id": "",
	}, nil)
	err := SheetCreateFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "does not match --sheet-id") {
		t.Fatalf("expected sheet-id mismatch error, got: %v", err)
	}
}

func TestCreateFloatImageValidateRejectsOutOfBoundsDims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		intFlags  map[string]int
		wantSubst string
	}{
		{"width below 20", map[string]int{"width": 5}, "--width must be >= 20"},
		{"height below 20", map[string]int{"height": 10}, "--height must be >= 20"},
		{"negative offset-x", map[string]int{"offset-x": -1}, "--offset-x must be >= 0"},
		{"negative offset-y", map[string]int{"offset-y": -5}, "--offset-y must be >= 0"},
	}

	baseStr := map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
		"float-image-token": "boxToken", "range": "s1!A1:A1", "float-image-id": "",
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t, baseStr, tt.intFlags, nil)
			err := SheetCreateFloatImage.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubst) {
				t.Fatalf("want error containing %q, got: %v", tt.wantSubst, err)
			}
		})
	}
}

func TestCreateFloatImageValidateAcceptsBoundaryDims(t *testing.T) {
	t.Parallel()
	// Boundary values exactly at the lower bound should pass.
	rt := newDimTestRuntime(t,
		map[string]string{
			"url": "", "spreadsheet-token": "sht1", "sheet-id": "s1",
			"float-image-token": "boxToken", "range": "s1!A1:A1", "float-image-id": "",
		},
		map[string]int{"width": 20, "height": 20, "offset-x": 0, "offset-y": 0}, nil)
	if err := SheetCreateFloatImage.Validate(context.Background(), rt); err != nil {
		t.Fatalf("boundary values should pass, got: %v", err)
	}
}

func TestCreateFloatImageDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{
			"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
			"float-image-token": "boxToken", "range": "sheet1!A1:A1", "float-image-id": "",
		},
		map[string]int{"width": 200, "height": 150}, nil)
	got := mustMarshalSheetsDryRun(t, SheetCreateFloatImage.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"POST"`) {
		t.Fatalf("DryRun should use POST: %s", got)
	}
	if !strings.Contains(got, `float_images`) {
		t.Fatalf("DryRun URL missing float_images: %s", got)
	}
	if !strings.Contains(got, `"float_image_token":"boxToken"`) {
		t.Fatalf("DryRun missing float_image_token: %s", got)
	}
	if !strings.Contains(got, `"width":200`) || !strings.Contains(got, `"height":150`) {
		t.Fatalf("DryRun should emit numeric width/height, got: %s", got)
	}
}

func TestCreateFloatImageExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	stub := &httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{
				"float_image_id": "fi12345678", "float_image_token": "boxToken",
				"range": "sheet1!A1:A1", "width": 200, "height": 150,
			},
		}},
	}
	reg.Register(stub)
	err := mountAndRunSheets(t, SheetCreateFloatImage, []string{
		"+create-float-image", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--float-image-token", "boxToken",
		"--range", "sheet1!A1:A1", "--width", "200", "--height", "150",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "float_image_id") {
		t.Fatalf("stdout missing float_image_id: %s", stdout.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["float_image_token"] != "boxToken" {
		t.Fatalf("unexpected float_image_token: %v", body["float_image_token"])
	}
	if w, ok := body["width"].(float64); !ok || w != 200 {
		t.Fatalf("width should be numeric 200, got %T=%v", body["width"], body["width"])
	}
	if h, ok := body["height"].(float64); !ok || h != 150 {
		t.Fatalf("height should be numeric 150, got %T=%v", body["height"], body["height"])
	}
}

func TestCreateFloatImageWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/float_images",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{"float_image_id": "fi12345678"},
		}},
	})
	err := mountAndRunSheets(t, SheetCreateFloatImage, []string{
		"+create-float-image", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--float-image-token", "boxToken",
		"--range", "sheet1!A1:A1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateFloatImageExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images",
		Status: 400, Body: map[string]interface{}{"code": 90001, "msg": "invalid"},
	})
	err := mountAndRunSheets(t, SheetCreateFloatImage, []string{
		"+create-float-image", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--float-image-token", "boxToken",
		"--range", "sheet1!A1:A1", "--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

// ── UpdateFloatImage ────────────────────────────────────────────────────────

func TestUpdateFloatImageValidateRejectsEmptyPayload(t *testing.T) {
	t.Parallel()
	// Only IDs set, no mutable field: PATCH would be an empty {} body.
	rt := newDimTestRuntime(t,
		map[string]string{
			"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
			"float-image-id": "fi123", "range": "",
		}, nil, nil)
	err := SheetUpdateFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "specify at least one of --range") {
		t.Fatalf("expected empty-payload error, got: %v", err)
	}
}

func TestUpdateFloatImageValidateAcceptsSingleField(t *testing.T) {
	t.Parallel()
	// Any single mutable field should satisfy the payload check.
	tests := []struct {
		name     string
		strFlags map[string]string
		intFlags map[string]int
	}{
		{
			name: "range only",
			strFlags: map[string]string{
				"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
				"float-image-id": "fi123", "range": "sheet1!B2:B2",
			},
		},
		{
			name: "offset-x only (zero value)",
			strFlags: map[string]string{
				"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
				"float-image-id": "fi123", "range": "",
			},
			intFlags: map[string]int{"offset-x": 0},
		},
	}
	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t, tt.strFlags, tt.intFlags, nil)
			if err := SheetUpdateFloatImage.Validate(context.Background(), rt); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateFloatImageValidateRejectsSheetIDMismatch(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
		"float-image-id": "fi123", "range": "other!A1:A1",
		"width": "", "height": "", "offset-x": "", "offset-y": "",
	}, nil)
	err := SheetUpdateFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "does not match --sheet-id") {
		t.Fatalf("expected sheet-id mismatch error, got: %v", err)
	}
}

func TestUpdateFloatImageValidateRejectsOutOfBoundsDims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		intFlags  map[string]int
		wantSubst string
	}{
		{"width below 20", map[string]int{"width": 19}, "--width must be >= 20"},
		{"height below 20", map[string]int{"height": 0}, "--height must be >= 20"},
		{"negative offset-x", map[string]int{"offset-x": -10}, "--offset-x must be >= 0"},
		{"negative offset-y", map[string]int{"offset-y": -1}, "--offset-y must be >= 0"},
	}

	baseStr := map[string]string{
		"url": "", "spreadsheet-token": "sht1", "sheet-id": "sheet1",
		"float-image-id": "fi123", "range": "",
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rt := newDimTestRuntime(t, baseStr, tt.intFlags, nil)
			err := SheetUpdateFloatImage.Validate(context.Background(), rt)
			if err == nil || !strings.Contains(err.Error(), tt.wantSubst) {
				t.Fatalf("want error containing %q, got: %v", tt.wantSubst, err)
			}
		})
	}
}

func TestUpdateFloatImageDryRun(t *testing.T) {
	t.Parallel()
	rt := newDimTestRuntime(t,
		map[string]string{
			"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
			"float-image-id": "fi12345678", "range": "sheet1!B2:B2",
		},
		map[string]int{"width": 300, "offset-y": 10}, nil)
	got := mustMarshalSheetsDryRun(t, SheetUpdateFloatImage.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"PATCH"`) {
		t.Fatalf("DryRun should use PATCH: %s", got)
	}
	if !strings.Contains(got, `fi12345678`) {
		t.Fatalf("DryRun missing float_image_id: %s", got)
	}
	if !strings.Contains(got, `"width":300`) || !strings.Contains(got, `"offset_y":10`) {
		t.Fatalf("DryRun should emit numeric width/offset_y, got: %s", got)
	}
}

func TestUpdateFloatImageExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{"float_image_id": "fi123", "width": 300},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFloatImage, []string{
		"+update-float-image", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--float-image-id", "fi123",
		"--width", "300", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateFloatImageWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{"float_image_id": "fi123"},
		}},
	})
	err := mountAndRunSheets(t, SheetUpdateFloatImage, []string{
		"+update-float-image", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--float-image-id", "fi123",
		"--range", "sheet1!C3:C3", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── GetFloatImage ───────────────────────────────────────────────────────────

func TestGetFloatImageValidateMissingToken(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "", "sheet-id": "s1", "float-image-id": "fi1",
	}, nil)
	err := SheetGetFloatImage.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestGetFloatImageDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "float-image-id": "fi123",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetGetFloatImage.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"GET"`) {
		t.Fatalf("DryRun should use GET: %s", got)
	}
	if !strings.Contains(got, `fi123`) {
		t.Fatalf("DryRun missing float_image_id: %s", got)
	}
}

func TestGetFloatImageExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{
				"float_image_id": "fi123", "range": "sheet1!A1:A1", "width": 100, "height": 100,
			},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFloatImage, []string{
		"+get-float-image", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--float-image-id", "fi123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "fi123") {
		t.Fatalf("stdout missing fi123: %s", stdout.String())
	}
}

func TestGetFloatImageWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"float_image": map[string]interface{}{"float_image_id": "fi123"},
		}},
	})
	err := mountAndRunSheets(t, SheetGetFloatImage, []string{
		"+get-float-image", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--float-image-id", "fi123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── ListFloatImages ─────────────────────────────────────────────────────────

func TestListFloatImagesDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetListFloatImages.DryRun(context.Background(), rt))
	if !strings.Contains(got, `float_images/query`) {
		t.Fatalf("DryRun URL missing query: %s", got)
	}
}

func TestListFloatImagesExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"float_image_id": "fi1"},
				map[string]interface{}{"float_image_id": "fi2"},
			},
		}},
	})
	err := mountAndRunSheets(t, SheetListFloatImages, []string{
		"+list-float-images", "--spreadsheet-token", "shtTOKEN", "--sheet-id", "sheet1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "fi1") {
		t.Fatalf("stdout missing fi1: %s", stdout.String())
	}
}

func TestListFloatImagesWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/float_images/query",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{"items": []interface{}{}}},
	})
	err := mountAndRunSheets(t, SheetListFloatImages, []string{
		"+list-float-images", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── DeleteFloatImage ────────────────────────────────────────────────────────

func TestDeleteFloatImageDryRun(t *testing.T) {
	t.Parallel()
	rt := newSheetsTestRuntime(t, map[string]string{
		"url": "", "spreadsheet-token": "sht_test", "sheet-id": "sheet1", "float-image-id": "fi123",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetDeleteFloatImage.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"method":"DELETE"`) {
		t.Fatalf("DryRun should use DELETE: %s", got)
	}
}

func TestDeleteFloatImageExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtTOKEN/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFloatImage, []string{
		"+delete-float-image", "--spreadsheet-token", "shtTOKEN",
		"--sheet-id", "sheet1", "--float-image-id", "fi123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteFloatImageWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "DELETE", URL: "/open-apis/sheets/v3/spreadsheets/shtFromURL/sheets/sheet1/float_images/fi123",
		Body: map[string]interface{}{"code": 0, "msg": "success", "data": map[string]interface{}{}},
	})
	err := mountAndRunSheets(t, SheetDeleteFloatImage, []string{
		"+delete-float-image", "--url", "https://example.feishu.cn/sheets/shtFromURL",
		"--sheet-id", "sheet1", "--float-image-id", "fi123", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
