// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"

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
	t.Parallel()

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
