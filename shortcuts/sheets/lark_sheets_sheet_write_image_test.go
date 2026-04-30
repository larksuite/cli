// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func sheetsTestConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "sheets-test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
}

func mountAndRunSheets(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "sheets"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

const existingWriteImageTestFile = "./lark_sheets_cell_images.go"

// ── Validate ─────────────────────────────────────────────────────────────────

func TestSheetWriteImageValidateRequiresToken(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"image": "./logo.png",
		"range": "A1",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--url or --spreadsheet-token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestSheetWriteImageValidateAcceptsURL(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"url":      "https://example.larksuite.com/sheets/shtABC123",
		"image":    existingWriteImageTestFile,
		"range":    "sheetId!A1:A1",
		"sheet-id": "",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageValidateAcceptsSpreadsheetToken(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "shtABC123",
		"image":             existingWriteImageTestFile,
		"range":             "sheetId!A1:A1",
		"sheet-id":          "",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageValidateRejectsRelativeRangeWithoutSheetID(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "shtABC123",
		"image":             "./logo.png",
		"range":             "A1",
		"sheet-id":          "",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "--sheet-id") {
		t.Fatalf("expected sheet-id error, got: %v", err)
	}
}

func TestSheetWriteImageValidateAcceptsRelativeRangeWithSheetID(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "shtABC123",
		"image":             existingWriteImageTestFile,
		"range":             "A1",
		"sheet-id":          "sheet1",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageValidateRejectsMultiCellRange(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "shtABC123",
		"image":             "./logo.png",
		"range":             "sheet1!A1:B2",
		"sheet-id":          "",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err == nil || !strings.Contains(err.Error(), "single cell") {
		t.Fatalf("expected single cell error, got: %v", err)
	}
}

func TestSheetWriteImageValidateAcceptsSameCellSpan(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "shtABC123",
		"image":             existingWriteImageTestFile,
		"range":             "sheet1!A1:A1",
		"sheet-id":          "",
	}, nil)
	err := SheetWriteImage.Validate(context.Background(), runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── DryRun ───────────────────────────────────────────────────────────────────

func TestSheetWriteImageDryRun(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             "sheet1!B2",
		"sheet-id":          "",
		"image":             "./chart.png",
		"name":              "",
		"url":               "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetWriteImage.DryRun(context.Background(), runtime))

	if !strings.Contains(got, `"range":"sheet1!B2:B2"`) {
		t.Fatalf("DryRun range not normalized: %s", got)
	}
	if !strings.Contains(got, `"name":"chart.png"`) {
		t.Fatalf("DryRun name not derived from image path: %s", got)
	}
	// JSON escapes < and > to \u003c and \u003e.
	if !strings.Contains(got, `binary: ./chart.png`) {
		t.Fatalf("DryRun image field not showing binary placeholder: %s", got)
	}
	if !strings.Contains(got, `"description":"JSON upload with inline image bytes"`) {
		t.Fatalf("DryRun description incorrect: %s", got)
	}
}

func TestSheetWriteImageDryRunCustomName(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             "sheet1!A1:A1",
		"sheet-id":          "",
		"image":             "./output.png",
		"name":              "revenue_chart.png",
		"url":               "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetWriteImage.DryRun(context.Background(), runtime))

	if !strings.Contains(got, `"name":"revenue_chart.png"`) {
		t.Fatalf("DryRun should use custom name: %s", got)
	}
}

func TestSheetWriteImageDryRunUsesURL(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "",
		"range":             "sheet1!C3",
		"sheet-id":          "",
		"image":             "./logo.png",
		"name":              "",
		"url":               "https://example.larksuite.com/sheets/shtFromURL",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetWriteImage.DryRun(context.Background(), runtime))

	if !strings.Contains(got, `shtFromURL`) {
		t.Fatalf("DryRun should extract token from URL: %s", got)
	}
	if !strings.Contains(got, `"range":"sheet1!C3:C3"`) {
		t.Fatalf("DryRun range not normalized: %s", got)
	}
}

func TestSheetWriteImageDryRunWithSheetID(t *testing.T) {
	t.Parallel()
	runtime := newSheetsTestRuntime(t, map[string]string{
		"spreadsheet-token": "sht_test",
		"range":             "A1",
		"sheet-id":          "mySheet",
		"image":             "./img.png",
		"name":              "",
		"url":               "",
	}, nil)
	got := mustMarshalSheetsDryRun(t, SheetWriteImage.DryRun(context.Background(), runtime))

	if !strings.Contains(got, `"range":"mySheet!A1:A1"`) {
		t.Fatalf("DryRun should normalize relative range with sheet-id: %s", got)
	}
}

func TestSheetWriteImageDryRunRejectsMissingFile(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./missing.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "image file not found") {
		t.Fatalf("expected file-not-found error before dry-run planning, got: %v", err)
	}
}

func TestSheetWriteImageDryRunRejectsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)
	if err := os.Mkdir("imgdir", 0o755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./imgdir",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("expected regular-file error before dry-run planning, got: %v", err)
	}
}

