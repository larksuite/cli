// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mcp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

func asExit(t *testing.T, err error) *output.ExitError {
	t.Helper()
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T: %v", err, err)
	}
	return ee
}

func newServer(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestJSONRPC_ToolsList(t *testing.T) {
	srv := newServer(t, 200, "application/json",
		`{"jsonrpc":"2.0","id":"1","result":{"tools":[{"name":"search"},{"name":"get_item"}]}}`)

	got, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("jsonRPC: %v", err)
	}
	res, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T, want map", got)
	}
	tools, ok := res["tools"].([]interface{})
	if !ok || len(tools) != 2 {
		t.Fatalf("tools = %v, want 2 entries", res["tools"])
	}
}

func TestJSONRPC_ToolsCall(t *testing.T) {
	srv := newServer(t, 200, "application/json",
		`{"jsonrpc":"2.0","id":"1","result":{"content":[{"type":"text","text":"hi"}],"isError":false}}`)

	got, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/call",
		map[string]any{"name": "search", "arguments": map[string]any{"q": "x"}}, nil)
	if err != nil {
		t.Fatalf("jsonRPC: %v", err)
	}
	if _, ok := got.(map[string]interface{})["content"]; !ok {
		t.Fatalf("result missing content: %v", got)
	}
}

func TestJSONRPC_SSEFraming(t *testing.T) {
	// Streamable HTTP may frame the JSON-RPC payload as a Server-Sent Event.
	body := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"1\",\"result\":{\"ok\":true}}\n\n"
	srv := newServer(t, 200, "text/event-stream", body)

	got, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, nil)
	if err != nil {
		t.Fatalf("jsonRPC (SSE): %v", err)
	}
	if got.(map[string]interface{})["ok"] != true {
		t.Fatalf("SSE result = %v, want ok:true", got)
	}
}

func TestJSONRPC_ErrorPayload(t *testing.T) {
	srv := newServer(t, 200, "application/json",
		`{"jsonrpc":"2.0","id":"1","error":{"code":-32601,"message":"Method not found"}}`)

	_, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, nil)
	ee := asExit(t, err)
	if ee.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want ExitAPI(%d)", ee.Code, output.ExitAPI)
	}
	if ee.Detail == nil || ee.Detail.Type != "mcp_error" {
		t.Errorf("error type = %v, want mcp_error", ee.Detail)
	}
}

func TestJSONRPC_HTTPError(t *testing.T) {
	srv := newServer(t, http.StatusInternalServerError, "text/plain", "boom")

	_, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, nil)
	ee := asExit(t, err)
	if ee.Code != output.ExitAPI {
		t.Errorf("exit code = %d, want ExitAPI(%d)", ee.Code, output.ExitAPI)
	}
}

func TestJSONRPC_NonJSON(t *testing.T) {
	srv := newServer(t, 200, "application/json", "<html>not json</html>")

	_, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, nil)
	if asExit(t, err).Code != output.ExitAPI {
		t.Errorf("want ExitAPI for non-JSON body")
	}
}

func TestParseHeaders(t *testing.T) {
	h, err := parseHeaders([]string{"Authorization: Bearer abc", "X-Trace:  t1 "})
	if err != nil {
		t.Fatalf("parseHeaders: %v", err)
	}
	if got := h.Get("Authorization"); got != "Bearer abc" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer abc")
	}
	if got := h.Get("X-Trace"); got != "t1" {
		t.Errorf("X-Trace = %q (whitespace not trimmed)", got)
	}

	for _, bad := range []string{"no-colon", ": empty-key"} {
		if _, err := parseHeaders([]string{bad}); err == nil {
			t.Errorf("parseHeaders(%q) = nil error, want validation error", bad)
		} else if asExit(t, err).Code != output.ExitValidation {
			t.Errorf("parseHeaders(%q) wrong exit code", bad)
		}
	}
}

func TestParseHeaders_SentOnWire(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":"1","result":{}}`)
	}))
	t.Cleanup(srv.Close)

	h, _ := parseHeaders([]string{"Authorization: Bearer xyz"})
	if _, err := jsonRPC(context.Background(), srv.Client(), srv.URL, "tools/list", map[string]any{}, h); err != nil {
		t.Fatalf("jsonRPC: %v", err)
	}
	if gotAuth != "Bearer xyz" {
		t.Errorf("server saw Authorization = %q, want %q", gotAuth, "Bearer xyz")
	}
}
