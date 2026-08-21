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
	extcred "github.com/larksuite/cli/extension/credential"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/output"
	"github.com/spf13/cobra"
)

// ── helpers ──

var testConfig = &core.CliConfig{
	AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
}

type panicCredentialProvider struct{}

func (panicCredentialProvider) Name() string { return "panic-if-called" }

func (panicCredentialProvider) ResolveAccount(context.Context) (*extcred.Account, error) {
	panic("credential resolution must not run")
}

func (panicCredentialProvider) ResolveToken(context.Context, extcred.TokenSpec) (*extcred.Token, error) {
	panic("credential resolution must not run")
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

func TestNewPreflightMissingScopeErrorUsesCanonicalFieldGate(t *testing.T) {
	err := newPreflightMissingScopeError(
		"feishu",
		"cli_test",
		"user",
		[]string{"docx:document"},
	)
	var permissionErr *errs.PermissionError
	if !errors.As(err, &permissionErr) {
		t.Fatalf("error = %T, want *errs.PermissionError", err)
	}
	if permissionErr.Subtype != errs.SubtypeMissingScope {
		t.Fatalf("subtype = %q, want %q", permissionErr.Subtype, errs.SubtypeMissingScope)
	}
	if permissionErr.ConsoleURL != "" {
		t.Fatalf("missing_scope console_url = %q, want empty", permissionErr.ConsoleURL)
	}
	if len(permissionErr.MissingScopes) != 1 ||
		permissionErr.MissingScopes[0] != "docx:document" ||
		permissionErr.Identity != "user" {
		t.Fatalf("permission facts = %+v", permissionErr)
	}
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
			var got map[string]interface{}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
			}
			if got["ok"] != true || got["dry_run"] != true {
				t.Fatalf("unexpected dry-run envelope: %#v", got)
			}
			data := got["data"].(map[string]interface{})
			api := data["api"].([]interface{})
			call := api[0].(map[string]interface{})
			if call["url"] != tt.wantInURL {
				t.Errorf("url = %q, want %q\nstdout:\n%s", call["url"], tt.wantInURL, stdout.String())
			}
		})
	}
}

func TestServiceMethod_DryRunWithJq(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, driveSpec(), driveMethod("GET", nil), "get", "files", nil)
	cmd.SetArgs([]string{
		"--params", `{"file_token":"boxcn123abc"}`,
		"--dry-run",
		"--jq", ".data.api[0].url",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "/open-apis/drive/v1/files/boxcn123abc/copy"; got != want {
		t.Fatalf("jq output = %q, want %q", got, want)
	}
}

func TestServiceMethod_IMWriteDryRunReportsDefaultedIdentity(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]interface{}{
		"id":           "chats.create",
		"path":         "chats",
		"httpMethod":   "POST",
		"risk":         "write",
		"accessTokens": []interface{}{"user", "tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
	}
	notice, ok := env.Notice[imcontract.IdentityDefaultedNoticeKey].(map[string]interface{})
	if !ok || notice["resolved"] != "bot" {
		t.Fatalf("identity notice = %#v", env.Notice)
	}
	if got := stderr.String(); !strings.Contains(got, "warning: identity_defaulted:") {
		t.Fatalf("stderr = %q, want identity_defaulted warning", got)
	}
}

func TestServiceMethod_UnrestrictedIMWriteDryRunReportsDefaultedIdentity(t *testing.T) {
	f, stdout, stderr, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]interface{}{
		"id":         "chats.create",
		"path":       "chats",
		"httpMethod": "POST",
		"risk":       "write",
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--data", `{}`, "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
	}
	notice, ok := env.Notice[imcontract.IdentityDefaultedNoticeKey].(map[string]interface{})
	if !ok || notice["resolved"] != "bot" {
		t.Fatalf("unrestricted IM write identity notice = %#v", env.Notice)
	}
	if !strings.Contains(stderr.String(), "warning: identity_defaulted:") {
		t.Fatalf("stderr = %q, want identity_defaulted warning", stderr.String())
	}
}

