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
				FileToken: "wiki_token_test",
				FileType:  "wiki",
			},
			wantErr: "underlying file token first",
		},
		{
			name: "folder",
			spec: driveCreateShortcutSpec{
				FileToken: "folder_token_test",
				FileType:  "folder",
			},
			wantErr: "not folders",
		},
		{
			name: "unknown",
			spec: driveCreateShortcutSpec{
				FileToken: "file_token_test",
				FileType:  "unknown",
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

func TestDriveCreateShortcutDryRunWithoutFolderTokenIncludesRootLookup(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "drive +create-shortcut"}
	cmd.Flags().String("file-token", "", "")
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("folder-token", "", "")
	if err := cmd.Flags().Set("file-token", "doc_token_test"); err != nil {
		t.Fatalf("set --file-token: %v", err)
	}
	if err := cmd.Flags().Set("type", "docx"); err != nil {
		t.Fatalf("set --type: %v", err)
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
	if len(got.API) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(got.API))
	}
	if got.API[0].Method != "GET" {
		t.Fatalf("first method = %q, want GET", got.API[0].Method)
	}
	if got.API[1].Method != "POST" {
		t.Fatalf("second method = %q, want POST", got.API[1].Method)
	}
	if got.API[1].Body["parent_token"] != "<root_folder_token>" {
		t.Fatalf("parent_token = %#v, want <root_folder_token>", got.API[1].Body["parent_token"])
	}
	referEntity, _ := got.API[1].Body["refer_entity"].(map[string]interface{})
	if referEntity["token"] != "doc_token_test" || referEntity["type"] != "docx" {
		t.Fatalf("unexpected refer_entity: %#v", referEntity)
	}
}

func TestDriveCreateShortcutUsesRootFolderWhenFolderTokenMissing(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"token": "folder_root_token_test",
			},
		},
	})
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/create_shortcut",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"token": "shortcut_token_test",
				"url":   "https://example.feishu.cn/drive/shortcut/shortcut_token_test",
			},
		},
	}
	reg.Register(createStub)

	err := mountAndRunDrive(t, DriveCreateShortcut, []string{
		"+create-shortcut",
		"--file-token", "doc_token_test",
		"--type", "docx",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, createStub)
	if body["parent_token"] != "folder_root_token_test" {
		t.Fatalf("parent_token = %#v, want folder_root_token_test", body["parent_token"])
	}
	referEntity, _ := body["refer_entity"].(map[string]interface{})
	if referEntity["token"] != "doc_token_test" || referEntity["type"] != "docx" {
		t.Fatalf("unexpected refer_entity: %#v", referEntity)
	}

	data := decodeDriveEnvelope(t, stdout)
	if data["shortcut_token"] != "shortcut_token_test" {
		t.Fatalf("shortcut_token = %#v, want shortcut_token_test", data["shortcut_token"])
	}
	if data["folder_token"] != "folder_root_token_test" {
		t.Fatalf("folder_token = %#v, want folder_root_token_test", data["folder_token"])
	}
	if data["source_file_token"] != "doc_token_test" {
		t.Fatalf("source_file_token = %#v, want doc_token_test", data["source_file_token"])
	}
	if data["created"] != true {
		t.Fatalf("created = %#v, want true", data["created"])
	}
}

func TestDriveCreateShortcutRootFolderLookupRequiresToken(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveCreateShortcut, []string{
		"+create-shortcut",
		"--file-token", "doc_token_test",
		"--type", "docx",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing root folder token error, got nil")
	}
	if !strings.Contains(err.Error(), "root_folder/meta returned no token") {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
			wantHint:    "avoid concurrent duplicate create-shortcut requests",
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
