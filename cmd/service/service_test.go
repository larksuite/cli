// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ── helpers ──

var testConfig = &core.CliConfig{
	AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
}

func driveSpec() map[string]interface{} {
	return map[string]interface{}{
		"name":        "drive",
		"servicePath": "/open-apis/drive/v1",
	}
}

func driveMethod(httpMethod string, params map[string]interface{}) map[string]interface{} {
	m := map[string]interface{}{
		"path":       "files/{file_token}/copy",
		"httpMethod": httpMethod,
	}
	if params != nil {
		m["parameters"] = params
	} else {
		m["parameters"] = map[string]interface{}{
			"file_token": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
		}
	}
	return m
}

// ── registerService ──

func TestRegisterService(t *testing.T) {
	parent := &cobra.Command{Use: "root"}
	f := &cmdutil.Factory{}
	spec := map[string]interface{}{
		"name":        "base",
		"description": "Base API",
		"servicePath": "/open-apis/base/v3",
	}
	resources := map[string]interface{}{
		"tables": map[string]interface{}{
			"methods": map[string]interface{}{
				"list": map[string]interface{}{
					"description": "List tables",
					"httpMethod":  "GET",
				},
			},
		},
	}

	registerService(parent, spec, resources, f)

	// service command exists
	svc, _, err := parent.Find([]string{"base"})
	if err != nil || svc.Name() != "base" {
		t.Fatalf("expected 'base' command, got err=%v", err)
	}
	// resource sub-command
	res, _, err := parent.Find([]string{"base", "tables"})
	if err != nil || res.Name() != "tables" {
		t.Fatalf("expected 'tables' command, got err=%v", err)
	}
	// method sub-command
	meth, _, err := parent.Find([]string{"base", "tables", "list"})
	if err != nil || meth.Name() != "list" {
		t.Fatalf("expected 'list' command, got err=%v", err)
	}
}

func TestRegisterService_MergesExistingCommand(t *testing.T) {
	parent := &cobra.Command{Use: "root"}
	existing := &cobra.Command{Use: "base", Short: "existing"}
	parent.AddCommand(existing)

	f := &cmdutil.Factory{}
	spec := map[string]interface{}{
		"name": "base", "description": "Base API", "servicePath": "/open-apis/base/v3",
	}
	resources := map[string]interface{}{
		"tables": map[string]interface{}{
			"methods": map[string]interface{}{
				"list": map[string]interface{}{"description": "List", "httpMethod": "GET"},
			},
		},
	}

	registerService(parent, spec, resources, f)

	// Should reuse existing, not duplicate
	count := 0
	for _, c := range parent.Commands() {
		if c.Name() == "base" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 'base' command, got %d", count)
	}
	// Resource should be added under the existing command
	_, _, err := parent.Find([]string{"base", "tables", "list"})
	if err != nil {
		t.Fatalf("expected 'list' under existing 'base' command, got err=%v", err)
	}
}

// ── NewCmdServiceMethod flags ──

func TestNewCmdServiceMethod_GETHasNoDataFlag(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "GET"}, "list", "files", nil)

	if cmd.Flags().Lookup("data") != nil {
		t.Error("GET method should not have --data flag")
	}
	if cmd.Use != "list" {
		t.Errorf("expected Use=list, got %s", cmd.Use)
	}
	if !strings.Contains(cmd.Long, "schema drive.files.list") {
		t.Errorf("expected schema path in Long, got %s", cmd.Long)
	}
}

func TestNewCmdServiceMethod_POSTHasDataFlag(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "POST"}, "create", "files", nil)

	if cmd.Flags().Lookup("data") == nil {
		t.Error("POST method should have --data flag")
	}
}

func TestNewCmdServiceMethod_RunFCallback(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "GET"}, "list", "files",
		func(opts *ServiceMethodOptions) error {
			captured = opts
			return nil
		})
	cmd.SetArgs([]string{"--as", "bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("runF was not called")
	}
	if captured.As != core.AsBot {
		t.Errorf("expected As=bot, got %s", captured.As)
	}
	if captured.SchemaPath != "drive.files.list" {
		t.Errorf("expected SchemaPath=drive.files.list, got %s", captured.SchemaPath)
	}
}