func TestServiceMethod_IMWriteDryRunIdentityNoticeBoundaries(t *testing.T) {
	tests := []struct {
		name         string
		config       *core.CliConfig
		service      meta.Service
		accessTokens []interface{}
		args         []string
		wantIdentity core.Identity
	}{
		{
			name:         "explicit identity",
			service:      imSpec(),
			accessTokens: []interface{}{"user", "tenant"},
			args:         []string{"--as", "bot", "--data", `{}`, "--dry-run"},
		},
		{
			name:         "explicit auto",
			service:      imSpec(),
			accessTokens: []interface{}{"user", "tenant"},
			args:         []string{"--as", "auto", "--data", `{}`, "--dry-run"},
		},
		{
			name:         "single identity",
			service:      imSpec(),
			accessTokens: []interface{}{"tenant"},
			args:         []string{"--data", `{}`, "--dry-run"},
		},
		{
			name: "configured default identity",
			config: &core.CliConfig{
				AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
				DefaultAs: core.AsUser,
			},
			service:      imSpec(),
			accessTokens: []interface{}{"user", "tenant"},
			args:         []string{"--data", `{}`, "--dry-run"},
			wantIdentity: core.AsUser,
		},
		{
			name:         "non IM",
			service:      meta.ServiceFromMap(map[string]interface{}{"name": "svc", "servicePath": "/open-apis/svc/v1"}),
			accessTokens: []interface{}{"user", "tenant"},
			args:         []string{"--data", `{}`, "--dry-run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.config
			if config == nil {
				config = testConfig
			}
			f, stdout, stderr, _ := cmdutil.TestFactory(t, config)
			method := meta.FromMap(map[string]interface{}{
				"id":           "chats.create",
				"path":         "chats",
				"httpMethod":   "POST",
				"risk":         "write",
				"accessTokens": tt.accessTokens,
			})
			cmd := NewCmdServiceMethod(f, tt.service, method, "create", "chats", nil)
			cmd.SetArgs(tt.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var env output.Envelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
			}
			if tt.wantIdentity != "" && env.Identity != string(tt.wantIdentity) {
				t.Fatalf("identity = %q, want configured default %q", env.Identity, tt.wantIdentity)
			}
			if _, ok := env.Notice[imcontract.IdentityDefaultedNoticeKey]; ok {
				t.Fatalf("unexpected identity notice: %#v", env.Notice)
			}
			if strings.Contains(stderr.String(), "identity_defaulted") {
				t.Fatalf("unexpected identity warning: %q", stderr.String())
			}
		})
	}
}

func TestServiceMethod_IMWriteSuccessReportsDefaultedIdentity(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"chat_id": "oc_test"},
		},
	})
	method := meta.FromMap(map[string]interface{}{
		"id":           "chats.create",
		"path":         "chats",
		"httpMethod":   "POST",
		"risk":         "write",
		"accessTokens": []interface{}{"user", "tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--data", `{}`})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	notice, ok := env.Notice[imcontract.IdentityDefaultedNoticeKey].(map[string]interface{})
	if !ok || notice["resolved"] != "bot" {
		t.Fatalf("identity notice = %#v", env.Notice)
	}
	if got := strings.Count(stderr.String(), "warning: identity_defaulted:"); got != 1 {
		t.Fatalf("identity warning count = %d, stderr=%q", got, stderr.String())
	}
}

