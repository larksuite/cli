// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	_ "github.com/larksuite/cli/internal/vfs/localfileio"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestImportDefaultFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "strip xlsx extension",
			filePath: "/tmp/base-import.xlsx",
			want:     "base-import",
		},
		{
			name:     "strip last extension only",
			filePath: "/tmp/report.final.csv",
			want:     "report.final",
		},
		{
			name:     "keep name without extension",
			filePath: "/tmp/README",
			want:     "README",
		},
		{
			name:     "keep hidden file name when trim would be empty",
			filePath: "/tmp/.env",
			want:     ".env",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := importDefaultFileName(tt.filePath); got != tt.want {
				t.Fatalf("importDefaultFileName(%q) = %q, want %q", tt.filePath, got, tt.want)
			}
		})
	}
}

func TestImportTargetFileName(t *testing.T) {
	t.Parallel()

	if got := importTargetFileName("/tmp/base-import.xlsx", "custom-name.xlsx"); got != "custom-name.xlsx" {
		t.Fatalf("explicit name should win, got %q", got)
	}
	if got := importTargetFileName("/tmp/base-import.xlsx", ""); got != "base-import" {
		t.Fatalf("default import name = %q, want %q", got, "base-import")
	}
}

func TestDriveImportDryRunUsesExtensionlessDefaultName(t *testing.T) {
	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.WriteFile("base-import.xlsx", []byte("fake-xlsx"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	cmd := &cobra.Command{Use: "drive +import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./base-import.xlsx"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("type", "bitable"); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", "fld_test"); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveImport.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Body map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 3 {
		t.Fatalf("expected 3 API calls, got %d", len(got.API))
	}

	uploadName, _ := got.API[0].Body["file_name"].(string)
	if uploadName != "base-import.xlsx" {
		t.Fatalf("upload file_name = %q, want %q", uploadName, "base-import.xlsx")
	}

	importName, _ := got.API[1].Body["file_name"].(string)
	if importName != "base-import" {
		t.Fatalf("import task file_name = %q, want %q", importName, "base-import")
	}
}

func TestDriveImportDryRunShowsMultipartUploadForLargeFile(t *testing.T) {
	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.xlsx")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(common.MaxDriveMediaUploadSinglePartSize + 1); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	cmd := &cobra.Command{Use: "drive +import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./large.xlsx"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("type", "sheet"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveImport.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Method string `json:"method"`
			URL    string `json:"url"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 5 {
		t.Fatalf("expected 5 API calls, got %d", len(got.API))
	}
	if got.API[0].URL != "/open-apis/drive/v1/medias/upload_prepare" {
		t.Fatalf("dry-run first URL = %q, want upload_prepare", got.API[0].URL)
	}
	if got.API[1].URL != "/open-apis/drive/v1/medias/upload_part" {
		t.Fatalf("dry-run second URL = %q, want upload_part", got.API[1].URL)
	}
	if got.API[2].URL != "/open-apis/drive/v1/medias/upload_finish" {
		t.Fatalf("dry-run third URL = %q, want upload_finish", got.API[2].URL)
	}
}

func TestDriveImportDryRunReturnsErrorForUnsafePath(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "../outside.md"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("type", "docx"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveImport.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API   []struct{} `json:"api"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if got.Error == "" || !strings.Contains(got.Error, "unsafe file path") {
		t.Fatalf("dry-run error = %q, want unsafe file path error", got.Error)
	}
	if len(got.API) != 0 {
		t.Fatalf("expected no API calls when preflight fails, got %d", len(got.API))
	}
}

func TestDriveImportDryRunReturnsErrorForOversizedMarkdown(t *testing.T) {
	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	fh, err := os.Create("large.md")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := fh.Truncate(driveImport20MBFileSizeLimit + 5*1024*1024); err != nil {
		t.Fatalf("Truncate() error: %v", err)
	}
	if err := fh.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	cmd := &cobra.Command{Use: "drive +import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./large.md"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("type", "docx"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveImport.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API   []struct{} `json:"api"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if got.Error == "" || !strings.Contains(got.Error, "exceeds 20.0 MB import limit for .md") {
		t.Fatalf("dry-run error = %q, want oversized markdown error", got.Error)
	}
	if len(got.API) != 0 {
		t.Fatalf("expected no API calls when size preflight fails, got %d", len(got.API))
	}
}

func TestDriveImportDryRunReturnsErrorForDirectoryInput(t *testing.T) {
	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.Mkdir("folder-input", 0755); err != nil {
		t.Fatalf("Mkdir() error: %v", err)
	}

	cmd := &cobra.Command{Use: "drive +import"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	cmd.Flags().String("name", "", "")
	if err := cmd.Flags().Set("file", "./folder-input"); err != nil {
		t.Fatalf("set --file: %v", err)
	}
	if err := cmd.Flags().Set("type", "docx"); err != nil {
		t.Fatalf("set --type: %v", err)
	}

	runtime := common.TestNewRuntimeContextWithCtx(context.Background(), cmd, nil)
	dry := DriveImport.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API   []struct{} `json:"api"`
		Error string     `json:"error"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if got.Error == "" || !strings.Contains(got.Error, "file must be a regular file") {
		t.Fatalf("dry-run error = %q, want regular file error", got.Error)
	}
	if len(got.API) != 0 {
		t.Fatalf("expected no API calls when file type preflight fails, got %d", len(got.API))
	}
}

func TestDriveImportCreateTaskBodyKeepsEmptyMountKeyForRoot(t *testing.T) {
	t.Parallel()

	spec := driveImportSpec{
		FilePath: "/tmp/README.md",
		DocType:  "docx",
	}

	body := spec.CreateTaskBody("file_token_test")
	point, ok := body["point"].(map[string]interface{})
	if !ok {
		t.Fatalf("point = %#v, want map", body["point"])
	}

	raw, exists := point["mount_key"]
	if !exists {
		t.Fatal("mount_key missing; want empty string for root import")
	}
	got, ok := raw.(string)
	if !ok {
		t.Fatalf("mount_key type = %T, want string", raw)
	}
	if got != "" {
		t.Fatalf("mount_key = %q, want empty string for root import", got)
	}

	spec.FolderToken = "fld_test"
	body = spec.CreateTaskBody("file_token_test")
	point, ok = body["point"].(map[string]interface{})
	if !ok {
		t.Fatalf("point = %#v, want map", body["point"])
	}
	if got, _ := point["mount_key"].(string); got != "fld_test" {
		t.Fatalf("mount_key = %q, want %q", got, "fld_test")
	}
}

// driveImportMockEnv mounts the three stubs needed for a full +import run:
// media upload_all -> import_tasks (create) -> import_tasks/<ticket> (poll).
// Returns nothing; caller asserts on stdout via decodeDriveEnvelope.
func driveImportMockEnv(t *testing.T, reg *httpmock.Registry, ticket string, pollData map[string]interface{}) {
	t.Helper()
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"file_token": "file_import_media"},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/import_tasks",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"ticket": ticket},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/import_tasks/" + ticket,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"result": pollData},
		},
	})
}

