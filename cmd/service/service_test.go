// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/meta"
	"github.com/spf13/cobra"
)

// ── helpers ──

var testConfig = &core.CliConfig{
	AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
}

func driveSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "drive",
		"servicePath": "/open-apis/drive/v1",
	})
}

func driveMethod(httpMethod string, params map[string]interface{}) meta.Method {
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
	return meta.FromMap(m)
}

// ── registerService ──

func TestRegisterService(t *testing.T) {
	parent := &cobra.Command{Use: "root"}
	f := &cmdutil.Factory{}
	base := meta.ServiceFromMap(map[string]interface{}{
		"name":        "base",
		"description": "Base API",
		"servicePath": "/open-apis/base/v3",
		"resources": map[string]interface{}{
			"tables": map[string]interface{}{
				"methods": map[string]interface{}{
					"list": map[string]interface{}{
						"description": "List tables",
						"httpMethod":  "GET",
					},
				},
			},
		},
	})

	registerService(parent, base, f)

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
	svc := meta.ServiceFromMap(map[string]interface{}{
		"name": "base", "description": "Base API", "servicePath": "/open-apis/base/v3",
		"resources": map[string]interface{}{
			"tables": map[string]interface{}{
				"methods": map[string]interface{}{
					"list": map[string]interface{}{"description": "List", "httpMethod": "GET"},
				},
			},
		},
	})

	registerService(parent, svc, f)

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
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "GET"}), "list", "files", nil)

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
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "POST"}), "create", "files", nil)

	if cmd.Flags().Lookup("data") == nil {
		t.Error("POST method should have --data flag")
	}
}

func TestNewCmdServiceMethod_RunFCallback(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "GET"}), "list", "files",
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

func TestServiceMethod_DryRun_MailRuleReorderWarnsAboutExecutionExpansion(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["C","A"]}`,
		"--dry-run",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), `"rule_ids"`) || !strings.Contains(stdout.String(), `"C"`) {
		t.Fatalf("expected dry-run request body to remain unchanged, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "expands rule_ids") {
		t.Fatalf("expected mail rule reorder dry-run warning, got: %s", stderr.String())
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{
		"path": "items", "httpMethod": "GET",
		"parameters": map[string]interface{}{
			"q": map[string]interface{}{"location": "query", "required": true},
		},
	})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{
		"path": "items", "httpMethod": "GET",
		"parameters": map[string]interface{}{
			"page_size": map[string]interface{}{"location": "query", "required": true},
		},
	})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET"})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "POST", "parameters": map[string]interface{}{}})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "POST", "parameters": map[string]interface{}{}})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET"})
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	if got["ok"] != true || got["identity"] != "bot" {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, hasCode := got["code"]; hasCode {
		t.Fatalf("success envelope leaked outer code: %s", stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if !ok || data["result"] != "success" {
		t.Fatalf("data = %#v, want result=success", got["data"])
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	data, ok := got["data"].(map[string]interface{})
	if got["ok"] != true || got["identity"] != "bot" || !ok {
		t.Fatalf("unexpected envelope: %#v", got)
	}
	if _, hasCode := got["code"]; hasCode {
		t.Fatalf("success envelope leaked outer code: %s", stdout.String())
	}
	items, ok := data["items"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("data.items = %#v, want one item", data["items"])
	}
}

type serviceContentSafetyProvider struct {
	called bool
	path   string
	data   interface{}
	match  string
}

func (p *serviceContentSafetyProvider) Name() string { return "service-test" }

func (p *serviceContentSafetyProvider) Scan(_ context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	p.called = true
	p.path = req.Path
	p.data = req.Data
	if p.match != "" {
		b, _ := json.Marshal(req.Data)
		if !strings.Contains(string(b), p.match) {
			return nil, nil
		}
	}
	return &extcs.Alert{Provider: "service-test", MatchedRules: []string{"pagination"}}, nil
}

func TestServiceMethod_PageAll_DefaultJSONRunsContentSafety(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	provider := &serviceContentSafetyProvider{}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-safety", AppSecret: "test-secret-service-safety", Brand: core.BrandFeishu,
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(NewCmdServiceMethod(f, spec, method, "list", "items", nil))
	root.SetArgs([]string{"list", "--as", "bot", "--page-all"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.called {
		t.Fatal("expected content safety provider to scan paginated output")
	}
	if provider.path != "list" {
		t.Fatalf("scan path = %q, want list", provider.path)
	}
	data, ok := provider.data.(map[string]interface{})
	if !ok {
		t.Fatalf("scanned data type = %T, want map", provider.data)
	}
	if _, hasCode := data["code"]; hasCode {
		t.Fatalf("scanned data should be business data only, got %#v", data)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout.String())
	}
	alert, ok := got["_content_safety_alert"].(map[string]interface{})
	if !ok || alert["provider"] != "service-test" {
		t.Fatalf("missing content safety alert in envelope: %#v", got)
	}
}

func TestServiceMethod_PageAll_StreamFormatRunsContentSafety(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "warn")
	provider := &serviceContentSafetyProvider{}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, stderr, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-stream-safety", AppSecret: "test-secret-service-stream-safety", Brand: core.BrandFeishu,
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(NewCmdServiceMethod(f, spec, method, "list", "items", nil))
	root.SetArgs([]string{"list", "--as", "bot", "--page-all", "--format", "ndjson"})

	if err := root.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.called {
		t.Fatal("expected content safety provider to scan streamed paginated output")
	}
	if provider.path != "list" {
		t.Fatalf("scan path = %q, want list", provider.path)
	}
	items, ok := provider.data.([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("scanned data = %#v, want one streamed item", provider.data)
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert from service-test") {
		t.Fatalf("expected content safety warning on stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"id":"1"`) {
		t.Fatalf("expected streamed ndjson output, got: %s", stdout.String())
	}
}

