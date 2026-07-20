// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
)

// apiErrWithScopes builds the typed error errclass.BuildAPIError produces for a
// Lark API failure carrying permission_violations. For authorization codes this
// yields an *errs.PermissionError with MissingScopes populated; for other codes
// it yields the corresponding typed error.
func apiErrWithScopes(code int, msg string, subjects ...string) error {
	resp := map[string]any{"code": code, "msg": msg}
	if len(subjects) > 0 {
		violations := make([]any, 0, len(subjects))
		for _, s := range subjects {
			violations = append(violations, map[string]any{"subject": s})
		}
		resp["error"] = map[string]any{"permission_violations": violations}
	}
	return errclass.BuildAPIError(resp, errclass.ClassifyContext{})
}

func TestPermissionGrantPermMessageUsesAPINameOnly(t *testing.T) {
	t.Parallel()

	if got := permissionGrantPermMessage(); got != "full_access" {
		t.Fatalf("permissionGrantPermMessage() = %q, want %q", got, "full_access")
	}
}

func TestAutoGrantStderrWarning_SkippedNoUser(t *testing.T) {
	config := &core.CliConfig{
		AppID:     "perm-grant-test-skip",
		AppSecret: "perm-grant-test-secret-skip",
		Brand:     core.BrandFeishu,
	}
	f, _, stderr, _ := cmdutil.TestFactory(t, config)

	ctx := cmdutil.ContextWithShortcut(context.Background(), "test:shortcut", "exec-1")
	runtime := &RuntimeContext{
		ctx:        ctx,
		Config:     config,
		Factory:    f,
		resolvedAs: core.AsBot,
	}

	result := AutoGrantCurrentUserDrivePermission(runtime, "tkn_doc", "docx")
	if result == nil {
		t.Fatal("expected non-nil result for bot mode with empty user open_id")
	}
	if result["status"] != PermissionGrantSkipped {
		t.Fatalf("status = %v, want %q", result["status"], PermissionGrantSkipped)
	}
	if !strings.Contains(stderr.String(), "auto-grant was skipped") {
		t.Fatalf("stderr missing auto-grant skipped warning; got:\n%s", stderr.String())
	}
	if hint, ok := result["hint"].(string); !ok || !strings.Contains(hint, "auth login") {
		t.Fatalf("hint = %#v, want string containing 'auth login'", result["hint"])
	}
	if hint, ok := result["hint"].(string); !ok || !strings.Contains(hint, "not logged in") {
		t.Fatalf("hint = %#v, want string containing 'not logged in'", result["hint"])
	}
}

func TestAutoGrantStderrWarning_GrantFailed(t *testing.T) {
	config := &core.CliConfig{
		AppID:      "perm-grant-test-fail",
		AppSecret:  "perm-grant-test-secret-fail",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test_user",
	}
	f, _, stderr, reg := cmdutil.TestFactory(t, config)

	// Register a stub that returns an error code so CallAPI returns an error.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/tkn_doc/members",
		Body: map[string]interface{}{
			"code": 230001,
			"msg":  "no permission",
		},
	})

	ctx := cmdutil.ContextWithShortcut(context.Background(), "test:shortcut", "exec-2")
	runtime := &RuntimeContext{
		ctx:        ctx,
		Config:     config,
		Factory:    f,
		resolvedAs: core.AsBot,
	}

	result := AutoGrantCurrentUserDrivePermission(runtime, "tkn_doc", "docx")
	if result == nil {
		t.Fatal("expected non-nil result for bot mode with grant failure")
	}
	if result["status"] != PermissionGrantFailed {
		t.Fatalf("status = %v, want %q", result["status"], PermissionGrantFailed)
	}
	if !strings.Contains(stderr.String(), "auto-grant failed") {
		t.Fatalf("stderr missing auto-grant failed warning; got:\n%s", stderr.String())
	}
	if hint, ok := result["hint"].(string); !ok || !strings.Contains(hint, "Retry later") {
		t.Fatalf("hint = %#v, want string containing 'Retry later'", result["hint"])
	}
	if hint, ok := result["hint"].(string); !ok || !strings.Contains(hint, "scope") {
		t.Fatalf("hint = %#v, want string containing 'scope'", result["hint"])
	}
	if hint, ok := result["hint"].(string); !ok || !strings.Contains(hint, "permission changes") {
		t.Fatalf("hint = %#v, want string containing 'permission changes'", result["hint"])
	}
}

