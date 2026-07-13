// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
)

func TestDryRunAPI_SingleGET(t *testing.T) {
	dr := NewDryRunAPI().
		Desc("list calendars").
		GET("/open-apis/calendar/v4/calendars")

	text := dr.Format()
	if !strings.Contains(text, "# list calendars") {
		t.Errorf("expected description in text output, got: %s", text)
	}
	if !strings.Contains(text, "GET /open-apis/calendar/v4/calendars") {
		t.Errorf("expected GET line in text output, got: %s", text)
	}
}

func TestDryRunAPI_WithParams(t *testing.T) {
	dr := NewDryRunAPI().
		GET("/open-apis/test").
		Params(map[string]interface{}{"page_size": 20})

	text := dr.Format()
	if !strings.Contains(text, "page_size=20") {
		t.Errorf("expected query params in text output, got: %s", text)
	}
}

func TestDryRunAPI_WithBody(t *testing.T) {
	dr := NewDryRunAPI().
		POST("/open-apis/test").
		Body(map[string]interface{}{"title": "hello"})

	text := dr.Format()
	if !strings.Contains(text, "POST /open-apis/test") {
		t.Errorf("expected POST line, got: %s", text)
	}
	if !strings.Contains(text, `"title"`) {
		t.Errorf("expected body in output, got: %s", text)
	}
}

func TestDryRunAPI_ResolveURL(t *testing.T) {
	dr := NewDryRunAPI().
		GET("/open-apis/calendar/v4/calendars/:calendar_id/events").
		Set("calendar_id", "cal_abc123")

	text := dr.Format()
	if !strings.Contains(text, "cal_abc123") {
		t.Errorf("expected resolved calendar_id in URL, got: %s", text)
	}
	if strings.Contains(text, ":calendar_id") {
		t.Errorf("expected placeholder to be resolved, got: %s", text)
	}
}

func TestDryRunAPI_ResolveURLMatchesFullPlaceholderOnly(t *testing.T) {
	dr := NewDryRunAPI().
		GET("/open-apis/task/v2/tasks/:assignee_id").
		Set("assignee", "ou_bot")

	text := dr.Format()
	if strings.Contains(text, "ou_bot_id") {
		t.Fatalf("prefix placeholder key corrupted longer token: %s", text)
	}
	if !strings.Contains(text, ":assignee_id") {
		t.Fatalf("missing unresolved placeholder, got: %s", text)
	}

	dr.Set("assignee_id", "ou_abc/123")
	text = dr.Format()
	if !strings.Contains(text, "/open-apis/task/v2/tasks/ou_abc%2F123") {
		t.Fatalf("expected full placeholder replacement with path escaping, got: %s", text)
	}
}

func TestDryRunAPI_MarshalJSON(t *testing.T) {
	dr := NewDryRunAPI().
		Desc("test api").
		GET("/open-apis/test").
		Set("note", "audit")

	data, err := json.Marshal(dr)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if m["description"] != "test api" {
		t.Errorf("expected description, got: %v", m["description"])
	}
	if m["note"] != "audit" {
		t.Errorf("expected note=audit, got: %v", m["note"])
	}
	api, ok := m["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Errorf("expected 1 api call, got: %v", m["api"])
	}
}

func TestDryRunAPI_MultipleCalls(t *testing.T) {
	dr := NewDryRunAPI().
		GET("/open-apis/first").Desc("step 1").
		POST("/open-apis/second").Desc("step 2")

	text := dr.Format()
	if !strings.Contains(text, "# step 1") || !strings.Contains(text, "# step 2") {
		t.Errorf("expected both step descriptions, got: %s", text)
	}
	if !strings.Contains(text, "GET /open-apis/first") || !strings.Contains(text, "POST /open-apis/second") {
		t.Errorf("expected both calls, got: %s", text)
	}
}

func TestDryRunAPI_ExtraFieldsOnly(t *testing.T) {
	dr := NewDryRunAPI().
		Desc("info only").
		Set("calendar_id", "cal_123").
		Set("summary", "My Calendar")

	text := dr.Format()
	if !strings.Contains(text, "calendar_id: cal_123") {
		t.Errorf("expected extra field, got: %s", text)
	}
	if !strings.Contains(text, "summary: My Calendar") {
		t.Errorf("expected extra field, got: %s", text)
	}
}

func TestPrintDryRun_JSON(t *testing.T) {
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	err := PrintDryRun(client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "user",
	}, &core.CliConfig{AppID: "app123"}, DryRunOutputOptions{
		Format:      "json",
		CommandPath: "lark-cli api",
		Identity:    core.AsUser,
		Out:         &buf,
		ErrOut:      &errBuf,
	})
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "=== Dry Run ===") {
		t.Fatalf("JSON stdout must not contain banner, got: %s", out)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, out)
	}
	if env["ok"] != true || env["identity"] != "user" || env["dry_run"] != true {
		t.Fatalf("unexpected envelope: %#v", env)
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected data: %#v", env["data"])
	}
	dctx, ok := data["context"].(map[string]interface{})
	if !ok || dctx["app_id"] != "app123" {
		t.Fatalf("unexpected data.context: %#v", data["context"])
	}
	if _, exists := data["as"]; exists {
		t.Fatalf("data.as must not appear; identity lives at the envelope top level: %#v", data)
	}
	api, ok := data["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Fatalf("api = %#v, want one call", data["api"])
	}
	call, ok := api[0].(map[string]interface{})
	if !ok || call["url"] != "/open-apis/test" {
		t.Fatalf("api[0] = %#v", api[0])
	}
}