// ── dry-run / buildServiceRequest ──

func TestServiceMethod_DryRun_PathParam(t *testing.T) {
	tests := []struct {
		name      string
		fileToken string
		wantInURL string
	}{
		{"normal token", "boxcn123abc", "/open-apis/drive/v1/files/boxcn123abc/copy"},
		{"hyphen and underscore", "ou_abc-123_def", "/open-apis/drive/v1/files/ou_abc-123_def/copy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
			cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("POST", nil), "copy", "files", nil)
			cmd.SetArgs([]string{
				"--params", `{"file_token":"` + tt.fileToken + `"}`,
				"--data", `{"name":"test.txt"}`,
				"--dry-run",
			})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(stdout.String(), tt.wantInURL) {
				t.Errorf("expected URL containing %q, got:\n%s", tt.wantInURL, stdout.String())
			}
		})
	}
}

func TestServiceMethod_PathParamRejectsTraversal(t *testing.T) {
	tests := []struct {
		name      string
		fileToken string
		wantErr   string
	}{
		{"path traversal with slashes", "../../auth/v3/token", "path traversal"},
		{"single dot-dot", "../admin", "path traversal"},
		{"question mark injection", "token?evil=true", "invalid characters"},
		{"hash injection", "token#fragment", "invalid characters"},
		{"percent-encoded bypass", "token%2F..%2Fadmin", "invalid characters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _, _ := cmdutil.TestFactory(t, testConfig)
			cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("POST", nil), "copy", "files", nil)
			cmd.SetArgs([]string{
				"--params", `{"file_token":"` + tt.fileToken + `"}`,
				"--data", `{"name":"test.txt"}`,
				"--dry-run",
			})
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error for malicious path parameter")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestServiceMethod_MissingPathParam(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("POST", nil), "copy", "files", nil)
	cmd.SetArgs([]string{"--params", `{}`, "--data", `{}`, "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing path param")
	}
	if !strings.Contains(err.Error(), "missing required path parameter") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServiceMethod_MissingRequiredQueryParam(t *testing.T) {
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{
		"path": "items", "httpMethod": "GET",
		"parameters": map[string]interface{}{
			"q": map[string]interface{}{"location": "query", "required": true},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--params", `{}`, "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required query param")
	}
	if !strings.Contains(err.Error(), "missing required query parameter: q") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServiceMethod_PaginationParamSkippedWithPageAll(t *testing.T) {
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{
		"path": "items", "httpMethod": "GET",
		"parameters": map[string]interface{}{
			"page_size": map[string]interface{}{"location": "query", "required": true},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--params", `{}`, "--page-all", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error with --page-all skipping page_size, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Dry Run") {
		t.Error("expected dry-run output")
	}
}

func TestServiceMethod_InvalidParamsJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET"}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--params", "{bad", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "--params invalid format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServiceMethod_InvalidDataJSON(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "POST", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "create", "items", nil)
	cmd.SetArgs([]string{"--data", "{bad", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --data JSON")
	}
	if !strings.Contains(err.Error(), "--data invalid JSON format") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServiceMethod_ParamsAndDataBothStdinConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "POST", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "create", "items", nil)
	cmd.SetArgs([]string{"--params", "-", "--data", "-", "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when both --params and --data use stdin")
	}
	if !strings.Contains(err.Error(), "cannot both read from stdin") {
		t.Errorf("expected stdin conflict error, got: %v", err)
	}
}

func TestServiceMethod_OutputAndPageAllConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET"}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--page-all", "--output", "file.bin", "--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --output + --page-all conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── bot mode integration with httpmock ──

func TestServiceMethod_BotMode_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"result": "success"},
		},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "success") {
		t.Errorf("expected 'success' in output, got:\n%s", stdout.String())
	}
}