func TestServiceMethod_PageAll_StreamFormatBlockSkipsBlockedPage(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	provider := &serviceContentSafetyProvider{match: "blocked"}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-stream-block", AppSecret: "test-secret-service-stream-block", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "safe-page"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"id": "blocked-page"}},
				"has_more": false,
			},
		},
	})

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(NewCmdServiceMethod(f, spec, method, "list", "items", nil))
	root.SetArgs([]string{"list", "--as", "bot", "--page-all", "--format", "ndjson"})

	err := root.Execute()
	if err == nil {
		t.Fatal("expected content safety block error")
	}
	var safetyErr *errs.ContentSafetyError
	if !errors.As(err, &safetyErr) {
		t.Fatalf("expected ContentSafetyError, got %T: %v", err, err)
	}
	if safetyErr.Category != errs.CategoryPolicy || safetyErr.Subtype != errs.SubtypeContentSafety {
		t.Fatalf("problem = %s/%s, want %s/%s", safetyErr.Category, safetyErr.Subtype, errs.CategoryPolicy, errs.SubtypeContentSafety)
	}
	if len(safetyErr.Rules) != 1 || safetyErr.Rules[0] != "pagination" {
		t.Fatalf("rules = %v, want [pagination]", safetyErr.Rules)
	}
	out := stdout.String()
	if !strings.Contains(out, "safe-page") {
		t.Fatalf("expected earlier safe page to remain streamed, got: %s", out)
	}
	if strings.Contains(out, "blocked-page") {
		t.Fatalf("blocked page was written before safety block: %s", out)
	}
}

func TestServiceMethod_BusinessErrorReturnsTypedErrorWithoutSuccessEnvelope(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-err", AppSecret: "test-secret-service-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 230027, "msg": "user not authorized",
		},
	})

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected PermissionError, got %T: %v", err, err)
	}
	if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("unexpected success envelope on error path: %s", stdout.String())
	}
}

func TestServiceMethod_PageAll_DefaultBusinessErrorOutputsRawResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-pageall-err", AppSecret: "test-secret-service-pageall-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 230027, "msg": "user not authorized",
		},
	})

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	if !strings.Contains(stdout.String(), "230027") || !strings.Contains(stdout.String(), "user not authorized") {
		t.Fatalf("expected raw error response on stdout, got: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("unexpected success envelope on error path: %s", stdout.String())
	}
}

func TestServiceMethod_PageAll_StreamBusinessErrorDoesNotDumpJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-service-pageall-stream-err", AppSecret: "test-secret-service-pageall-stream-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"id": "safe-page"}},
				"has_more":   true,
				"page_token": "next",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 230027,
			"msg":  "user not authorized",
		},
	})

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all", "--format", "ndjson"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	out := stdout.String()
	if !strings.Contains(out, "safe-page") {
		t.Fatalf("expected earlier successful page to remain streamed, got: %s", out)
	}
	if strings.Contains(out, "230027") || strings.Contains(out, "user not authorized") {
		t.Fatalf("streaming stdout should not contain raw error JSON, got: %s", out)
	}
	if strings.Contains(out, "\n  \"code\"") {
		t.Fatalf("streaming stdout should not contain indented JSON error dump, got: %s", out)
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
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
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "GET"}), "list", "files",
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
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "GET"}), "list", "files",
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET"})
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET"})
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
	spec := meta.ServiceFromMap(map[string]interface{}{
		"name": "svc", "servicePath": "/open-apis/svc/v1",
	})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET"})
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

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
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

