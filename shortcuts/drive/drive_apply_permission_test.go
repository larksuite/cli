// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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

func TestResolvePermApplyTarget_BareTokenWithAppsType(t *testing.T) {
	t.Parallel()

	token, docType, err := resolvePermApplyTarget("appBareToken", "apps")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "appBareToken" || docType != "apps" {
		t.Fatalf("got token=%q type=%q, want appBareToken/apps", token, docType)
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
		{"apps page", "https://example.feishu.cn/page/appMetaTok/?from=share", "appMetaTok", "apps"},
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

func TestResolvePermApplyTarget_RejectsMalformedPageURL(t *testing.T) {
	t.Parallel()

	token, docType, err := resolvePermApplyTarget("https://example.feishu.cn/page/?from=share", "")
	if err == nil || !strings.Contains(err.Error(), "could not infer token") {
		t.Fatalf("expected page token inference error, got token=%q type=%q error=%v", token, docType, err)
	}
}

func TestResolvePermApplyTarget_RejectsAppsMarkerOutsidePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
	}{
		{
			name: "query",
			raw:  "https://example.feishu.cn/share?redirect=/page/appMetaTok",
		},
		{
			name: "fragment",
			raw:  "https://example.feishu.cn/share#/page/appMetaTok",
		},
	}
	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			token, docType, err := resolvePermApplyTarget(tt.raw, "")
			if err == nil {
				t.Fatalf("expected URL path inference error, got token=%q type=%q", token, docType)
			}
			problem, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf(error) ok = false, error = %T %v", err, err)
			}
			if problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("error category/subtype = %q/%q, want %q/%q",
					problem.Category, problem.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != "--token" {
				t.Fatalf("error param = %q, want %q", validationErr.Param, "--token")
			}
		})
	}
}

func TestResolvePermApplyTarget_RejectsConflictingURLType(t *testing.T) {
	t.Parallel()
	_, _, err := resolvePermApplyTarget("https://example.feishu.cn/docx/doxTok123", "wiki")
	if err == nil || !strings.Contains(err.Error(), "conflicts with URL path type") {
		t.Fatalf("expected URL type conflict error, got: %v", err)
	}
}

func TestResolvePermApplyTarget_RejectsUnsafeOrAmbiguousTargets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		raw   string
		type_ string
	}{
		{"bare traversal token", "..", "docx"},
		{"bare dot token", ".", "docx"},
		{"URL traversal token", "https://example.feishu.cn/docx/../victim", ""},
		{"marker outside resource root", "https://example.feishu.cn/share/docx/doxUnexpected", ""},
		{"encoded path separator", "https://example.feishu.cn/docx/doxTarget%2Fother", ""},
		{"encoded query separator", "https://example.feishu.cn/docx/doxTarget%3Fother", ""},
	}

	for _, temp := range tests {
		tt := temp
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := resolvePermApplyTarget(tt.raw, tt.type_)
			if err == nil {
				t.Fatalf("resolvePermApplyTarget(%q, %q) unexpectedly succeeded", tt.raw, tt.type_)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("error = %T, want *errs.ValidationError", err)
			}
			if validationErr.Param != "--token" {
				t.Fatalf("error param = %q, want --token", validationErr.Param)
			}
		})
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

func TestDriveApplyPermission_DryRunAcceptsAppsBareToken(t *testing.T) {
	t.Parallel()

	f, stdout, _, _ := cmdutil.TestFactory(t, driveTestConfig())
	err := mountAndRunDrive(t, DriveApplyPermission, []string{
		"+apply-permission",
		"--token", "appBareToken",
		"--type", "apps",
		"--perm", "edit",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"/open-apis/drive/v1/permissions/appBareToken/members/apply",
		`"apps"`,
		`"edit"`,
		`"appBareToken"`,
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
			"error": map[string]interface{}{
				"details": []interface{}{
					map[string]interface{}{"value": "server says requests are disabled"},
				},
			},
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
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf(error) ok = false, error = %T %v", err, err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeInvalidParameters || problem.Code != 1063007 {
		t.Fatalf("problem = %+v, want api/invalid_parameters code 1063007", problem)
	}
	for _, want := range []string{"server says requests are disabled", "does not accept a permission-apply request", "contact the owner"} {
		if !strings.Contains(problem.Hint, want) {
			t.Fatalf("hint missing %q: %q", want, problem.Hint)
		}
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
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("ProblemOf(error) ok = false, error = %T %v", err, err)
	}
	if problem.Category != errs.CategoryAPI || problem.Subtype != errs.SubtypeRateLimit || problem.Code != 1063006 {
		t.Fatalf("problem = %+v, want api/rate_limit code 1063006", problem)
	}
	if problem.Retryable {
		t.Fatalf("problem.Retryable = true, want false for the daily per-document quota")
	}
	if !strings.Contains(problem.Hint, "at most 5 times per day") {
		t.Fatalf("hint missing daily quota guidance: %q", problem.Hint)
	}
}
