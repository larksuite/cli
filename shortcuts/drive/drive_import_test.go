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
