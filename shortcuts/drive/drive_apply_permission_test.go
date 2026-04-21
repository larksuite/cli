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

// ── resolvePermApplyTarget unit tests ────────────────────────────────────────

func TestResolvePermApplyTarget_BareTokenNeedsType(t *testing.T) {
	t.Parallel()
	_, _, err := resolvePermApplyTarget("bareToken", "")
	if err == nil || !strings.Contains(err.Error(), "--type is required") {
		t.Fatalf("expected --type required error, got: %v", err)
	}
}

func TestResolvePermApplyTarget_BareTokenWithType(t *testing.T) {
	t.Parallel()
	token, docType, err := resolvePermApplyTarget("bareToken", "docx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "bareToken" || docType != "docx" {
		t.Fatalf("got token=%q type=%q, want bareToken/docx", token, docType)
	}
}

func TestResolvePermApplyTarget_URLInference(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		raw      string
		wantTok  string
		wantType string
	}{
		{"docx", "https://example.feishu.cn/docx/doxTok123?from=share", "doxTok123", "docx"},
		{"sheets", "https://example.feishu.cn/sheets/shtTok456?sheet=abc", "shtTok456", "sheet"},
		{"base", "https://example.feishu.cn/base/bscTok789", "bscTok789", "bitable"},
		{"bitable", "https://example.feishu.cn/bitable/bscTok789", "bscTok789", "bitable"},
		{"file", "https://example.feishu.cn/file/boxTok111", "boxTok111", "file"},
		{"wiki", "https://example.feishu.cn/wiki/wikTok222", "wikTok222", "wiki"},
		{"legacy doc", "https://example.feishu.cn/doc/docTok333", "docTok333", "doc"},
		{"mindnote", "https://example.feishu.cn/mindnote/mnTok444", "mnTok444", "mindnote"},
		{"slides", "https://example.feishu.cn/slides/slTok666", "slTok666", "slides"},
	}
	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			token, docType, err := resolvePermApplyTarget(tt.raw, "")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token != tt.wantTok || docType != tt.wantType {
				t.Fatalf("got (%q,%q), want (%q,%q)", token, docType, tt.wantTok, tt.wantType)
			}
		})
	}
}

func TestResolvePermApplyTarget_ExplicitTypeOverridesURL(t *testing.T) {
	t.Parallel()
	// Even though the URL marker is /docx/, an explicit --type wins.
	token, docType, err := resolvePermApplyTarget("https://example.feishu.cn/docx/doxTok123", "wiki")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "doxTok123" || docType != "wiki" {
		t.Fatalf("got (%q,%q), want (doxTok123,wiki)", token, docType)
	}
}

func TestResolvePermApplyTarget_UnrecognizedURL(t *testing.T) {
	t.Parallel()
	_, _, err := resolvePermApplyTarget("https://example.feishu.cn/unknown/xyz", "")
	if err == nil || !strings.Contains(err.Error(), "could not infer token") {
		t.Fatalf("expected infer error, got: %v", err)
	}
}

func TestResolvePermApplyTarget_Empty(t *testing.T) {
	t.Parallel()
	_, _, err := resolvePermApplyTarget("   ", "docx")
	if err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("expected token required error, got: %v", err)
	}
}

// ── shortcut integration tests ──────────────────────────────────────────────

func TestDriveApplyPermission_ValidateMissingToken(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission", "--perm", "view", "--type", "docx", "--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("expected token error, got: %v", err)
	}
}

func TestDriveApplyPermission_ValidateRejectsBadPerm(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "doxTok",
		"--type", "docx",
		"--perm", "full_access",
		"--as", "user",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--perm") {
		t.Fatalf("expected perm enum error, got: %v", err)
	}
}

func TestDriveApplyPermission_DryRunInfersTypeFromURL(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "https://example.feishu.cn/sheets/shtTok?sheet=abc",
		"--perm", "edit",
		"--remark", "please",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v1/permissions/shtTok/members/apply",
		`"POST"`,
		`"sheet"`,
		`"edit"`,
		`"please"`,
		`"shtTok"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run output missing %q:\n%s", want, out)
		}
	}
}

func TestDriveApplyPermission_ExecuteSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	// Stub URL includes "?type=docx" — the stub only matches when the request
	// URL contains that query, so this doubles as an assertion that the
	// shortcut emits the type query parameter.
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxTok123/members/apply?type=docx",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{"applied": true},
		},
	}
	reg.Register(stub)

	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "doxTok123",
		"--type", "docx",
		"--perm", "view",
		"--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body["perm"] != "view" {
		t.Fatalf("perm = %v, want view", body["perm"])
	}
	if _, hasRemark := body["remark"]; hasRemark {
		t.Fatalf("remark should be omitted when empty, got: %v", body["remark"])
	}
}

func TestDriveApplyPermission_ExecuteNotApplicableHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxTok/members/apply",
		Status: 400,
		Body: map[string]interface{}{
			"code": 1063007, "msg": "request not applicable",
		},
	})

	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "doxTok",
		"--type", "docx",
		"--perm", "view",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for 1063007")
	}
	if !strings.Contains(err.Error(), "not applicable") {
		t.Fatalf("expected surfaced server message, got: %v", err)
	}
}

func TestDriveApplyPermission_ExecuteRateLimitHint(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, driveTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/doxTok/members/apply",
		Status: 429,
		Body: map[string]interface{}{
			"code": 1063006, "msg": "quota exceeded",
		},
	})

	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "doxTok",
		"--type", "docx",
		"--perm", "view",
		"--as", "user",
	}, f, nil)
	if err == nil {
		t.Fatal("expected error for 1063006")
	}
}
