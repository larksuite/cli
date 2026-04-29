// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestValidateDriveVersionHistorySpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		spec    driveVersionHistorySpec
		wantErr string
	}{
		{
			name: "ok",
			spec: driveVersionHistorySpec{FileToken: "box123", Limit: 20, Cursor: "1777013761763"},
		},
		{
			name:    "bad limit",
			spec:    driveVersionHistorySpec{FileToken: "box123", Limit: 0},
			wantErr: "invalid --limit",
		},
		{
			name:    "bad cursor",
			spec:    driveVersionHistorySpec{FileToken: "box123", Limit: 20, Cursor: "abc"},
			wantErr: "--cursor must be a numeric version string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateDriveVersionHistorySpec(tt.spec)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestDriveVersionHistoryExecuteTransformsResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/files/box_hist/history",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"version":      "7633658129540910621",
						"name":         "report.md",
						"edit_time":    "1777013761763",
						"edit_user_id": "ou_hist_1",
						"size":         "12345",
						"type":         1,
						"tag":          7,
					},
					{
						"version":      "7633658129540910622",
						"name":         "report.md",
						"edit_time":    "1777013770000",
						"edit_user_id": "ou_hist_2",
						"size":         "12346",
						"type":         4,
						"tag":          8,
					},
				},
				"has_more": true,
			},
		},
	})

	err := mountAndRunDrive(t, DriveVersionHistory, []string{
		"+version-history",
		"--file-token", "box_hist",
		"--limit", "2",
		"--cursor", "1777013000000",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}

	if got := common.GetBool(envelope.Data, "has_more"); !got {
		t.Fatalf("has_more = %v, want true", got)
	}
	if got := common.GetString(envelope.Data, "next_cursor"); got != "1777013770000" {
		t.Fatalf("next_cursor = %q, want %q", got, "1777013770000")
	}

	versions, _ := envelope.Data["versions"].([]interface{})
	if len(versions) != 2 {
		t.Fatalf("len(versions) = %d, want 2", len(versions))
	}
	first, _ := versions[0].(map[string]interface{})
	if got := common.GetString(first, "version"); got != "7633658129540910621" {
		t.Fatalf("first.version = %q", got)
	}
	if got := common.GetString(first, "action_type"); got != "upload" {
		t.Fatalf("first.action_type = %q, want upload", got)
	}
	second, _ := versions[1].(map[string]interface{})
	if got := common.GetString(second, "action_type"); got != "revert" {
		t.Fatalf("second.action_type = %q, want revert", got)
	}
}

func TestDriveVersionGetWritesSpecificVersion(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/box_ver/download?version=7633658129540910621",
		Status:  200,
		RawBody: []byte("versioned-data"),
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename="report-v7.md"`},
		},
	})

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	err := mountAndRunDrive(t, DriveVersionGet, []string{
		"+version-get",
		"--file-token", "box_ver",
		"--version", "7633658129540910621",
		"--output", "version.bin",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "version.bin"))
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	if string(data) != "versioned-data" {
		t.Fatalf("downloaded content = %q", string(data))
	}
	if !strings.Contains(stdout.String(), `"version": "7633658129540910621"`) {
		t.Fatalf("stdout missing version: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"saved_path":`) {
		t.Fatalf("stdout missing saved_path: %s", stdout.String())
	}
}

func TestDriveVersionRevertPostsVersionAndReturnsEmptyData(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	revertStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/box_rev/revert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(revertStub)

	err := mountAndRunDrive(t, DriveVersionRevert, []string{
		"+version-revert",
		"--file-token", "box_rev",
		"--version", "7633658129540910621",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, revertStub)
	if got := common.GetString(body, "version"); got != "7633658129540910621" {
		t.Fatalf("body.version = %q, want 7633658129540910621", got)
	}
	if !strings.Contains(stdout.String(), `"data": {}`) {
		t.Fatalf("stdout = %s, want empty data object", stdout.String())
	}
}

func TestDriveVersionDeletePostsVersionAndReturnsEmptyData(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	deleteStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/box_del/version_del",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(deleteStub)

	err := mountAndRunDrive(t, DriveVersionDelete, []string{
		"+version-delete",
		"--file-token", "box_del",
		"--version", "7633658129540910621",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := decodeCapturedJSONBody(t, deleteStub)
	if got := common.GetString(body, "version"); got != "7633658129540910621" {
		t.Fatalf("body.version = %q, want 7633658129540910621", got)
	}
	if !strings.Contains(stdout.String(), `"data": {}`) {
		t.Fatalf("stdout = %s, want empty data object", stdout.String())
	}
}

func TestDriveVersionShortcutsDoNotAcceptYes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "revert",
			shortcut: DriveVersionRevert,
			args: []string{
				"+version-revert",
				"--file-token", "box_rev",
				"--version", "7633658129540910621",
				"--yes",
				"--as", "bot",
			},
		},
		{
			name:     "delete",
			shortcut: DriveVersionDelete,
			args: []string{
				"+version-delete",
				"--file-token", "box_del",
				"--version", "7633658129540910621",
				"--yes",
				"--as", "bot",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, driveTestConfig())

			err := mountAndRunDrive(t, tt.shortcut, tt.args, f, nil)
			if err == nil {
				t.Fatal("expected unknown flag error, got nil")
			}
			if !strings.Contains(err.Error(), "unknown flag: --yes") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
