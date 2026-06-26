// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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

func TestDryRunAPI_MarshalJSON(t *testing.T) {
	dr := NewDryRunAPI().
		Desc("test api").
		GET("/open-apis/test").
		Set("as", "user")

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
	if m["as"] != "user" {
		t.Errorf("expected as=user, got: %v", m["as"])
	}
	api, ok := m["api"].([]interface{})
	if !ok || len(api) != 1 {
		t.Errorf("expected 1 api call, got: %v", m["api"])
	}
}

func TestPrintDryRunJSONDoesNotHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRun(&buf, client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/docs_ai/v1/documents/<obj_token from step 1>/fetch",
		Data:   map[string]interface{}{"content": "<docx>hello</docx>"},
		As:     "bot",
	}, &core.CliConfig{AppID: "app123"}, "json")
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		`/open-apis/docs_ai/v1/documents/<obj_token from step 1>/fetch`,
		`"<docx>hello</docx>"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("JSON output missing unescaped %q: %s", want, out)
		}
	}
	if strings.Contains(out, `\u003c`) || strings.Contains(out, `\u003e`) {
		t.Fatalf("JSON output HTML-escaped angle brackets: %s", out)
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
	err := PrintDryRun(&buf, client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test",
		As:     "user",
	}, &core.CliConfig{AppID: "app123"}, "json")
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "=== Dry Run ===") {
		t.Errorf("expected header, got: %s", out)
	}
	if !strings.Contains(out, "app123") {
		t.Errorf("expected appId in output, got: %s", out)
	}
}

func TestPrintDryRun_Pretty(t *testing.T) {
	var buf bytes.Buffer
	err := PrintDryRun(&buf, client.RawApiRequest{
		Method: "POST",
		URL:    "/open-apis/test",
		Data:   map[string]interface{}{"key": "val"},
		As:     "bot",
	}, &core.CliConfig{AppID: "app456"}, "pretty")
	if err != nil {
		t.Fatalf("PrintDryRun failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "POST /open-apis/test") {
		t.Errorf("expected POST line in pretty output, got: %s", out)
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