func TestPrintDryRun_Pretty(t *testing.T) {
	var buf bytes.Buffer
	var errBuf bytes.Buffer
	err := PrintDryRun(client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/test",
		Data:   map[string]interface{}{"key": "val"},
		As:     "bot",
	}, &core.CliConfig{AppID: "app456"}, DryRunOutputOptions{
		Format:   "pretty",
		Identity: core.AsBot,
		Out:      &buf,
		ErrOut:   &errBuf,
	})
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "POST /open-apis/test") {
		t.Errorf("expected POST line in pretty output, got: %s", out)
	}
	if !strings.HasPrefix(out, "# dry-run: request not sent\n") {
		t.Fatalf("pretty stdout should start with the dry-run marker, got: %s", out)
	}
	if strings.Contains(out, "=== Dry Run ===") {
		t.Fatalf("pretty stdout must not contain banner, got: %s", out)
	}
	if !strings.Contains(errBuf.String(), "=== Dry Run ===") {
		t.Fatalf("pretty stderr should contain banner, got: %s", errBuf.String())
	}
}

func TestPrintDryRun_WithJqUsesEnvelope(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRun(client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "bot",
	}, &core.CliConfig{AppID: "app123"}, DryRunOutputOptions{
		Format:   "json",
		JqExpr:   ".data.api[0].url",
		Identity: core.AsBot,
		Out:      &buf,
		ErrOut:   io.Discard,
	})
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "/open-apis/test" {
		t.Fatalf("jq output = %q, want /open-apis/test", got)
	}
}

func TestPrintDryRunWithFile_JSONEnvelope(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRunWithFile(client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		As:     "bot",
	}, &core.CliConfig{AppID: "app123", UserOpenId: "ou_tester"}, DryRunOutputOptions{
		Format:   "json",
		Identity: core.AsBot,
		Out:      &buf,
		ErrOut:   io.Discard,
	}, FileUploadMeta{FieldName: "file", FilePath: "report.txt", FormFields: map[string]any{"parent": "fld"}})
	if err != nil {
		t.Fatalf("PrintDryRunWithFile failed: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, buf.String())
	}
	if env["dry_run"] != true {
		t.Fatalf("dry_run = %#v, want true", env["dry_run"])
	}
	data := env["data"].(map[string]interface{})
	api := data["api"].([]interface{})
	call := api[0].(map[string]interface{})
	body := call["body"].(map[string]interface{})
	file := body["file"].(map[string]interface{})
	if file["path"] != "report.txt" {
		t.Fatalf("file body = %#v", body)
	}
	dctx, ok := data["context"].(map[string]interface{})
	if !ok || dctx["app_id"] != "app123" || dctx["user_open_id"] != "ou_tester" {
		t.Fatalf("unexpected data.context: %#v", data["context"])
	}
	for _, legacy := range []string{"as", "appId", "userOpenId"} {
		if _, exists := data[legacy]; exists {
			t.Fatalf("legacy key %q must not appear in data: %#v", legacy, data)
		}
	}
}

func TestPrintDryRun_MethodTranscribedVerbatim(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRun(client.RawApiRequest{
		Method: "OPTIONS",
		URL:    "/open-apis/test",
		As:     "bot",
	}, &core.CliConfig{AppID: "app123"}, DryRunOutputOptions{
		Format:   "json",
		Identity: core.AsBot,
		Out:      &buf,
		ErrOut:   io.Discard,
	})
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, buf.String())
	}
	call := env["data"].(map[string]interface{})["api"].([]interface{})[0].(map[string]interface{})
	if call["method"] != "OPTIONS" {
		t.Fatalf("method = %#v, want OPTIONS transcribed verbatim (not coerced to GET)", call["method"])
	}
}

func TestPrintDryRun_EmptyConfigOmitsContext(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRun(client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
	}, &core.CliConfig{}, DryRunOutputOptions{
		Format: "json",
		Out:    &buf,
		ErrOut: io.Discard,
	})
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("dry-run stdout is not JSON: %v\n%s", err, buf.String())
	}
	data := env["data"].(map[string]interface{})
	if _, exists := data["context"]; exists {
		t.Fatalf("empty app/user context must be omitted entirely, got: %#v", data["context"])
	}
}

func TestWriteDryRun_NilPreviewIsInternalError(t *testing.T) {
	err := WriteDryRun(nil, DryRunOutputOptions{Format: "json", Out: io.Discard})
	if err == nil {
		t.Fatal("WriteDryRun(nil) should fail instead of emitting an empty preview")
	}
	var internal *errs.InternalError
	if !errors.As(err, &internal) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
}

func TestDryRunFormatValue(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
		want string
	}{
		{"string", "hello", "hello"},
		{"nil", nil, ""},
		{"number", 42, "42"},
		{"bool", true, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dryRunFormatValue(tt.v); got != tt.want {
				t.Errorf("dryRunFormatValue(%v) = %q, want %q", tt.v, got, tt.want)
			}
		})
	}
}
