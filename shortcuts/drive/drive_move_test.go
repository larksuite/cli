// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveMoveUsesRootFolderWhenFolderTokenMissing(t *testing.T) {
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
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/file_token_test/move",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveMove, []string{
		"+move",
		"--file-token", "file_token_test",
		"--type", "file",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"folder_token": "folder_root_token_test"`) {
		t.Fatalf("stdout missing resolved root folder token: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"file_token": "file_token_test"`) {
		t.Fatalf("stdout missing file token: %s", stdout.String())
	}
}

func TestDriveMoveRootFolderLookupRequiresToken(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/explorer/v2/root_folder/meta",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRunDrive(t, DriveMove, []string{
		"+move",
		"--file-token", "file_token_test",
		"--type", "file",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing root folder token error, got nil")
	}
	if !strings.Contains(err.Error(), "root_folder/meta returned no token") {
		t.Fatalf("unexpected error: %v", err)
	}
}