func TestServiceMethod_BotMode_APIError(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-err", AppSecret: "test-secret-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/svc/v1/items",
		Body: map[string]interface{}{"code": 40003, "msg": "invalid token"},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected API error")
	}
	var exitErr *output.ExitError
	if !isExitError(err, &exitErr) {
		t.Fatalf("expected ExitError, got: %T %v", err, err)
	}
	if exitErr.Code != output.ExitAPI {
		t.Errorf("expected ExitAPI code, got %d", exitErr.Code)
	}
	// stdout must be empty on API error — error details belong in stderr envelope only.
	// This guards against re-introducing duplicate output (see commit 86215a10).
	if stdout.Len() > 0 {
		t.Errorf("expected no stdout on API error, got: %s", stdout.String())
	}
}

func TestServiceMethod_BotMode_PageAll_JSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-page", AppSecret: "test-secret-page", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "1"}},
				"has_more": false,
			},
		},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"id"`) {
		t.Errorf("expected items in output, got:\n%s", stdout.String())
	}
}

func TestServiceMethod_UnknownFormat_Warning(t *testing.T) {
	f, _, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-fmt", AppSecret: "test-secret-fmt", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/svc/v1/items",
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--format", "unknown"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: unknown format") {
		t.Errorf("expected format warning in stderr, got:\n%s", stderr.String())
	}
}

// ── jq flag ──

func TestNewCmdServiceMethod_JqFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "GET"}, "list", "files",
		func(opts *ServiceMethodOptions) error {
			captured = opts
			return nil
		})
	cmd.SetArgs([]string{"--jq", ".data"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured == nil {
		t.Fatal("runF was not called")
	}
	if captured.JqExpr != ".data" {
		t.Errorf("expected JqExpr=.data, got %s", captured.JqExpr)
	}
}

func TestNewCmdServiceMethod_JqShortForm(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "GET"}, "list", "files",
		func(opts *ServiceMethodOptions) error {
			captured = opts
			return nil
		})
	cmd.SetArgs([]string{"-q", ".data"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.JqExpr != ".data" {
		t.Errorf("expected JqExpr=.data, got %s", captured.JqExpr)
	}
}

func TestServiceMethod_JqAndOutputConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET"}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--jq", ".data", "--output", "file.bin", "--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --jq + --output conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestServiceMethod_JqFilter_AppliesExpression(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-jq", AppSecret: "test-secret-jq", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			},
		},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--jq", ".data.items[].name"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("expected jq-filtered names, got: %s", out)
	}
	if strings.Contains(out, `"code"`) {
		t.Errorf("expected jq to filter out envelope, got: %s", out)
	}
}

func TestServiceMethod_JqAndFormatConflict(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET"}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--jq", ".data", "--format", "ndjson", "--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for --jq + --format ndjson conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestServiceMethod_JqInvalidExpression(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET"}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--jq", "invalid[", "--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid jq expression")
	}
	if !strings.Contains(err.Error(), "invalid jq expression") {
		t.Errorf("expected 'invalid jq expression' error, got: %v", err)
	}
}

func TestServiceMethod_PageAll_WithJq(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-spjq", AppSecret: "test-secret-spjq", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "s1"}, map[string]interface{}{"id": "s2"}},
				"has_more": false,
			},
		},
	})

	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all", "--jq", ".data.items[].id"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "s1") || !strings.Contains(out, "s2") {
		t.Errorf("expected jq-filtered ids, got: %s", out)
	}
	if strings.Contains(out, `"code"`) {
		t.Errorf("expected jq to filter out envelope, got: %s", out)
	}
}

// ── scopeAwareChecker ──

func TestScopeAwareChecker_Success(t *testing.T) {
	checker := scopeAwareChecker(nil, false)
	err := checker(map[string]interface{}{"code": 0.0, "msg": "ok"})
	if err != nil {
		t.Errorf("expected nil error for code=0, got: %v", err)
	}
}

func TestScopeAwareChecker_NonMapResult(t *testing.T) {
	checker := scopeAwareChecker(nil, false)
	err := checker("not a map")
	if err != nil {
		t.Errorf("expected nil for non-map result, got: %v", err)
	}
}

func TestScopeAwareChecker_APIError(t *testing.T) {
	checker := scopeAwareChecker(nil, false)
	err := checker(map[string]interface{}{"code": 40003.0, "msg": "bad request"})
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	if !strings.Contains(err.Error(), "API error: [40003]") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestScopeAwareChecker_ScopeError_UserMode(t *testing.T) {
	scopes := []interface{}{"calendar:read"}
	checker := scopeAwareChecker(scopes, false)
	err := checker(map[string]interface{}{
		"code": float64(output.LarkErrUserScopeInsufficient),
		"msg":  "scope insufficient",
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	var exitErr *output.ExitError
	if !isExitError(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T", err)
	}
	if exitErr.Detail.Type != "permission" {
		t.Errorf("expected type=permission, got %s", exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Hint, "auth login") {
		t.Errorf("expected auth login hint, got %s", exitErr.Detail.Hint)
	}
}

func TestScopeAwareChecker_ScopeError_BotMode(t *testing.T) {
	scopes := []interface{}{"calendar:read"}
	checker := scopeAwareChecker(scopes, true)
	err := checker(map[string]interface{}{
		"code": float64(output.LarkErrUserScopeInsufficient),
		"msg":  "scope insufficient",
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	// Bot mode should still include the scope hint
	if !strings.Contains(err.Error(), "insufficient permissions") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── file upload ──

func imImageMethod() map[string]interface{} {
	return map[string]interface{}{
		"path":       "images",
		"httpMethod": "POST",
		"requestBody": map[string]interface{}{
			"image_type": map[string]interface{}{
				"type":     "string",
				"required": true,
			},
			"image": map[string]interface{}{
				"type":     "file",
				"required": true,
			},
		},
		"accessTokens": []interface{}{"user", "tenant"},
	}
}

func imSpec() map[string]interface{} {
	return map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
	}
}

func TestServiceMethod_FileFlagRegistered(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imImageMethod(), "create", "images", nil)
	flag := cmd.Flags().Lookup("file")
	if flag == nil {
		t.Fatal("expected --file flag to be registered for file upload method")
	}
}

func TestServiceMethod_FileFlagNotRegistered(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("POST", nil), "copy", "files", nil)
	flag := cmd.Flags().Lookup("file")
	if flag != nil {
		t.Fatal("expected --file flag NOT to be registered for non-file method")
	}
}

func TestServiceMethod_FileFlagNotRegisteredForGET(t *testing.T) {
	getMethod := map[string]interface{}{
		"path":       "images",
		"httpMethod": "GET",
		"requestBody": map[string]interface{}{
			"image": map[string]interface{}{
				"type": "file",
			},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), getMethod, "get", "images", nil)
	flag := cmd.Flags().Lookup("file")
	if flag != nil {
		t.Fatal("expected --file flag NOT to be registered for GET method")
	}
}

func TestServiceMethod_FileUpload_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.jpg"
	if err := os.WriteFile(tmpFile, []byte("fake-image"), 0600); err != nil {
		t.Fatal(err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, imSpec(), imImageMethod(), "create", "images", nil)
	cmd.SetArgs([]string{
		"--file", "image=" + tmpFile,
		"--data", `{"image_type":"message"}`,
		"--dry-run",
		"--as", "bot",
	})
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "image") {
		t.Errorf("expected dry-run output to mention file field, got: %s", out)
	}
	if !strings.Contains(out, "Dry Run") {
		t.Errorf("expected dry-run header, got: %s", out)
	}
}

func TestDetectFileFields(t *testing.T) {
	tests := []struct {
		name   string
		method map[string]interface{}
		want   []string
	}{
		{
			name: "single file field",
			method: map[string]interface{}{
				"requestBody": map[string]interface{}{
					"image": map[string]interface{}{"type": "file"},
					"name":  map[string]interface{}{"type": "string"},
				},
			},
			want: []string{"image"},
		},
		{
			name: "no file fields",
			method: map[string]interface{}{
				"requestBody": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
				},
			},
			want: nil,
		},
		{
			name:   "no requestBody",
			method: map[string]interface{}{},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectFileFields(tt.method)
			if len(got) != len(tt.want) {
				t.Errorf("detectFileFields() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("detectFileFields()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ── helpers ──

func isExitError(err error, target **output.ExitError) bool {
	ee, ok := err.(*output.ExitError)
	if ok && target != nil {
		*target = ee
	}
	return ok
}