func TestServiceMethod_IMIdentityDefaultNoticeExcludesOutOfScopeCommands(t *testing.T) {
	tests := []struct {
		name   string
		config *core.CliConfig
		id     string
		method string
		risk   string
	}{
		{
			name:   "read",
			config: testConfig,
			id:     "chats.get",
			method: "GET",
			risk:   "read",
		},
		{
			name: "strict mode",
			config: &core.CliConfig{
				AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu, SupportedIdentities: 2,
			},
			id:     "chats.create",
			method: "POST",
			risk:   "write",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, stdout, stderr, _ := cmdutil.TestFactory(t, tt.config)
			method := meta.FromMap(map[string]interface{}{
				"id":           tt.id,
				"path":         "chats",
				"httpMethod":   tt.method,
				"risk":         tt.risk,
				"accessTokens": []interface{}{"user", "tenant"},
			})
			cmd := NewCmdServiceMethod(f, imSpec(), method, strings.TrimPrefix(tt.id, "chats."), "chats", nil)
			args := []string{"--dry-run"}
			if tt.method == "POST" {
				args = append(args, "--data", `{}`)
			}
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var env output.Envelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
			}
			if _, ok := env.Notice[imcontract.IdentityDefaultedNoticeKey]; ok {
				t.Fatalf("unexpected identity notice: %#v", env.Notice)
			}
			if strings.Contains(stderr.String(), "identity_defaulted") {
				t.Fatalf("unexpected identity warning: %q", stderr.String())
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
	var got map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if got["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", got["dry_run"])
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
	if _, hasMeta := got["meta"]; hasMeta {
		t.Fatalf("non-IM response unexpectedly gained completeness metadata: %s", stdout.String())
	}
	if _, hasHint := got["hint"]; hasHint {
		t.Fatalf("non-IM response unexpectedly gained an IM recovery hint: %s", stdout.String())
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

func TestGeneratedIMRequiredResultRejectsFalseSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/im/v1/chats",
		Body: map[string]any{"code": 0, "msg": "ok", "data": map[string]any{}},
	})
	method := meta.FromMap(map[string]any{
		"id": "chats.create", "path": "chats", "httpMethod": "POST",
		"risk": "write", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--as", "bot", "--data", `{}`})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid response")
	}
	requireProblem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse, 0)
	if stdout.Len() != 0 {
		t.Fatalf("false success reached stdout: %s", stdout.String())
	}
}

func TestIMContractManagedResponsesRequireValidJSON(t *testing.T) {
	const secret = "SECRET_MARKER"
	cases := []struct {
		name        string
		contentType string
		body        []byte
	}{
		{name: "empty", contentType: "application/json", body: []byte{}},
		{name: "plain", contentType: "text/plain", body: []byte(secret)},
		{name: "html", contentType: "text/html", body: []byte("<html>" + secret + "</html>")},
		{name: "malformed", contentType: "application/json", body: []byte(`{"secret":"` + secret + `"`)},
	}
	commands := []struct {
		name   string
		verb   string
		method meta.Method
		args   []string
	}{
		{
			name: "read",
			verb: "get",
			method: meta.FromMap(map[string]any{
				"id": "chats.get", "path": "chats/{chat_id}", "httpMethod": "GET",
				"risk": "read", "accessTokens": []any{"tenant"},
				"parameters": map[string]any{
					"chat_id": map[string]any{"type": "string", "location": "path", "required": true},
				},
			}),
			args: []string{"--as", "bot", "--params", `{"chat_id":"oc_x"}`},
		},
		{
			name: "write",
			verb: "create",
			method: meta.FromMap(map[string]any{
				"id": "chats.create", "path": "chats", "httpMethod": "POST",
				"risk": "write", "accessTokens": []any{"tenant"},
			}),
			args: []string{"--as", "bot", "--data", `{}`},
		},
	}

	for _, command := range commands {
		for _, tc := range cases {
			t.Run(command.name+"/"+tc.name, func(t *testing.T) {
				cmdutil.TestChdir(t, t.TempDir())
				f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
				reg.Register(&httpmock.Stub{
					URL:         "/open-apis/im/v1/chats",
					RawBody:     tc.body,
					ContentType: tc.contentType,
				})
				cmd := NewCmdServiceMethod(f, imSpec(), command.method, command.verb, "chats", nil)
				cmd.SetArgs(command.args)

				err := cmd.Execute()
				if err == nil {
					t.Fatal("expected invalid response")
				}
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryInternal ||
					problem.Subtype != errs.SubtypeInvalidResponse ||
					problem.Message != "IM contract response must be valid JSON" {
					t.Fatalf("problem = %#v, err=%T %v", problem, err, err)
				}
				if output.ExitCodeOf(err) != output.ExitInternal {
					t.Fatalf("exit = %d, want %d", output.ExitCodeOf(err), output.ExitInternal)
				}
				if stdout.Len() != 0 || stderr.Len() != 0 {
					t.Fatalf("unexpected output: stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
				if strings.Contains(err.Error(), secret) ||
					strings.Contains(problem.Message, secret) ||
					strings.Contains(problem.Hint, secret) ||
					strings.Contains(problem.LogID, secret) ||
					errors.Unwrap(err) != nil {
					t.Fatalf("response body or parse cause leaked: problem=%#v err=%#v", problem, err)
				}
			})
		}
	}
}

func TestIMContractManagedReadDirectTransportErrorIsRetryable(t *testing.T) {
	// No stub is registered, so the request fails at the direct transport call.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]any{
		"id": "chats.get", "path": "chats/{chat_id}", "httpMethod": "GET",
		"risk": "read", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"chat_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "get", "chats", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"chat_id":"oc_x"}`})

	err := cmd.Execute()
	problem, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if problem.Category != errs.CategoryNetwork || !problem.Retryable {
		t.Fatalf("problem = %#v", problem)
	}
}

func TestNonIMBinaryResponseStillDownloads(t *testing.T) {
	tmp := t.TempDir()
	cmdutil.TestChdir(t, tmp)
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL:         "/open-apis/svc/v1/items",
		RawBody:     []byte("binary-payload"),
		ContentType: "application/octet-stream",
		Headers: http.Header{
			"Content-Type":        []string{"application/octet-stream"},
			"Content-Disposition": []string{`attachment; filename="kept.bin"`},
		},
	})
	spec := meta.ServiceFromMap(map[string]any{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]any{
		"id": "items.get", "path": "items", "httpMethod": "GET",
		"risk": "read", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, spec, method, "get", "items", nil)
	cmd.SetArgs([]string{"--as", "bot"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "binary response detected") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	var downloaded map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &downloaded); err != nil {
		t.Fatalf("download metadata is not JSON: %v\n%s", err, stdout.String())
	}
	path, _ := downloaded["saved_path"].(string)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "binary-payload" {
		t.Fatalf("downloaded = %q", raw)
	}
}

func TestIMContractManagedWriteJQFailureUsesCompletionFallback(t *testing.T) {
	const secret = "SECRET_MARKER"
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"chat_id": "oc_x",
			"items":   []any{"safe-prefix", secret},
		}},
	})
	method := meta.FromMap(map[string]any{
		"id": "chats.create", "path": "chats", "httpMethod": "POST",
		"risk": "write", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{
		"--as", "bot", "--data", `{}`,
		"--jq", `.data.items[] | if . == "SECRET_MARKER" then error("SECRET_MARKER") else . end`,
	})

	err := cmd.Execute()
	assertIMPresentationFallback(t, stdout, stderr, err, output.ExitAPI, "api", "unknown",
		"Output failed after the IM write completed", "complete", secret, true)
}

