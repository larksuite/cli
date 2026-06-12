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

func TestNewCmdServiceMethod_StrictModeHidesAsFlag(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, SupportedIdentities: 2,
	})

	cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("GET", nil), "copy", "files", nil)
	flag := cmd.Flags().Lookup("as")
	if flag == nil {
		t.Fatal("expected --as flag to be registered")
	}
	if !flag.Hidden {
		t.Fatal("expected --as flag to be hidden in strict mode")
	}
	if got := flag.DefValue; got != "bot" {
		t.Fatalf("default value = %q, want %q", got, "bot")
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

// ── path parameter auto-flags ──

func calendarEventDeleteSpec() map[string]interface{} {
	return map[string]interface{}{
		"name":        "calendar",
		"servicePath": "/open-apis/calendar/v4",
	}
}

func calendarEventDeleteMethod() map[string]interface{} {
	return map[string]interface{}{
		"path":       "calendars/{calendar_id}/events/{event_id}",
		"httpMethod": "DELETE",
		"parameters": map[string]interface{}{
			"calendar_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
				"description": "calendar id",
			},
			"event_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
				"description": "event id",
			},
			"need_notification": map[string]interface{}{
				"type": "boolean", "location": "query",
			},
		},
	}
}

func TestServiceMethod_PathParamFlagsRegistered(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, calendarEventDeleteSpec(), calendarEventDeleteMethod(), "delete", "events", nil)

	for _, name := range []string{"calendar-id", "event-id"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s flag to be auto-registered for path parameter", name)
		}
	}
	// Query params must NOT be auto-registered as flags — they continue to flow through --params.
	if cmd.Flags().Lookup("need-notification") != nil {
		t.Error("--need-notification must not be auto-registered: it is a query parameter, not a path parameter")
	}
}

func TestServiceMethod_PathParamFlagsAcceptedInDryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, calendarEventDeleteSpec(), calendarEventDeleteMethod(), "delete", "events", nil)
	cmd.SetArgs([]string{
		"--calendar-id", "cal_abc",
		"--event-id", "evt_xyz_0",
		"--params", `{"need_notification":false}`,
		"--dry-run",
		"--as", "bot",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected --calendar-id / --event-id to be accepted, got: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "/open-apis/calendar/v4/calendars/cal_abc/events/evt_xyz_0") {
		t.Errorf("expected URL with substituted path params, got:\n%s", out)
	}
	if !strings.Contains(out, "need_notification") {
		t.Errorf("expected query param need_notification preserved, got:\n%s", out)
	}
}

func TestServiceMethod_PathParamFlags_ParamsJSONStillSupported(t *testing.T) {
	// Backward compatibility: callers who already worked around the bug by
	// passing path params through --params JSON must keep working unchanged.
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, calendarEventDeleteSpec(), calendarEventDeleteMethod(), "delete", "events", nil)
	cmd.SetArgs([]string{
		"--params", `{"calendar_id":"cal_abc","event_id":"evt_xyz_0","need_notification":false}`,
		"--dry-run",
		"--as", "bot",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "/open-apis/calendar/v4/calendars/cal_abc/events/evt_xyz_0") {
		t.Errorf("expected --params JSON path values to drive URL, got:\n%s", stdout.String())
	}
}

func TestServiceMethod_PathParamFlags_ParamsJSONTakesPrecedence(t *testing.T) {
	// If a value is provided in BOTH --calendar-id and --params, the JSON
	// value wins. This keeps `--params` as the canonical "I know exactly
	// what I'm doing" escape hatch.
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, calendarEventDeleteSpec(), calendarEventDeleteMethod(), "delete", "events", nil)
	cmd.SetArgs([]string{
		"--calendar-id", "cal_FROM_FLAG",
		"--event-id", "evt_FROM_FLAG",
		"--params", `{"calendar_id":"cal_FROM_JSON","event_id":"evt_FROM_JSON"}`,
		"--dry-run",
		"--as", "bot",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "cal_FROM_JSON") || !strings.Contains(out, "evt_FROM_JSON") {
		t.Errorf("expected --params JSON to win over flags, got:\n%s", out)
	}
	if strings.Contains(out, "cal_FROM_FLAG") || strings.Contains(out, "evt_FROM_FLAG") {
		t.Errorf("expected flag values to be overridden by --params JSON, got:\n%s", out)
	}
}

func TestServiceMethod_PathParamFlags_RejectInvalid(t *testing.T) {
	// Path-traversal rejection still applies regardless of which input
	// channel (flag vs --params) supplied the value.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, calendarEventDeleteSpec(), calendarEventDeleteMethod(), "delete", "events", nil)
	cmd.SetArgs([]string{
		"--calendar-id", "../admin",
		"--event-id", "evt_ok",
		"--dry-run",
		"--as", "bot",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected validation error for path traversal in --calendar-id")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected 'path traversal' error, got: %v", err)
	}
}

func TestServiceMethod_PathParamFlags_NotRegisteredWhenAbsent(t *testing.T) {
	// Methods without path parameters must not gain spurious auto-flags.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{
		"path": "items", "httpMethod": "GET",
		"parameters": map[string]interface{}{
			"q": map[string]interface{}{"location": "query"},
		},
	}
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	if cmd.Flags().Lookup("q") != nil {
		t.Error("query-only methods must not produce path-param flags")
	}
}

func TestServiceMethod_PathParamFlags_DoNotShadowReservedFlags(t *testing.T) {
	// A path parameter named "format" or "data" must not clobber the
	// command's built-in flags. The collision-detection branch in the
	// registration loop should silently skip the auto-flag and keep the
	// reserved flag's behavior intact.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	spec := map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}
	method := map[string]interface{}{
		"path":       "items/{format}",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"format": map[string]interface{}{"location": "path", "required": true},
		},
	}
	cmd := NewCmdServiceMethod(f, spec, method, "get", "items", nil)
	formatFlag := cmd.Flags().Lookup("format")
	if formatFlag == nil {
		t.Fatal("built-in --format flag must remain registered")
	}
	if formatFlag.Usage == "URL path parameter format" {
		t.Error("auto-registration must not overwrite the built-in --format flag")
	}
}

func TestPathParamFlagName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"calendar_id", "calendar-id"},
		{"event_id", "event-id"},
		{"file_token", "file-token"},
		{"already-kebab", "already-kebab"}, // no-op
		{"single", "single"},
	}
	for _, tt := range tests {
		if got := pathParamFlagName(tt.in); got != tt.want {
			t.Errorf("pathParamFlagName(%q) = %q, want %q", tt.in, got, tt.want)
		}
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

func TestServiceMethod_JsonFlag_Accepted(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		map[string]interface{}{"description": "desc", "httpMethod": "GET"}, "list", "files",
		func(opts *ServiceMethodOptions) error {
			captured = opts
			return nil
		})
	cmd.SetArgs([]string{"--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("--json should be accepted without error, got: %v", err)
	}
	if captured == nil {
		t.Fatal("expected runF to be called")
	}
}
