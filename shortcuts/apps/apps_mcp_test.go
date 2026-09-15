// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

const mcpTestSecret = "MCP_EXAMPLE_TOKEN"
const mcpTestConfig = `{"mcpServers":{"example":{"url":"https://example.com/mcp?x-mcp-token=MCP_EXAMPLE_TOKEN","headers":{"X-Mcp-Token":"MCP_EXAMPLE_TOKEN"},"env":{"TOKEN":"MCP_EXAMPLE_TOKEN"},"future":{"enabled":true}}}}`

func TestMCPExecute(t *testing.T) {
	for _, secret := range []bool{false, true} {
		for _, credential := range []bool{false, true} {
			name := "connection"
			sc := AppsMCPGet
			method := "GET"
			data := map[string]interface{}{"config_json": mcpTestConfig}
			if credential {
				name = "credential"
				sc = AppsMCPKeyCreate
				method = "POST"
				data = map[string]interface{}{"credential_id": "key_example", "plain_key": mcpTestSecret}
			}
			t.Run(name+map[bool]string{false: "_redacted", true: "_secret"}[secret], func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				flags := map[string]string{"app-id": "app_example"}
				if secret {
					flags["include-secret"] = "true"
				}
				r, out, reg := newOpenAPIKeyRCtx(t, map[string]string{"app-id": "string", "include-secret": "bool"}, flags)
				stub := &httpmock.Stub{Method: method, URL: "/open-apis/spark/v1/apps/app_example/mcp/" + name, Body: map[string]interface{}{"code": 0, "data": data}}
				reg.Register(stub)
				if err := sc.Execute(context.Background(), r); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(out.String(), mcpTestSecret) != secret {
					t.Fatalf("secret visibility incorrect: %s", out.String())
				}
				var envelope struct {
					Data struct {
						Config       json.RawMessage `json:"config"`
						CredentialID string          `json:"credential_id"`
						Redacted     bool            `json:"redacted"`
					} `json:"data"`
				}
				if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Data.Redacted == secret {
					t.Fatal("incorrect redaction flag")
				}
				if credential && envelope.Data.CredentialID != "key_example" {
					t.Fatal("missing credential ID")
				}
				if !credential && secret {
					var got, want interface{}
					_ = json.Unmarshal(envelope.Data.Config, &got)
					_ = json.Unmarshal([]byte(mcpTestConfig), &want)
					gotJSON, _ := json.Marshal(got)
					wantJSON, _ := json.Marshal(want)
					if string(gotJSON) != string(wantJSON) {
						t.Fatal("explicit output lost config fields")
					}
				}
				if !credential && !secret && (strings.Contains(out.String(), "future") || strings.Contains(out.String(), "?")) {
					t.Fatal("unexpected fields in redacted config")
				}
				if len(stub.CapturedBody) != 0 {
					t.Fatalf("unexpected request body %s", stub.CapturedBody)
				}
			})
		}
	}
}

func TestMCPInvalidResponses(t *testing.T) {
	for _, tc := range []struct {
		name string
		sc   common.Shortcut
		data map[string]interface{}
	}{
		{"missing_config", AppsMCPGet, map[string]interface{}{}},
		{"wrong_config_type", AppsMCPGet, map[string]interface{}{"config_json": 42}},
		{"malformed_config", AppsMCPGet, map[string]interface{}{"config_json": "invalid"}},
		{"empty_config", AppsMCPGet, map[string]interface{}{"config_json": "{}"}},
		{"empty_servers", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{}}`}},
		{"invalid_url", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"x":{"url":"javascript:alert(1)"}}}`}},
		{"missing_key", AppsMCPKeyCreate, map[string]interface{}{"credential_id": "key_example"}},
		{"missing_id", AppsMCPKeyCreate, map[string]interface{}{"plain_key": mcpTestSecret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, out, reg := newOpenAPIKeyRCtx(t, map[string]string{"app-id": "string", "include-secret": "bool"}, map[string]string{"app-id": "app_example"})
			reg.Register(&httpmock.Stub{URL: "/mcp/", Body: map[string]interface{}{"code": 0, "data": tc.data}})
			err := tc.sc.Execute(context.Background(), r)
			var typed *errs.InternalError
			if !errors.As(err, &typed) || typed.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("expected invalid_response, got %v", err)
			}
			if out.Len() != 0 {
				t.Fatal("invalid response emitted success")
			}
		})
	}
}