func TestIMContractManagedWriteContentSafetyUsesCompletionFallback(t *testing.T) {
	const secret = "SECRET_MARKER"
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "block")
	provider := &serviceContentSafetyProvider{match: secret}
	extcs.Register(provider)
	t.Cleanup(func() { extcs.Register(nil) })

	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"chat_id": "oc_x",
			"subject": secret,
		}},
	})
	method := meta.FromMap(map[string]any{
		"id": "chats.create", "path": "chats", "httpMethod": "POST",
		"risk": "write", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	root := &cobra.Command{Use: "lark-cli"}
	root.AddCommand(cmd)
	root.SetArgs([]string{"create", "--as", "bot", "--data", `{}`})

	err := root.Execute()
	assertIMPresentationFallback(t, stdout, stderr, err, output.ExitContentSafety, "policy", "content_safety",
		"Output blocked after the IM write completed", "complete", secret, false)
}

func assertIMPresentationFallback(
	t *testing.T,
	stdout, stderr *bytes.Buffer,
	err error,
	exit int,
	category, subtype, message, status, secret string,
	wantJQError bool,
) {
	t.Helper()
	if err == nil || output.ExitCodeOf(err) != exit {
		t.Fatalf("error = %T %v, exit=%d want %d", err, err, output.ExitCodeOf(err), exit)
	}
	if wantJQError && !strings.Contains(stderr.String(),
		"error: jq projection failed after the IM write completed; inspect --jq") {
		t.Fatalf("stderr did not identify the jq failure: %q", stderr.String())
	}
	if !wantJQError && stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) ||
		strings.Contains(err.Error(), secret) {
		t.Fatalf("presentation detail leaked: stdout=%q stderr=%q err=%v", stdout.String(), stderr.String(), err)
	}
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatalf("fallback is not one JSON envelope: %v\n%s", jsonErr, stdout.String())
	}
	if len(env) != 3 || env["ok"] != false {
		t.Fatalf("fallback top-level fields = %#v", env)
	}
	data, _ := env["data"].(map[string]any)
	if len(data) != 1 {
		t.Fatalf("fallback data = %#v", data)
	}
	completion, _ := data["completion"].(map[string]any)
	if completion["status"] != status || completion["retry_scope"] != "none" {
		t.Fatalf("completion = %#v", completion)
	}
	problem, _ := env["error"].(map[string]any)
	if problem["type"] != category || problem["subtype"] != subtype || problem["message"] != message {
		t.Fatalf("error = %#v", problem)
	}
	if _, exists := env["presentation"]; exists {
		t.Fatalf("fallback introduced presentation: %#v", env)
	}
}