func TestServiceMethod_PageAll_WithJqBusinessErrorOutputsRawResponse(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-spjq-err", AppSecret: "test-secret-spjq-err", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		Body: map[string]interface{}{
			"code": 230027, "msg": "user not authorized",
		},
	})

	spec := meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]interface{}{"path": "items", "httpMethod": "GET", "parameters": map[string]interface{}{}})
	cmd := NewCmdServiceMethod(f, spec, method, "list", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--page-all", "--jq", ".data.items[].id"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-zero code")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
	var permErr *errs.PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("expected PermissionError, got %T: %v", err, err)
	}
	if !strings.Contains(stdout.String(), "230027") || !strings.Contains(stdout.String(), "user not authorized") {
		t.Fatalf("expected raw error response on stdout, got: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"ok": true`) || strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("unexpected success envelope on error path: %s", stdout.String())
	}
}

func requireProblem(t *testing.T, err error, category errs.Category, subtype errs.Subtype, code int) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype || p.Code != code {
		t.Fatalf("problem = %s/%s/%d, want %s/%s/%d", p.Category, p.Subtype, p.Code, category, subtype, code)
	}
}

func TestCompleteMailRuleIDs(t *testing.T) {
	tests := []struct {
		name        string
		requested   []string
		current     []string
		want        []string
		wantErr     string
		wantSubtype errs.Subtype
		wantParam   string
	}{
		{
			name:      "partial input is prefix and missing rules append in current order",
			requested: []string{"C", "A"},
			current:   []string{"A", "B", "C", "D"},
			want:      []string{"C", "A", "B", "D"},
		},
		{
			name:      "complete input preserves user order",
			requested: []string{"C", "B", "A"},
			current:   []string{"A", "B", "C"},
			want:      []string{"C", "B", "A"},
		},
		{
			name:        "duplicate requested id",
			requested:   []string{"A", "A"},
			current:     []string{"A", "B"},
			wantErr:     "duplicate rule_id",
			wantSubtype: errs.SubtypeInvalidArgument,
			wantParam:   "rule_ids",
		},
		{
			name:        "unknown requested id",
			requested:   []string{"A", "X"},
			current:     []string{"A", "B"},
			wantErr:     "not found",
			wantSubtype: errs.SubtypeInvalidArgument,
			wantParam:   "rule_ids",
		},
		{
			name:        "empty current list rejects requested ids",
			requested:   []string{"A"},
			current:     nil,
			wantErr:     "current mailbox rule list is empty",
			wantSubtype: errs.SubtypeFailedPrecondition,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := completeMailRuleIDs(tt.requested, tt.current)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("completeMailRuleIDs() error = %v, want substring %q", err, tt.wantErr)
				}
				requireProblem(t, err, errs.CategoryValidation, tt.wantSubtype, 0)
				var ve *errs.ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("expected ValidationError, got %T: %v", err, err)
				}
				if ve.Param != tt.wantParam {
					t.Fatalf("ValidationError.Param = %q, want %q", ve.Param, tt.wantParam)
				}
				return
			}
			if err != nil {
				t.Fatalf("completeMailRuleIDs() error = %v", err)
			}
			if !stringSlicesEqual(got, tt.want) {
				t.Fatalf("completeMailRuleIDs() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestServiceMethod_MailRuleReorderCompletesIDs(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules", AppSecret: "test-secret-mail-rules", Brand: core.BrandFeishu,
	})

	reg.Register(mailRuleListStub("", []string{"A", "B", "C", "D"}, false, ""))
	reorderStub := mailRuleReorderStub(t, []string{"C", "A", "B", "D"}, map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{"ok": true},
	})
	reg.Register(reorderStub)

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["C","A"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
	if !strings.Contains(stdout.String(), `"ok": true`) && !strings.Contains(stdout.String(), `"ok":true`) {
		t.Fatalf("expected success envelope, got: %s", stdout.String())
	}
}