func TestSheetWriteImageDryRunRejectsAbsolutePath(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "/etc/passwd",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "unsafe image path") {
		t.Fatalf("expected unsafe-path error before dry-run planning, got: %v", err)
	}
}

func TestSheetWriteImageDryRunRejectsOversizedFile(t *testing.T) {
	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	fh, err := os.Create("huge.png")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(20*1024*1024 + 1); err != nil {
		fh.Close()
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())
	err = mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./huge.png",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "exceeds 20MB limit") {
		t.Fatalf("expected size error before dry-run planning, got: %v", err)
	}
}

// ── Execute ──────────────────────────────────────────────────────────────────

func TestSheetWriteImageExecuteSendsJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/values_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"spreadsheetToken": "shtTOKEN",
				"revision":         float64(5),
				"updateRange":      "sheet1!A1:A1",
			},
		},
	}
	reg.Register(stub)

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	// Create a small test image file.
	imgData := []byte{0x89, 0x50, 0x4E, 0x47} // PNG magic bytes
	if err := os.WriteFile("test.png", imgData, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./test.png",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the request was sent as JSON (not multipart/form-data).
	if stub.CapturedHeaders == nil {
		t.Fatal("request headers not captured")
	}
	ct := stub.CapturedHeaders.Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	// Verify the captured body contains the image as base64 in JSON.
	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if body["range"] != "sheet1!A1:A1" {
		t.Fatalf("body range = %v, want sheet1!A1:A1", body["range"])
	}
	if body["name"] != "test.png" {
		t.Fatalf("body name = %v, want test.png", body["name"])
	}
	if body["image"] == nil {
		t.Fatal("body image field is nil")
	}

	// Verify output contains expected fields.
	if !strings.Contains(stdout.String(), "spreadsheetToken") {
		t.Fatalf("stdout missing spreadsheetToken: %s", stdout.String())
	}
}

func TestSheetWriteImageExecuteRejectsNonexistentFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./nonexistent.png",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageExecuteRejectsDirectory(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	// Create a directory where the image path points.
	if err := os.Mkdir("not_a_file", 0755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./not_a_file",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for directory, got nil")
	}
	if !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageExecuteWithURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtFromURL/values_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"spreadsheetToken": "shtFromURL",
				"revision":         float64(1),
				"updateRange":      "sheet1!B2:B2",
			},
		},
	}
	reg.Register(stub)

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	if err := os.WriteFile("pic.png", []byte{0x89, 0x50, 0x4E, 0x47}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--url", "https://example.larksuite.com/sheets/shtFromURL",
		"--range", "sheet1!B2:B2",
		"--image", "./pic.png",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "shtFromURL") {
		t.Fatalf("stdout missing token: %s", stdout.String())
	}
}

func TestSheetWriteImageExecuteCustomName(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/values_image",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"spreadsheetToken": "shtTOKEN",
				"revision":         float64(2),
				"updateRange":      "sheet1!A1:A1",
			},
		},
	}
	reg.Register(stub)

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	if err := os.WriteFile("raw.png", []byte{0x89, 0x50, 0x4E, 0x47}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./raw.png",
		"--name", "custom_chart.png",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("request body is not valid JSON: %v", err)
	}
	if body["name"] != "custom_chart.png" {
		t.Fatalf("body name = %v, want custom_chart.png", body["name"])
	}
}

func TestSheetWriteImageExecuteAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, sheetsTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/sheets/v2/spreadsheets/shtTOKEN/values_image",
		Status: 400,
		Body: map[string]interface{}{
			"code": 90001,
			"msg":  "invalid range",
		},
	})

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	if err := os.WriteFile("bad.png", []byte{0x89, 0x50}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./bad.png",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected API error, got nil")
	}
}

func TestSheetWriteImageExecuteRejectsOversizedFile(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	// Create a sparse file that reports > 20MB without writing actual data.
	fh, err := os.Create("huge.png")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(21 * 1024 * 1024); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	fh.Close()

	err = mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "./huge.png",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds 20MB limit") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSheetWriteImageExecuteRejectsAbsolutePath(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, sheetsTestConfig())

	tmpDir := t.TempDir()
	cmdutil.TestChdir(t, tmpDir)

	if err := os.WriteFile("abs.png", []byte{0x89, 0x50}, 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	err := mountAndRunSheets(t, SheetWriteImage, []string{
		"+write-image",
		"--spreadsheet-token", "shtTOKEN",
		"--range", "sheet1!A1:A1",
		"--image", "/etc/passwd",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe image path") {
		t.Fatalf("unexpected error: %v", err)
	}
}