func TestGeneratedIMBatchPartialWritesCompletion(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/urgent_app",
		Body: map[string]any{
			"code": 0, "msg": "ok",
			"data": map[string]any{"invalid_user_id_list": []any{"ou_b"}},
		},
	})
	method := meta.FromMap(map[string]any{
		"id": "messages.urgent_app", "path": "messages/{message_id}/urgent_app", "httpMethod": "PATCH",
		"risk": "write", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"message_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "urgent_app", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x"}`, "--data", `{"user_id_list":["ou_a","ou_b"]}`})

	err := cmd.Execute()
	var partial *output.PartialFailureError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %T %v", err, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr must stay empty: %s", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env["ok"] != false || env["hint"] == "" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMBatchRejectsUnsupportedRequestBeforeAPI(t *testing.T) {
	// No HTTP stub is registered. A validation error therefore also proves the
	// malformed request evidence was rejected before transport.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]any{
		"id": "messages.urgent_app", "path": "messages/{message_id}/urgent_app", "httpMethod": "PATCH",
		"risk": "write", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"message_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "urgent_app", "messages", nil)
	cmd.SetArgs([]string{
		"--as", "bot",
		"--params", `{"message_id":"om_x"}`,
		"--data", `{"user_id_list":{"not":"a list"}}`,
	})

	err := cmd.Execute()
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryValidation ||
		problem.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("error = %T %#v", err, problem)
	}
}

func TestGeneratedIMTransientWriteRequiresSameKey(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		body        any
	}{
		{name: "plain body", contentType: "text/plain", body: "unavailable"},
		{name: "JSON body with unclassified code", contentType: "application/json", body: map[string]any{
			"code": 123456,
			"msg":  "unclassified business error",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _, _, reg := cmdutil.TestFactory(t, testConfig)
			reg.Register(&httpmock.Stub{
				URL:         "/open-apis/im/v1/chats",
				Status:      503,
				Body:        tc.body,
				ContentType: tc.contentType,
			})
			method := meta.FromMap(map[string]any{
				"id": "chats.create", "path": "chats", "httpMethod": "POST",
				"risk": "write", "accessTokens": []any{"tenant"},
				"parameters": map[string]any{
					"uuid": map[string]any{"type": "string", "location": "query"},
				},
			})
			cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
			cmd.SetArgs([]string{"--as", "bot", "--params", `{"uuid":"stable-key"}`, "--data", `{}`})

			err := cmd.Execute()
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T %v", err, err)
			}
			if p.Category != errs.CategoryNetwork ||
				p.Subtype != errs.SubtypeNetworkServer ||
				p.Code != http.StatusServiceUnavailable ||
				!p.Retryable ||
				p.Hint != "The write result is unknown. Retry only with the same idempotency key." {
				t.Fatalf("problem = %#v", p)
			}
		})
	}
}

func TestGeneratedIMModerationAlwaysReportsAcceptedUnverified(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/im/v1/chats/oc_x/moderation",
		Body: map[string]any{"code": 0, "msg": "ok", "data": nil},
	})
	method := meta.FromMap(map[string]any{
		"id": "chat.moderation.update", "path": "chats/{chat_id}/moderation", "httpMethod": "PUT",
		"risk": "write", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"chat_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "update", "chat.moderation", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"chat_id":"oc_x"}`, "--data", `{}`})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	completion := env["data"].(map[string]any)["completion"].(map[string]any)
	if completion["status"] != "accepted_unverified" || completion["final_state_verified"] != false ||
		env["hint"] != imcontract.HelpAcceptanceOnly.Text() {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMWriteRejectsPageAll(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]any{
		"id": "chats.create", "path": "chats", "httpMethod": "POST",
		"risk": "write", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--as", "bot", "--data", `{}`, "--page-all"})

	err := cmd.Execute()
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Message != "--page-all is not valid for an IM write command" {
		t.Fatalf("error = %T %#v", err, p)
	}
}

func TestGeneratedIMEntityReadRejectsPageAllBeforeAPI(t *testing.T) {
	// No HTTP stub is registered. The validation result therefore also proves
	// the hidden pagination flag is rejected before any API request is sent.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]any{
		"id": "chats.get", "path": "chats/{chat_id}", "httpMethod": "GET",
		"risk": "read", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"chat_id": map[string]any{"type": "string", "location": "path", "required": true},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "get", "chats", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"chat_id":"oc_x"}`, "--page-all"})

	err := cmd.Execute()
	p, ok := errs.ProblemOf(err)
	var validation *errs.ValidationError
	if !ok || p.Category != errs.CategoryValidation ||
		p.Message != "--page-all is not valid for this IM read command" ||
		!errors.As(err, &validation) || validation.Param != "--page-all" {
		t.Fatalf("error = %T %#v", err, p)
	}
}

func TestIMNonPaginatedReadRejectsPageAllBeforeIdentityResolution(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	f.Credential = credential.NewCredentialProvider(
		[]extcred.Provider{panicCredentialProvider{}},
		nil,
		nil,
		nil,
	)
	f.Config = func() (*core.CliConfig, error) {
		t.Fatal("config resolution must not run")
		return nil, nil
	}

	for _, key := range []imcontract.ContractKey{
		"im chats get",
		"im +messages-resources-download",
	} {
		t.Run(string(key), func(t *testing.T) {
			err := serviceMethodRun(&ServiceMethodOptions{
				Factory:     f,
				Ctx:         context.Background(),
				ContractKey: key,
				PageAll:     true,
			})
			p, ok := errs.ProblemOf(err)
			var validation *errs.ValidationError
			if !ok || p.Category != errs.CategoryValidation ||
				p.Message != "--page-all is not valid for this IM read command" ||
				!errors.As(err, &validation) || validation.Param != "--page-all" {
				t.Fatalf("error = %T %#v", err, p)
			}
		})
	}
}

func TestGeneratedIMWriteRejectsOutputBeforeAPI(t *testing.T) {
	// No HTTP stub is registered. Reaching the transport would therefore
	// produce a different error, so the typed validation result also proves
	// the API was not called.
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	method := meta.FromMap(map[string]any{
		"id": "chats.create", "path": "chats", "httpMethod": "POST",
		"risk": "write", "accessTokens": []any{"tenant"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), method, "create", "chats", nil)
	cmd.SetArgs([]string{"--as", "bot", "--data", `{}`, "--output", "result.json"})

	err := cmd.Execute()
	p, ok := errs.ProblemOf(err)
	var validation *errs.ValidationError
	if !ok || p.Category != errs.CategoryValidation || !errors.As(err, &validation) || validation.Param != "--output" {
		t.Fatalf("error = %T %#v", err, p)
	}
	if !strings.Contains(p.Hint, "completion result from stdout") {
		t.Fatalf("hint = %q", p.Hint)
	}
}

func imReadPagination(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	meta, ok := env["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta = %#v, want object", env["meta"])
	}
	pagination, ok := meta["pagination"].(map[string]any)
	if !ok {
		t.Fatalf("meta.pagination = %#v, want object", meta["pagination"])
	}
	return pagination
}

func imReadStopReason(t *testing.T, env map[string]any) string {
	t.Helper()
	notice, ok := env["_notice"].(map[string]any)
	if !ok {
		t.Fatalf("_notice = %#v, want object", env["_notice"])
	}
	readNotice, ok := notice["im_read"].(map[string]any)
	if !ok {
		t.Fatalf("_notice.im_read = %#v, want object", notice["im_read"])
	}
	reason, _ := readNotice["stop_reason"].(string)
	return reason
}

func TestGeneratedIMCollectionSinglePageReportsIncomplete(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"user_id": "ou_a"}}, "has_more": true, "page_token": "next",
		}},
	})
	method := generatedIMReadUsersMethod()
	cmd := NewCmdServiceMethod(f, imSpec(), method, "read_users", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x"}`})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	pagination := imReadPagination(t, env)
	if env["ok"] != true || pagination["complete"] != false || imReadStopReason(t, env) != "single_page" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	if _, exists := env["error"]; exists {
		t.Fatalf("successful IM read emitted error field: %#v", env)
	}
	if !strings.Contains(env["hint"].(string), "--page-all --page-limit 0") {
		t.Fatalf("missing recovery hint: %#v", env)
	}
}

