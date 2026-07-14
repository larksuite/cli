// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestAutomationEnable_PostsEnabledStatus(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	rctx.Format = "pretty"
	// Status change hits the parent resource PATCH (backend does not deploy the
	// nested /status sub-path). Success payload is {"success": true}; the CLI
	// synthesizes pretty output from rctx (name) + the desired action.
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/t1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"success": true}},
	})
	if err := AppsAutomationEnable.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "trigger t1 status: enabled") {
		t.Errorf("enable output = %q", stdoutBuf.String())
	}
}

func TestAutomationDisable_PostsDisabledStatus(t *testing.T) {
	rctx, stdoutBuf, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	rctx.Format = "pretty"
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/t1",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"success": true}},
	})
	if err := AppsAutomationDisable.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if !strings.Contains(stdoutBuf.String(), "trigger t1 status: disabled") {
		t.Errorf("disable output = %q", stdoutBuf.String())
	}
}

func TestAutomationEnableDisableMeta(t *testing.T) {
	if AppsAutomationEnable.Risk != "write" || AppsAutomationDisable.Risk != "write" {
		t.Error("enable/disable must be Risk=write")
	}
	if AppsAutomationEnable.Command != "+automation-enable" || AppsAutomationDisable.Command != "+automation-disable" {
		t.Error("command names mismatch")
	}
}

// TestAutomationEnable_APIErrorAttachesNotFoundHint exercises the failure path
// of runAutomationStatus. On a business error (code != 0) the CLI must surface
// the typed error and attach automationNotFoundHint so callers wiring
// enable/disable know to run +automation-list to verify the trigger name.
func TestAutomationEnable_APIErrorAttachesNotFoundHint(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "missing"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/missing",
		Body: map[string]interface{}{"code": 400400001, "msg": "trigger not found"},
	})
	err := AppsAutomationEnable.Execute(context.Background(), rctx)
	if err == nil {
		t.Fatal("expected typed api error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	// Per AGENTS.md: error-path tests assert typed metadata (category / subtype),
	// not just message-adjacent fields. Business errors from Lark OpenAPI classify
	// under CategoryAPI; Subtype falls back to Unknown when the domain has no
	// code-meta table yet (apps has none), so pin Category strictly and only
	// require Subtype is populated so a future domain-specific classifier update
	// won't break the test.
	if p.Category != errs.CategoryAPI {
		t.Errorf("category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype == "" {
		t.Error("subtype must be populated on typed API errors")
	}
	if p.Code != 400400001 {
		t.Errorf("code = %d, want 400400001", p.Code)
	}
	if !strings.Contains(p.Hint, "+automation-list") {
		t.Errorf("hint must point at +automation-list, got %q", p.Hint)
	}
}

// TestAutomationDisable_APIErrorAttachesNotFoundHint mirrors the enable test
// against the disable Execute closure. Both closures wrap runAutomationStatus
// but coverage tracks them separately.
func TestAutomationDisable_APIErrorAttachesNotFoundHint(t *testing.T) {
	rctx, _, reg := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "missing"})
	reg.Register(&httpmock.Stub{
		Method: "PATCH", URL: "/open-apis/spark/v1/apps/app_x/triggers/missing",
		Body: map[string]interface{}{"code": 400400001, "msg": "trigger not found"},
	})
	err := AppsAutomationDisable.Execute(context.Background(), rctx)
	if err == nil {
		t.Fatal("expected typed api error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Category != errs.CategoryAPI {
		t.Errorf("category = %q, want %q", p.Category, errs.CategoryAPI)
	}
	if p.Subtype == "" {
		t.Error("subtype must be populated on typed API errors")
	}
	if p.Code != 400400001 {
		t.Errorf("code = %d, want 400400001", p.Code)
	}
	if !strings.Contains(p.Hint, "+automation-list") {
		t.Errorf("hint must point at +automation-list, got %q", p.Hint)
	}
}

// TestAutomationEnable_DryRunPreview exercises the DryRun closure so it appears
// in coverage and pins the request shape (PATCH + status body).
func TestAutomationEnable_DryRunPreview(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	preview := AppsAutomationEnable.DryRun(context.Background(), rctx)
	if preview == nil {
		t.Fatal("DryRun returned nil")
	}
	blob, err := preview.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	got := string(blob)
	if !strings.Contains(got, `"method":"PATCH"`) ||
		!strings.Contains(got, "/apps/app_x/triggers/t1") ||
		!strings.Contains(got, `"status":"enabled"`) {
		t.Errorf("preview missing expected PATCH/URL/body fields: %s", got)
	}
}

func TestAutomationDisable_DryRunPreview(t *testing.T) {
	rctx, _, _ := newOpenAPIKeyRCtx(t,
		map[string]string{"app-id": "string", "name": "string"},
		map[string]string{"app-id": "app_x", "name": "t1"})
	preview := AppsAutomationDisable.DryRun(context.Background(), rctx)
	if preview == nil {
		t.Fatal("DryRun returned nil")
	}
	blob, err := preview.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal preview: %v", err)
	}
	got := string(blob)
	if !strings.Contains(got, `"method":"PATCH"`) ||
		!strings.Contains(got, "/apps/app_x/triggers/t1") ||
		!strings.Contains(got, `"status":"disabled"`) {
		t.Errorf("preview missing expected PATCH/URL/body fields: %s", got)
	}
}
