// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestDriveSecureLabelList_DryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--page-size", "5",
		"--page-token", "page_1",
		"--lang", "zh",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v2/my_secure_labels",
		`"GET"`,
		`"page_size": 5`,
		`"page_token": "page_1"`,
		`"lang": "zh"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDriveSecureLabelList_ValidatePageSize(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--page-size", "11",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "page-size") {
		t.Fatalf("expected page-size validation error, got: %v", err)
	}
}

func TestDriveSecureLabelList_ExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v2/my_secure_labels?page_size=10",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "7217780879644737540", "name": "L1"},
				},
			},
		},
	})

	err := mountAndRunDrive(t, DriveSecureLabelList, []string{
		"+secure-label-list",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"L1"`) {
		t.Fatalf("stdout missing label:\n%s", stdout.String())
	}
}

func TestDriveSecureLabelUpdate_DryRunInfersTypeFromURL(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "https://example.feishu.cn/docx/doxTok123?from=share",
		"--label-id", "7217780879644737539",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v2/files/doxTok123/secure_label",
		`"PATCH"`,
		`"docx"`,
		`"id": "7217780879644737539"`,
		`"file_token": "doxTok123"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDriveSecureLabelUpdate_ExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label?type=docx",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", "doxTok123",
		"--type", "docx",
		"--label-id", "7217780879644737539",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["id"] != "7217780879644737539" {
		t.Fatalf("id = %v, want label id", body["id"])
	}
}

func TestDriveSecureLabelUpdate_DowngradeApprovalReturnsAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/drive/v2/files/doxTok123/secure_label",
		Status: 403,
		Body: map[string]interface{}{
			"code": 1063013, "msg": "Security label downgrade requires approval",
		},
	})

	targetURL := "https://example.feishu.cn/docx/doxTok123"
	err := mountAndRunDrive(t, DriveSecureLabelUpdate, []string{
		"+secure-label-update",
		"--token", targetURL,
		"--label-id", "7217780879644737539",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected 1063013 error")
	}
	if !strings.Contains(err.Error(), "Security label downgrade requires approval") {
		t.Fatalf("expected raw API error message, got: %v", err)
	}
}
