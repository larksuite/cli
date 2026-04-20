// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// TestValidateDriveCreateShortcutSpecRejectsUnsupportedTypes verifies unsupported source types are rejected early.
func TestValidateDriveCreateShortcutSpecRejectsUnsupportedTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    driveCreateShortcutSpec
		wantErr string
	}{
		{
			name: "wiki",
			spec: driveCreateShortcutSpec{
				FileToken:   "wiki_token_test",
				FileType:    "wiki",
				FolderToken: "target_folder_token_test",
			},
			wantErr: "underlying file token first",
		},
		{
			name: "folder",
			spec: driveCreateShortcutSpec{
				FileToken:   "folder_token_test",
				FileType:    "folder",
				FolderToken: "target_folder_token_test",
			},
			wantErr: "not folders",
		},
		{
			name: "shortcut",
			spec: driveCreateShortcutSpec{
				FileToken:   "shortcut_token_test",
				FileType:    "shortcut",
				FolderToken: "target_folder_token_test",
			},
			wantErr: "Supported types",
		},
		{
			name: "missing folder token",
			spec: driveCreateShortcutSpec{
				FileToken: "file_token_test",
				FileType:  "docx",
			},
			wantErr: "--folder-token must not be empty",
		},
		{
			name: "unknown",
			spec: driveCreateShortcutSpec{
				FileToken:   "file_token_test",
				FileType:    "unknown",
				FolderToken: "target_folder_token_test",
			},
			wantErr: "Supported types",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateDriveCreateShortcutSpec(tt.spec)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestDriveCreateShortcutDryRunIncludesSingleCreateRequest verifies dry-run only previews the create request.
func TestDriveCreateShortcutDryRunIncludesSingleCreateRequest(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +create-shortcut"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	if err := cmd.Flags().Set("file-token", " doc_token_test "); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", " DOCX "); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", " folder_target_token_test "); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	dry := DriveCreateShortcut.DryRun(context.Background(), runtime)
	if dry == nil {
		t.Fatal("DryRun returned nil")
	}

	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}

	var got struct {
		API []struct {
			Method string                 `json:"method"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run json: %v", err)
	}
	if len(got.API) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(got.API))
	}
	if got.API[0].Method != "POST" {
		t.Fatalf("first method = %q, want POST", got.API[0].Method)
	}
	if got.API[0].Body["parent_token"] != "folder_target_token_test" {
		t.Fatalf("parent_token = %#v, want folder_target_token_test", got.API[0].Body["parent_token"])
	}
	referEntity, _ := got.API[0].Body["refer_entity"].(map[string]interface{})
	if referEntity["refer_token"] != "doc_token_test" || referEntity["refer_type"] != "docx" {
		t.Fatalf("unexpected refer_entity: %#v", referEntity)
	}
}

// TestDriveCreateShortcutUsesProvidedFolderToken verifies execution uses the explicit target folder token.
func TestDriveCreateShortcutUsesProvidedFolderToken(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_shortcut",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"succ_shortcut_node": map[string]interface{}{
					"token":        "shortcut_token_test",
					"name":         "shortcut_name_test",
					"type":         "docx",
					"parent_token": "folder_target_token_test",
					"url":          "https://example.feishu.cn/docx/shortcut_token_test",
					"shortcut_info": map[string]interface{}{
						"target_type":  "docx",
						"target_token": "doc_token_test",
					},
				},
			},
		},
	}
	reg.Register(createStub)

	err := mountAndRunDrive(t, DriveCreateShortcut, []string{
		"+create-shortcut",
		"--file-token", " doc_token_test ",
		"--type", " DOCX ",
		"--folder-token", " folder_target_token_test ",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, createStub)
	if body["parent_token"] != "folder_target_token_test" {
		t.Fatalf("parent_token = %#v, want folder_target_token_test", body["parent_token"])
	}
	referEntity, _ := body["refer_entity"].(map[string]interface{})
	if referEntity["refer_token"] != "doc_token_test" || referEntity["refer_type"] != "docx" {
		t.Fatalf("unexpected refer_entity: %#v", referEntity)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["shortcut_token"] != "shortcut_token_test" {
		t.Fatalf("shortcut_token = %#v, want shortcut_token_test", data["shortcut_token"])
	}
	if data["folder_token"] != "folder_target_token_test" {
		t.Fatalf("folder_token = %#v, want folder_target_token_test", data["folder_token"])
	}
	if data["source_file_token"] != "doc_token_test" {
		t.Fatalf("source_file_token = %#v, want doc_token_test", data["source_file_token"])
	}
	if data["title"] != "shortcut_name_test" {
		t.Fatalf("title = %#v, want shortcut_name_test", data["title"])
	}
	if data["url"] != "https://example.feishu.cn/docx/shortcut_token_test" {
		t.Fatalf("url = %#v, want https://example.feishu.cn/docx/shortcut_token_test", data["url"])
	}
	if data["created"] != true {
		t.Fatalf("created = %#v, want true", data["created"])
	}
}

// TestDriveCreateShortcutValidateRequiresFolderToken verifies folder-token is mandatory.
func TestDriveCreateShortcutValidateRequiresFolderToken(t *testing.T) {
	err := validateDriveCreateShortcutSpec(driveCreateShortcutSpec{
		FileToken: "doc_token_test",
		FileType:  "docx",
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "--folder-token must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDriveCreateShortcutValidateRejectsWhitespaceOnlyFolderToken verifies runtime normalization rejects blank folder tokens.
func TestDriveCreateShortcutValidateRejectsWhitespaceOnlyFolderToken(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +create-shortcut"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	if err := cmd.Flags().Set("file-token", "doc_token_test"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", " DOCX "); err != nil {
		t.Fatalf("set --type: %v", err)
	}
	if err := cmd.Flags().Set("folder-token", "   "); err != nil {
		t.Fatalf("set --folder-token: %v", err)
	}

	runtime := common.TestNewRuntimeContext(cmd, nil)
	err := DriveCreateShortcut.Validate(context.Background(), runtime)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "--folder-token must not be empty") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestDriveCreateShortcutClassifiesKnownAPIConstraints verifies known API constraints surface as structured errors.
func TestDriveCreateShortcutClassifiesKnownAPIConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		code        int
		msg         string
		wantType    string
		wantHint    string
		wantMsgPart string
	}{
		{
			name:        "resource contention",
			code:        output.LarkErrDriveResourceContention,
			msg:         "resource contention occurred, please retry",
			wantType:    "conflict",
			wantHint:    "avoid concurrent duplicate requests",
			wantMsgPart: "resource contention occurred",
		},
		{
			name:        "cross tenant and unit",
			code:        output.LarkErrDriveCrossTenantUnit,
			msg:         "cross tenant and unit not support",
			wantType:    "cross_tenant_unit",
			wantHint:    "same tenant and region/unit",
			wantMsgPart: "cross tenant and unit not support",
		},
		{
			name:        "cross brand",
			code:        output.LarkErrDriveCrossBrand,
			msg:         "cross brand not support",
			wantType:    "cross_brand",
			wantHint:    "same brand environment",
			wantMsgPart: "cross brand not support",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/drive/v1/files/create_shortcut",
				Body: map[string]interface{}{
					"code": float64(tt.code),
					"msg":  tt.msg,
				},
			})

			err := mountAndRunDrive(t, DriveCreateShortcut, []string{
				"+create-shortcut",
				"--file-token", "doc_token_test",
				"--type", "docx",
				"--folder-token", "folder_token_test",
				"--as", "bot",
			}, f, nil)
			if err == nil {
				t.Fatal("expected API error, got nil")
			}

			var exitErr *output.ExitError
			if !errors.As(err, &exitErr) || exitErr.Detail == nil {
				t.Fatalf("expected structured exit error, got %v", err)
			}
			if exitErr.Code != output.ExitAPI {
				t.Fatalf("exit code = %d, want %d", exitErr.Code, output.ExitAPI)
			}
			if exitErr.Detail.Type != tt.wantType {
				t.Fatalf("type = %q, want %q", exitErr.Detail.Type, tt.wantType)
			}
			if exitErr.Detail.Code != tt.code {
				t.Fatalf("detail code = %d, want %d", exitErr.Detail.Code, tt.code)
			}
			if !strings.Contains(exitErr.Detail.Message, tt.wantMsgPart) {
				t.Fatalf("message = %q, want substring %q", exitErr.Detail.Message, tt.wantMsgPart)
			}
			if !strings.Contains(exitErr.Detail.Hint, tt.wantHint) {
				t.Fatalf("hint = %q, want substring %q", exitErr.Detail.Hint, tt.wantHint)
			}
		})
	}
}