func TestServiceMethod_MailRuleReorderListKeepsRequestParams(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-params", AppSecret: "test-secret-mail-rules-params", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"rule_id": "A"}, map[string]interface{}{"rule_id": "B"}},
				"has_more": false,
			},
		},
		OnMatch: func(req *http.Request) {
			q := req.URL.Query()
			if q.Get("locale") != "en-US" {
				panic("expected original locale query param, got: " + req.URL.RawQuery)
			}
			if q.Get("page_size") != "" || q.Get("page_token") != "" {
				panic("unexpected pagination query params: " + req.URL.RawQuery)
			}
		},
	})
	reg.Register(mailRuleReorderStub(t, []string{"B", "A"}, map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{"ok": true},
	}))

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me","locale":"en-US"}`,
		"--data", `{"rule_ids":["B"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
}

func TestServiceMethod_MailRuleReorderCompletesIDsAcrossPages(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-pages", AppSecret: "test-secret-mail-rules-pages", Brand: core.BrandFeishu,
	})

	reg.Register(mailRuleListStub("", []string{"A", "B"}, true, "next-1"))
	reg.Register(mailRuleListStub("page_token=next-1", []string{"C", "D"}, false, ""))
	reg.Register(mailRuleReorderStub(t, []string{"D", "A", "B", "C"}, map[string]interface{}{
		"code": 0,
		"msg":  "ok",
		"data": map[string]interface{}{"ok": true},
	}))

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["D","A"]}`,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reg.Verify(t)
}

func TestServiceMethod_MailRuleReorderRejectsRepeatedPageToken(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-repeat-token", AppSecret: "test-secret-mail-rules-repeat-token", Brand: core.BrandFeishu,
	})

	reg.Register(mailRuleListStub("", []string{"A"}, true, "next-1"))
	reg.Register(mailRuleListStub("page_token=next-1", []string{"B"}, true, "next-1"))

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "repeated page_token") {
		t.Fatalf("expected repeated page_token error, got: %v", err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	reg.Verify(t)
}

func TestServiceMethod_MailRuleReorderRejectsMalformedHasMore(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-bad-has-more", AppSecret: "test-secret-mail-rules-bad-has-more", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"rule_id": "A"}},
				"has_more": "false",
			},
		},
	})

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "has_more must be a boolean") {
		t.Fatalf("expected malformed has_more error, got: %v", err)
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	reg.Verify(t)
}

func TestServiceMethod_MailRuleReorderListFailureDoesNotCallReorder(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-list-fail", AppSecret: "test-secret-mail-rules-list-fail", Brand: core.BrandFeishu,
	})

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body:   map[string]interface{}{"code": 99991400, "msg": "rate limited"},
	})

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected list API error")
	}
	requireProblem(t, err, errs.CategoryAPI, errs.SubtypeRateLimit, 99991400)
}

func TestServiceMethod_MailRuleReorderFailurePreservesAPIError(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-reorder-fail", AppSecret: "test-secret-mail-rules-reorder-fail", Brand: core.BrandFeishu,
	})

	reg.Register(mailRuleListStub("", []string{"A", "B"}, false, ""))
	reg.Register(mailRuleReorderStub(t, []string{"B", "A"}, map[string]interface{}{
		"code": 230027,
		"msg":  "user not authorized",
	}))

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["B"]}`,
	})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected reorder API error")
	}
	requireProblem(t, err, errs.CategoryAuthorization, errs.SubtypeUserUnauthorized, 230027)
}

func TestServiceMethod_MailRuleReorderValidationErrorsBeforeReorder(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "test-app-mail-rules-validation", AppSecret: "test-secret-mail-rules-validation", Brand: core.BrandFeishu,
	})

	reg.Register(mailRuleListStub("", []string{"A", "B"}, false, ""))

	cmd := NewCmdServiceMethod(f, mailRuleSpec(), mailRuleReorderMethod(), "reorder", "user_mailbox.rules", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"user_mailbox_id":"me"}`,
		"--data", `{"rule_ids":["A","X"]}`,
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `rule_id "X" was not found`) {
		t.Fatalf("expected unknown rule validation error, got: %v", err)
	}
	requireProblem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument, 0)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if ve.Param != "rule_ids" {
		t.Fatalf("ValidationError.Param = %q, want rule_ids", ve.Param)
	}
}

func mailRuleSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "mail",
		"servicePath": "/open-apis/mail/v1",
	})
}