// ── annotateGrantPermissionError unit tests ────────────────────────────────

func newAnnotateRuntime(brand core.LarkBrand, appID string) *RuntimeContext {
	return &RuntimeContext{
		Config: &core.CliConfig{
			AppID: appID,
			Brand: brand,
		},
	}
}

// permission_violations subjects must surface as required_scope, and the
// console_url must be brand-specific. The hint should be overridden to point
// at the developer console.
func TestAnnotateGrantPermissionError_AppScopeNotEnabled(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandFeishu, "cli_demo")
	result := map[string]interface{}{
		"hint": "generic fallback hint",
	}

	err := apiErrWithScopes(99991672, "Permission denied [99991672]", "docs:permission.member:create")

	annotateGrantPermissionError(rt, result, err)

	if got := result["lark_code"]; got != 99991672 {
		t.Errorf("expected lark_code=99991672, got %v", got)
	}
	if got, _ := result["required_scope"].(string); got != "docs:permission.member:create" {
		t.Errorf("required_scope mismatch: got %v", got)
	}
	consoleURL, _ := result["console_url"].(string)
	if !strings.HasPrefix(consoleURL, "https://open.feishu.cn/page/scope-apply") {
		t.Errorf("console_url should target open.feishu.cn, got %s", consoleURL)
	}
	if !strings.Contains(consoleURL, "clientID=cli_demo") {
		t.Errorf("console_url missing clientID, got %s", consoleURL)
	}
	hint, _ := result["hint"].(string)
	if !strings.Contains(hint, "console_url") {
		t.Errorf("hint should reference console_url, got %s", hint)
	}
	if !strings.Contains(hint, "docs:permission.member:create") {
		t.Errorf("hint should mention required scope, got %s", hint)
	}
}

func TestAnnotateGrantPermissionError_LarkBrand(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandLark, "cli_demo")
	result := map[string]interface{}{}
	err := apiErrWithScopes(99991679, "Permission denied [99991679]", "docs:permission.member:create")

	annotateGrantPermissionError(rt, result, err)

	if u, _ := result["console_url"].(string); !strings.Contains(u, "open.larksuite.com") {
		t.Errorf("lark brand should yield larksuite host, got %s", u)
	}
}

// Non-permission errors (network, validation, plain errors) must not be
// annotated — keep the existing generic hint untouched.
func TestAnnotateGrantPermissionError_NonPermissionErrorNoOp(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandFeishu, "cli_demo")

	cases := []error{
		errors.New("plain error"),
		apiErrWithScopes(230001, "no permission", "docs:doc"), // unknown code → *errs.APIError, not permission

	}
	for i, e := range cases {
		result := map[string]interface{}{
			"hint": "untouched hint",
		}
		annotateGrantPermissionError(rt, result, e)
		if _, ok := result["lark_code"]; ok {
			t.Errorf("case %d: expected no lark_code, got %v", i, result["lark_code"])
		}
		if _, ok := result["console_url"]; ok {
			t.Errorf("case %d: expected no console_url, got %v", i, result["console_url"])
		}
		if got, _ := result["hint"].(string); got != "untouched hint" {
			t.Errorf("case %d: hint should be unchanged, got %s", i, got)
		}
	}
}