// driveImportTestConfig builds a CliConfig for the import fallback tests.
// The brand defaults to BrandFeishu when omitted; pass core.BrandLark to
// exercise the larksuite.com branch of BuildResourceURL.
func driveImportTestConfig(suffix string, brands ...core.LarkBrand) *core.CliConfig {
	brand := core.BrandFeishu
	if len(brands) > 0 {
		brand = brands[0]
	}
	return &core.CliConfig{
		AppID:     "drive-import-fallback-" + suffix,
		AppSecret: "test-secret",
		Brand:     brand,
	}
}

func TestDriveImportFallbackURLWhenBackendOmitsIt(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveImportTestConfig("missing-url"))
	driveImportMockEnv(t, reg, "ticket_fallback", map[string]interface{}{
		"token":      "doxcn_imported",
		"type":       "docx",
		"job_status": float64(0),
		// "url" deliberately omitted: import API frequently returns the doc
		// without an absolute URL, leaving the CLI to backfill from token.
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.WriteFile("notes.md", []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := mountAndRunDrive(t, DriveImport, []string{
		"+import", "--file", "notes.md", "--type", "docx", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("import should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got, want := data["url"], "https://www.feishu.cn/docx/doxcn_imported"; got != want {
		t.Fatalf("data.url = %#v, want %q (brand-standard fallback)", got, want)
	}
}

func TestDriveImportPreservesBackendURL(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveImportTestConfig("preserve-url"))
	driveImportMockEnv(t, reg, "ticket_preserve", map[string]interface{}{
		"token":      "doxcn_imported",
		"type":       "docx",
		"job_status": float64(0),
		"url":        "https://tenant.larkoffice.com/docx/doxcn_imported",
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.WriteFile("notes.md", []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := mountAndRunDrive(t, DriveImport, []string{
		"+import", "--file", "notes.md", "--type", "docx", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("import should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got, want := data["url"], "https://tenant.larkoffice.com/docx/doxcn_imported"; got != want {
		t.Fatalf("data.url = %#v, want backend tenant URL %q (fallback must not overwrite)", got, want)
	}
}

func TestDriveImportFallbackURLWhenServerURLIsWhitespace(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveImportTestConfig("whitespace-url"))
	driveImportMockEnv(t, reg, "ticket_whitespace", map[string]interface{}{
		"token":      "doxcn_imported",
		"type":       "docx",
		"job_status": float64(0),
		"url":        "   ", // whitespace-only must trigger fallback, not pass through.
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.WriteFile("notes.md", []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := mountAndRunDrive(t, DriveImport, []string{
		"+import", "--file", "notes.md", "--type", "docx", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("import should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got, want := data["url"], "https://www.feishu.cn/docx/doxcn_imported"; got != want {
		t.Fatalf("data.url = %#v, want %q (whitespace-only backend URL must yield fallback)", got, want)
	}
}

func TestDriveImportFallbackURLForLarkBrand(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveImportTestConfig("lark-brand", core.BrandLark))
	driveImportMockEnv(t, reg, "ticket_lark", map[string]interface{}{
		"token":      "doxcn_imported",
		"type":       "docx",
		"job_status": float64(0),
		// "url" omitted to force the fallback through the lark host branch.
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.WriteFile("notes.md", []byte("# Hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := mountAndRunDrive(t, DriveImport, []string{
		"+import", "--file", "notes.md", "--type", "docx", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("import should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	if got, want := data["url"], "https://www.larksuite.com/docx/doxcn_imported"; got != want {
		t.Fatalf("data.url = %#v, want %q (lark brand fallback)", got, want)
	}
}

func TestDriveImportFallbackURLWhenServerTypeIsAlias(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveImportTestConfig("alias-type"))
	driveImportMockEnv(t, reg, "ticket_alias", map[string]interface{}{
		"token":      "shtcn_imported",
		"type":       "sheets", // non-canonical alias the server may return
		"job_status": float64(0),
	})

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origDir)
	if err := os.WriteFile("data.csv", []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := mountAndRunDrive(t, DriveImport, []string{
		"+import", "--file", "data.csv", "--type", "sheet", "--as", "user",
	}, f, stdout); err != nil {
		t.Fatalf("import should succeed, got: %v", err)
	}

	data := decodeDriveEnvelope(t, stdout)
	// Server returned "sheets" (alias) — normalize falls back to the user
	// --type "sheet", so BuildResourceURL picks the canonical /sheets/ path.
	if got, want := data["url"], "https://www.feishu.cn/sheets/shtcn_imported"; got != want {
		t.Fatalf("data.url = %#v, want %q (alias normalized via spec.DocType fallback)", got, want)
	}
}