func mailRuleReorderMethod() meta.Method {
	return meta.FromMap(map[string]interface{}{
		"id":         "ReorderUserMailboxRule",
		"path":       "user_mailboxes/{user_mailbox_id}/rules/reorder",
		"httpMethod": "POST",
		"risk":       "write",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
		},
		"requestBody": map[string]interface{}{
			"rule_ids": map[string]interface{}{
				"type": "array", "required": true,
			},
		},
		"accessTokens": []interface{}{"tenant"},
	})
}

func mailRuleListStub(queryContains string, ids []string, hasMore bool, nextToken string) *httpmock.Stub {
	items := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		items = append(items, map[string]interface{}{"rule_id": id})
	}
	data := map[string]interface{}{
		"items":    items,
		"has_more": hasMore,
	}
	if nextToken != "" {
		data["page_token"] = nextToken
	}
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		},
		BodyFilter: nil,
		OnMatch: func(req *http.Request) {
			if queryContains != "" && !strings.Contains(req.URL.RawQuery, queryContains) {
				panic("unexpected mail rule list query: " + req.URL.RawQuery)
			}
			if queryContains == "" && req.URL.Query().Get("page_token") != "" {
				panic("unexpected first mail rule list page token: " + req.URL.RawQuery)
			}
		},
	}
}

func mailRuleReorderStub(t *testing.T, wantIDs []string, body map[string]interface{}) *httpmock.Stub {
	t.Helper()
	return &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/mail/v1/user_mailboxes/me/rules/reorder",
		Body:   body,
		BodyFilter: func(raw []byte) bool {
			var got map[string]interface{}
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("reorder body is not JSON: %v\n%s", err, string(raw))
			}
			ids, ok := stringSlice(got["rule_ids"])
			if !ok {
				t.Fatalf("reorder rule_ids has wrong shape: %#v", got["rule_ids"])
			}
			return stringSlicesEqual(ids, wantIDs)
		},
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ── file upload ──

func imImageMethod() meta.Method {
	return meta.FromMap(map[string]interface{}{
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
	})
}

func imSpec() meta.Service {
	return meta.ServiceFromMap(map[string]interface{}{
		"name":        "im",
		"servicePath": "/open-apis/im/v1",
	})
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
	cmd := NewCmdServiceMethod(f, imSpec(), meta.FromMap(getMethod), "get", "images", nil)
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
			got := detectFileFields(meta.FromMap(tt.method))
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

// parseMultipartFilenames drives one service-method --file upload through the
// mock transport and returns a map of field name -> part filename parsed from
// the captured multipart body. Mirrors cmd/api's helper of the same name
// (inlined here rather than shared, since the two live in different packages)
// to give BuildFormdata's shared local-file fix a second real entry-point
// covering it.
func parseMultipartFilenames(t *testing.T, stub *httpmock.Stub) map[string]string {
	t.Helper()
	ct := stub.CapturedHeaders.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(ct)
	if err != nil {
		t.Fatalf("parse Content-Type %q: %v", ct, err)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("Content-Type = %q, want multipart/*", mediaType)
	}
	filenames := map[string]string{}
	mr := multipart.NewReader(bytes.NewReader(stub.CapturedBody), params["boundary"])
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}
		if fn := part.FileName(); fn != "" {
			filenames[part.FormName()] = fn
		}
	}
	return filenames
}

func TestServiceMethod_FileUpload_PreservesFilename(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)

	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("fake-image"), 0600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stub := &httpmock.Stub{
		URL:  "/open-apis/im/v1/images",
		Body: map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"image_key": "img_xxx"}},
	}
	reg.Register(stub)

	cmd := NewCmdServiceMethod(f, imSpec(), imImageMethod(), "create", "images", nil)
	cmd.SetArgs([]string{"--file", "photo.jpg", "--data", `{"image_type":"message"}`, "--as", "bot"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	filenames := parseMultipartFilenames(t, stub)
	if got := filenames["image"]; got != "photo.jpg" {
		t.Fatalf("part filename for field %q = %q, want %q", "image", got, "photo.jpg")
	}
}

func TestServiceMethod_JsonFlag_Accepted(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)

	var captured *ServiceMethodOptions
	cmd := NewCmdServiceMethod(f, driveSpec(),
		meta.FromMap(map[string]interface{}{"description": "desc", "httpMethod": "GET"}), "list", "files",
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