// permission_violations missing → only lark_code is annotated; no console_url
// and the existing hint stays as-is (caller's generic fallback wins).
func TestAnnotateGrantPermissionError_NoViolations(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandFeishu, "cli_demo")
	result := map[string]interface{}{
		"hint": "untouched fallback",
	}
	err := apiErrWithScopes(99991672, "Permission denied [99991672]")

	annotateGrantPermissionError(rt, result, err)

	if got := result["lark_code"]; got != 99991672 {
		t.Errorf("expected lark_code captured, got %v", got)
	}
	if _, ok := result["console_url"]; ok {
		t.Errorf("console_url must not be set when violations are absent")
	}
	if got, _ := result["hint"].(string); got != "untouched fallback" {
		t.Errorf("hint should remain fallback when no console_url, got %s", got)
	}
}

// AppID empty → no console_url even when violations exist.
func TestAnnotateGrantPermissionError_EmptyAppID(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandFeishu, "")
	result := map[string]interface{}{}
	err := apiErrWithScopes(99991672, "Permission denied", "docs:doc")

	annotateGrantPermissionError(rt, result, err)
	if _, ok := result["console_url"]; ok {
		t.Errorf("console_url must not be set when appID is empty")
	}
	if got, _ := result["required_scope"].(string); got != "docs:doc" {
		t.Errorf("required_scope should still be set when appID is empty, got %s", got)
	}
}

// Defensive: nil/empty arguments must be safe no-ops.
func TestAnnotateGrantPermissionError_NilArgsSafe(t *testing.T) {
	rt := newAnnotateRuntime(core.BrandFeishu, "cli_demo")

	annotateGrantPermissionError(nil, map[string]interface{}{}, nil)
	annotateGrantPermissionError(rt, nil, nil)
	annotateGrantPermissionError(rt, map[string]interface{}{}, nil)
	annotateGrantPermissionError(rt, map[string]interface{}{}, errors.New(""))
}

// Integration-style: end-to-end through AutoGrantCurrentUserDrivePermission
// with a mocked 99991672 response — verifies the annotated fields show up
// in the JSON result that callers downstream consume.
func TestAutoGrantStderrWarning_GrantFailed_AppScopeNotEnabled_Annotated(t *testing.T) {
	config := &core.CliConfig{
		AppID:      "cli_app_demo",
		AppSecret:  "secret",
		Brand:      core.BrandFeishu,
		UserOpenId: "ou_test_user",
	}
	f, _, _, reg := cmdutil.TestFactory(t, config)

	// Stub the permission member create endpoint with a 99991672 response that
	// includes permission_violations — what the platform returns when the app
	// has not enabled the API scope.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/permissions/tkn_doc/members",
		Body: map[string]interface{}{
			"code": 99991672,
			"msg":  "App scope not enabled",
			"error": map[string]interface{}{
				"permission_violations": []interface{}{
					map[string]interface{}{"subject": "docs:permission.member:create"},
				},
			},
		},
	})

	ctx := cmdutil.ContextWithShortcut(context.Background(), "test:shortcut", "exec-3")
	runtime := &RuntimeContext{
		ctx:        ctx,
		Config:     config,
		Factory:    f,
		resolvedAs: core.AsBot,
	}

	result := AutoGrantCurrentUserDrivePermission(runtime, "tkn_doc", "docx")
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["status"] != PermissionGrantFailed {
		t.Fatalf("status = %v, want failed", result["status"])
	}
	if result["lark_code"] != 99991672 {
		t.Errorf("lark_code = %v, want 99991672", result["lark_code"])
	}
	if got, _ := result["required_scope"].(string); got != "docs:permission.member:create" {
		t.Errorf("required_scope = %v, want docs:permission.member:create", got)
	}
	consoleURL, _ := result["console_url"].(string)
	if !strings.Contains(consoleURL, "open.feishu.cn/page/scope-apply") {
		t.Errorf("console_url missing or wrong host: %s", consoleURL)
	}
	if !strings.Contains(consoleURL, "scopes=docs%3Apermission.member%3Acreate") {
		t.Errorf("console_url missing escaped scope: %s", consoleURL)
	}
	hint, _ := result["hint"].(string)
	if !strings.Contains(hint, "console_url") {
		t.Errorf("hint should be overridden to mention console_url, got %s", hint)
	}
}