func TestMCPPreservesAPIErrors(t *testing.T) {
	for _, sc := range []common.Shortcut{AppsMCPGet, AppsMCPKeyCreate} {
		t.Run(sc.Command, func(t *testing.T) {
			r, out, reg := newOpenAPIKeyRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": "app_example"})
			reg.Register(&httpmock.Stub{URL: "/mcp/", Status: 403, Headers: http.Header{"X-Tt-Logid": []string{"test-log-id"}}, Body: map[string]interface{}{"code": 99991679, "msg": "Access denied"}})
			err := sc.Execute(context.Background(), r)
			if err == nil {
				t.Fatal("API error swallowed")
			}
			var typed *errs.PermissionError
			if !errors.As(err, &typed) {
				t.Fatalf("expected permission error, got %T: %v", err, err)
			}
			if typed.Code != 99991679 || typed.LogID != "test-log-id" {
				t.Fatalf("lost API metadata: %+v", typed)
			}
			if out.Len() != 0 {
				t.Fatal("failure emitted success")
			}
		})
	}
}

func TestMCPMetadataAndDryRun(t *testing.T) {
	for _, sc := range []common.Shortcut{AppsMCPGet, AppsMCPKeyCreate} {
		t.Run(sc.Command, func(t *testing.T) {
			if sc.Risk != "write" || len(sc.AuthTypes) != 1 || sc.AuthTypes[0] != "user" {
				t.Fatal("incorrect risk/identity")
			}
			registered := false
			for _, s := range Shortcuts() {
				if s.Command == sc.Command {
					registered = true
				}
			}
			if !registered {
				t.Fatal("command not registered")
			}
			r, _, _ := newOpenAPIKeyRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": "app_example"})
			if err := sc.Validate(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(sc.DryRun(context.Background(), r))
			if err != nil {
				t.Fatal(err)
			}
			method, resource := "GET", "connection"
			if sc.Command == "+mcp-key-create" {
				method, resource = "POST", "credential"
			}
			if !strings.Contains(string(raw), method) || !strings.Contains(string(raw), "/apps/app_example/mcp/"+resource) {
				t.Fatalf("incorrect dry-run %s", raw)
			}
			empty, _, _ := newOpenAPIKeyRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": "  "})
			var typed *errs.ValidationError
			if err := sc.Validate(context.Background(), empty); !errors.As(err, &typed) {
				t.Fatalf("expected validation error: %v", err)
			}
		})
	}
}

func TestMCPRedactionHidesURLCredentialsAndAllHeaders(t *testing.T) {
	// Build a synthetic credential-bearing URL without publishing a URL literal
	// that scanners or documentation tools could mistake for a real credential.
	fixtureURL := url.URL{Scheme: "https", Host: "example.com", Path: "/mcp",
		User: url.UserPassword("user", mcpTestSecret), RawQuery: "key=" + mcpTestSecret, Fragment: mcpTestSecret}
	fixture, err := json.Marshal(map[string]interface{}{"mcpServers": map[string]interface{}{"x": map[string]interface{}{
		"url": fixtureURL.String(), "headers": map[string]string{"Authorization": mcpTestSecret, "unexpected": mcpTestSecret}, "args": []string{mcpTestSecret},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	raw := string(fixture)
	got, err := mcpConnectionOutput(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), mcpTestSecret) || strings.Contains(string(got), "user") || strings.Contains(string(got), "args") {
		t.Fatalf("leaked credentials: %s", got)
	}
}
