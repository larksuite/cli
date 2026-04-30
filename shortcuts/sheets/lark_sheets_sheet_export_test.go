// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/tidwall/gjson"
)

func TestSheetExportValidateRejectsURLAndTokenTogether(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t, map[string]string{
		"url":               "https://example.feishu.cn/sheets/shtFromURL",
		"spreadsheet-token": "shtTOKEN",
		"file-extension":    "xlsx",
		"output-path":       "",
		"sheet-id":          "",
	}, nil, nil)
	err := SheetExport.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutual exclusivity error, got: %v", err)
	}
}

func TestSheetExportValidateRequiresSheetIDForCSV(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t, map[string]string{
		"url":               "",
		"spreadsheet-token": "shtTOKEN",
		"file-extension":    "csv",
		"output-path":       "",
		"sheet-id":          "",
	}, nil, nil)
	err := SheetExport.Validate(context.Background(), rt)
	if err == nil || !strings.Contains(err.Error(), "--sheet-id is required when --file-extension is csv") {
		t.Fatalf("expected csv sheet-id validation error, got: %v", err)
	}
}

func TestSheetExportValidateAllowsCSVWithSheetID(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t, map[string]string{
		"url":               "",
		"spreadsheet-token": "shtTOKEN",
		"file-extension":    "csv",
		"output-path":       "",
		"sheet-id":          "sheet1",
	}, nil, nil)
	if err := SheetExport.Validate(context.Background(), rt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetExportDryRunIncludesSubIDForCSV(t *testing.T) {
	t.Parallel()

	rt := newDimTestRuntime(t, map[string]string{
		"url":               "",
		"spreadsheet-token": "shtTOKEN",
		"file-extension":    "csv",
		"output-path":       "",
		"sheet-id":          "sheet1",
	}, nil, nil)
	got := mustMarshalSheetsDryRun(t, SheetExport.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"sub_id":"sheet1"`) {
		t.Fatalf("DryRun should include sub_id for csv export, got: %s", got)
	}
}

func TestSheetExportCommandRejectsInvalidFileExtension(t *testing.T) {
	t.Parallel()

	f, _, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetExport, []string{
		"+export",
		"--spreadsheet-token", "shtTOKEN",
		"--file-extension", "pdf",
		"--as", "user",
	}, f, nil)
	if err == nil || !strings.Contains(err.Error(), `allowed: xlsx, csv`) {
		t.Fatalf("expected invalid file-extension error, got: %v", err)
	}
}

func TestSheetExportExecuteWithoutOutputPathReturnsMetadataOnly(t *testing.T) {
	t.Parallel()

	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/export_tasks",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"ticket": "tk_123",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/export_tasks/tk_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"result": map[string]interface{}{
					"file_token": "box_123",
				},
			},
		},
	})

	err := mountAndRunSheets(t, SheetExport, []string{
		"+export",
		"--spreadsheet-token", "shtTOKEN",
		"--file-extension", "xlsx",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := stdout.String()
	if gjson.Get(got, "data.file_token").String() != "box_123" || gjson.Get(got, "data.ticket").String() != "tk_123" {
		t.Fatalf("stdout should return export metadata, got: %s", got)
	}
	if strings.Contains(got, `"saved_path"`) {
		t.Fatalf("stdout should not include saved_path when --output-path is omitted: %s", got)
	}
}