func TestGeneratedIMCollectionPageAllExhausted(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"user_id": "ou_a"}}, "has_more": true, "page_token": "next",
		}},
	})
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"user_id": "ou_b"}}, "has_more": false,
		}},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), generatedIMReadUsersMethod(), "read_users", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x"}`, "--page-all", "--page-limit", "0", "--page-delay", "-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	pagination := imReadPagination(t, env)
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 2 || pagination["complete"] != true || imReadStopReason(t, env) != "exhausted" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMCollectionPageAllLateErrorKeepsPartialJSON(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"user_id": "ou_a"}}, "has_more": true, "page_token": "next",
		}},
	})
	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 230027, "msg": "not authorized"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), generatedIMReadUsersMethod(), "read_users", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x"}`, "--page-all", "--page-limit", "0", "--page-delay", "-1"})

	err := cmd.Execute()
	var partial *output.PartialFailureError
	if !errors.As(err, &partial) || partial.Code != output.ExitAuth {
		t.Fatalf("error = %T %v", err, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	items := env["data"].(map[string]any)["items"].([]any)
	pagination := imReadPagination(t, env)
	rawProblem, exists := env["error"]
	if !exists {
		t.Fatalf("late failure omitted structured error: %#v", env)
	}
	problem, ok := rawProblem.(map[string]any)
	if !ok {
		t.Fatalf("late failure error = %T, want object: %#v", rawProblem, env)
	}
	if len(items) != 1 || env["ok"] != false || pagination["complete"] != false ||
		imReadStopReason(t, env) != "api_error" || problem["type"] != "authorization" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMModerationPageAllLateErrorKeepsPartialJSON(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/chats/oc_x/moderation",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"moderation_setting": "moderator_list",
			"items":              []any{map[string]any{"user_id": "ou_a"}},
			"has_more":           true,
			"page_token":         "next",
		}},
	})
	reg.Register(&httpmock.Stub{
		URL:  "/open-apis/im/v1/chats/oc_x/moderation",
		Body: map[string]any{"code": 230027, "msg": "not authorized"},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), generatedIMModerationGetMethod(), "get", "chat.moderation", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"chat_id":"oc_x"}`, "--page-all", "--page-limit", "0", "--page-delay", "-1"})

	err := cmd.Execute()
	var partial *output.PartialFailureError
	if !errors.As(err, &partial) || partial.Code != output.ExitAuth {
		t.Fatalf("error = %T %v", err, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	items := env["data"].(map[string]any)["items"].([]any)
	pagination := imReadPagination(t, env)
	problem := env["error"].(map[string]any)
	if len(items) != 1 || env["ok"] != false ||
		pagination["complete"] != false || imReadStopReason(t, env) != "api_error" ||
		problem["type"] != "authorization" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMCollectionPageAllJSON5xxUsesHTTPStatus(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{map[string]any{"user_id": "ou_a"}}, "has_more": true, "page_token": "next",
		}},
	})
	reg.Register(&httpmock.Stub{
		URL:    "/open-apis/im/v1/messages/om_x/read_users",
		Status: http.StatusServiceUnavailable,
		Body:   map[string]any{"code": 123456, "msg": "unclassified server failure"},
		Headers: http.Header{
			"Content-Type":                        []string{"application/json"},
			http.CanonicalHeaderKey("x-tt-logid"): []string{"log-page-503"},
		},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), generatedIMReadUsersMethod(), "read_users", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x"}`, "--page-all", "--page-limit", "0", "--page-delay", "-1"})

	err := cmd.Execute()
	var partial *output.PartialFailureError
	if !errors.As(err, &partial) || partial.Code != output.ExitNetwork {
		t.Fatalf("error = %T %v", err, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %s", stderr.String())
	}
	var env map[string]any
	if jsonErr := json.Unmarshal(stdout.Bytes(), &env); jsonErr != nil {
		t.Fatal(jsonErr)
	}
	problem, _ := env["error"].(map[string]any)
	pagination := imReadPagination(t, env)
	items := env["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || env["ok"] != false ||
		pagination["complete"] != false || imReadStopReason(t, env) != "api_error" ||
		problem["type"] != "network" || problem["subtype"] != "server_error" ||
		problem["code"] != float64(http.StatusServiceUnavailable) ||
		problem["log_id"] != "log-page-503" || problem["retryable"] != true {
		t.Fatalf("unexpected envelope: %#v", env)
	}
}

func TestGeneratedIMCollectionStartTokenNeverClaimsComplete(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, testConfig)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/im/v1/messages/om_x/read_users",
		Body: map[string]any{"code": 0, "data": map[string]any{
			"items": []any{}, "has_more": false,
		}},
	})
	cmd := NewCmdServiceMethod(f, imSpec(), generatedIMReadUsersMethod(), "read_users", "messages", nil)
	cmd.SetArgs([]string{"--as", "bot", "--params", `{"message_id":"om_x","page_token":"middle"}`})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	pagination := imReadPagination(t, env)
	if pagination["complete"] != false || imReadStopReason(t, env) != "start_page_token" {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	data := env["data"].(map[string]any)
	for _, field := range []string{"has_more", "page_token", "next_page_token"} {
		if _, exists := data[field]; exists {
			t.Fatalf("start-token result exposes page-relative %s alongside operation completeness: %#v", field, env)
		}
	}
}

