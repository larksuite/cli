// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

const mcpTestSecret = "MCP_EXAMPLE_TOKEN"
const mcpTestConfig = `{"mcpServers":{"app_example":{"url":"https://example.com/mcp?x-mcp-token=MCP_EXAMPLE_TOKEN","headers":{"X-Mcp-Token":"MCP_EXAMPLE_TOKEN"},"env":{"TOKEN":"MCP_EXAMPLE_TOKEN"},"future":{"enabled":true}}}}`

func TestMCPGetOutputsFullURL(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	r, out, reg := newAppsRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": " \napp_example\t "})
	stub := &httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_example/mcp/connection", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"config_json": mcpTestConfig}}}
	reg.Register(stub)
	if err := AppsMCPGet.Execute(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if got, want := envelope.Data["url"], "https://example.com/mcp?x-mcp-token="+mcpTestSecret; got != want {
		t.Fatalf("url = %v, want %q", got, want)
	}
	if len(envelope.Data) != 1 {
		t.Fatalf("unexpected fields in output: %v", envelope.Data)
	}
	if stderr := r.IO().ErrOut.(*bytes.Buffer).String(); stderr != "" {
		t.Fatalf("+mcp-get must write only the URL result, got stderr %q", stderr)
	}
}

func TestMCPServerURLIsLookedUpByAppID(t *testing.T) {
	// The requested app's entry is selected by key, whatever its position.
	const config = `{"mcpServers":{"app_other":{"url":"https://example.com/other?x-mcp-token=to"},"app_example":{"url":"https://example.com/mine?x-mcp-token=tm"}}}`
	got, err := mcpServerURL(config, "app_example")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/mine?x-mcp-token=tm" {
		t.Fatalf("url = %q, want the entry keyed by the app ID", got)
	}
}

func TestMCPGetPrettyPrintsOnlyURL(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/spark/v1/apps/app_x/mcp/connection", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"config_json": strings.ReplaceAll(mcpTestConfig, `"app_example"`, `"app_x"`)}}})
	if err := runAppsShortcut(t, AppsMCPGet, []string{"+mcp-get", "--app-id", "app_x", "--format", "pretty", "--as", "user"}, factory, stdout); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(stdout.String()), "https://example.com/mcp?x-mcp-token="+mcpTestSecret; got != want {
		t.Fatalf("pretty output = %q, want only the URL %q", got, want)
	}
}

func TestMCPGetRejectsRemovedIncludeSecretFlag(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsMCPGet, []string{"+mcp-get", "--app-id", "app_x", "--include-secret", "--as", "user"}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "include-secret") {
		t.Fatalf("expected unknown flag error for --include-secret, got %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatal("rejected flag emitted success output")
	}
}

func TestMCPKeyCreateExecute(t *testing.T) {
	for _, secret := range []bool{false, true} {
		t.Run(map[bool]string{false: "redacted", true: "secret"}[secret], func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			flags := map[string]string{"app-id": " \napp_example\t "}
			if secret {
				flags["include-secret"] = "true"
			}
			r, out, reg := newAppsRCtx(t, map[string]string{"app-id": "string", "include-secret": "bool"}, flags)
			stub := &httpmock.Stub{Method: "POST", URL: "/open-apis/spark/v1/apps/app_example/mcp/credential", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"credential_id": "key_example", "plain_key": mcpTestSecret}}}
			reg.Register(stub)
			if err := AppsMCPKeyCreate.Execute(context.Background(), r); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), mcpTestSecret) != secret {
				t.Fatalf("secret visibility incorrect: %s", out.String())
			}
			warning := r.IO().ErrOut.(*bytes.Buffer).String()
			if strings.Contains(warning, "output contains MCP credentials") != secret {
				t.Fatalf("incorrect credential warning: %q", warning)
			}
			var envelope struct {
				Data struct {
					CredentialID string `json:"credential_id"`
					Redacted     bool   `json:"redacted"`
				} `json:"data"`
			}
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Data.Redacted == secret || envelope.Data.CredentialID != "key_example" {
				t.Fatalf("incorrect output: %+v", envelope.Data)
			}
			if len(stub.CapturedBody) != 0 {
				t.Fatalf("unexpected request body %s", stub.CapturedBody)
			}
		})
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
		{"no_server_for_app", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_other":{"url":"https://example.com/other?x-mcp-token=to"}}}`}},
		{"app_server_invalid", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"javascript:alert(1)"},"app_other":{"url":"https://example.com/other?x-mcp-token=to"}}}`}},
		{"url_without_token", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"https://example.com/mcp"}}}`}},
		{"url_with_empty_token", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"https://example.com/mcp?x-mcp-token="}}}`}},
		{"url_with_blank_token", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"https://example.com/mcp?x-mcp-token=%20"}}}`}},
		{"token_only_in_header", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"https://example.com/mcp","headers":{"X-Mcp-Token":"MCP_EXAMPLE_TOKEN"}}}}`}},
		{"invalid_url", AppsMCPGet, map[string]interface{}{"config_json": `{"mcpServers":{"app_example":{"url":"javascript:alert(1)"}}}`}},
		{"missing_key", AppsMCPKeyCreate, map[string]interface{}{"credential_id": "key_example"}},
		{"missing_id", AppsMCPKeyCreate, map[string]interface{}{"plain_key": mcpTestSecret}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, out, reg := newAppsRCtx(t, map[string]string{"app-id": "string", "include-secret": "bool"}, map[string]string{"app-id": "app_example"})
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
			r, out, reg := newAppsRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": "app_example"})
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
			r, _, _ := newAppsRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": " \napp_example\t "})
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
			empty, _, _ := newAppsRCtx(t, map[string]string{"app-id": "string"}, map[string]string{"app-id": "  "})
			var typed *errs.ValidationError
			if err := sc.Validate(context.Background(), empty); !errors.As(err, &typed) {
				t.Fatalf("expected validation error: %v", err)
			}
		})
	}
}
