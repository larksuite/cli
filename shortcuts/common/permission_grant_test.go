// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

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