func generatedIMReadUsersMethod() meta.Method {
	return meta.FromMap(map[string]any{
		"id": "messages.read_users", "path": "messages/{message_id}/read_users", "httpMethod": "GET",
		"risk": "read", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"message_id": map[string]any{"type": "string", "location": "path", "required": true},
			"page_token": map[string]any{"type": "string", "location": "query"},
		},
	})
}

func generatedIMModerationGetMethod() meta.Method {
	return meta.FromMap(map[string]any{
		"id": "chat.moderation.get", "path": "chats/{chat_id}/moderation", "httpMethod": "GET",
		"risk": "read", "accessTokens": []any{"tenant"},
		"parameters": map[string]any{
			"chat_id":    map[string]any{"type": "string", "location": "path", "required": true},
			"page_token": map[string]any{"type": "string", "location": "query"},
		},
	})
}

func TestNonIMWriteOutputKeepsExistingFilePath(t *testing.T) {
	tmp := t.TempDir()
	cmdutil.TestChdir(t, tmp)
	f, _, _, reg := cmdutil.TestFactory(t, testConfig)
	calls := 0
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/svc/v1/items",
		OnMatch: func(*http.Request) {
			calls++
		},
		Body: map[string]any{"code": 0, "data": map[string]any{"id": "item_x"}},
	})
	spec := meta.ServiceFromMap(map[string]any{"name": "svc", "servicePath": "/open-apis/svc/v1"})
	method := meta.FromMap(map[string]any{
		"id": "items.create", "path": "items", "httpMethod": "POST", "risk": "write",
		"accessTokens": []any{"tenant"},
	})
	outputPath := "response.json"
	cmd := NewCmdServiceMethod(f, spec, method, "create", "items", nil)
	cmd.SetArgs([]string{"--as", "bot", "--data", `{}`, "--output", outputPath})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("API calls = %d, want 1", calls)
	}
	raw, err := os.ReadFile(filepath.Join(tmp, outputPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"item_x"`) {
		t.Fatalf("saved response = %s", raw)
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
	var env map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, out)
	}
	if env["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", env["dry_run"])
	}
	data := env["data"].(map[string]interface{})
	api := data["api"].([]interface{})
	call := api[0].(map[string]interface{})
	body := call["body"].(map[string]interface{})
	file := body["file"].(map[string]interface{})
	if file["field"] != "image" || file["path"] != tmpFile {
		t.Fatalf("unexpected file dry-run body: %#v", body)
	}
	if strings.Contains(out, "=== Dry Run ===") {
		t.Fatalf("stdout should not contain dry-run banner: %s", out)
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
