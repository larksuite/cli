// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/output"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDoMCPCallUnauthorizedHTTPError(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		}),
	}

	_, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "uat-token", "https://example.com/mcp", false)
	if got := output.ExitCodeOf(err); got != output.ExitAuth {
		t.Fatalf("expected auth exit code (%d), got %d", output.ExitAuth, got)
	}
}

func TestDoMCPCallJSONRPCErrorUsesLarkClassification(t *testing.T) {
	t.Parallel()

	client := &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"error":{"code":99991668,"message":"user_access_token invalid"}}`)),
			}, nil
		}),
	}

	_, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "uat-token", "https://example.com/mcp", false)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.Code != output.ExitAuth {
		t.Fatalf("expected auth exit code, got %d", exitErr.Code)
	}
	if exitErr.Detail == nil || exitErr.Detail.Type != "auth" {
		t.Fatalf("expected auth detail, got %#v", exitErr.Detail)
	}
}

func TestDoMCPCallSetsHeadersAndUnwrapsResult(t *testing.T) {
	t.Parallel()

	var seen *http.Request
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seen = req
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(`{"result":{"jsonrpc":"2.0","result":{"ok":true}}}`)),
			}, nil
		}),
	}

	got, err := DoMCPCall(context.Background(), client, "fetch-doc", map[string]interface{}{"doc_id": "doc_1"}, "tat-token", "https://example.com/mcp", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := got.(map[string]interface{})
	if !ok || result["ok"] != true {
		t.Fatalf("unexpected result: %#v", got)
	}
	if seen == nil {
		t.Fatalf("expected request to be captured")
	}
	if seen.Header.Get("X-Lark-MCP-TAT") != "tat-token" {
		t.Fatalf("expected bot token header, got %q", seen.Header.Get("X-Lark-MCP-TAT"))
	}
	if seen.Header.Get("X-Lark-MCP-Allowed-Tools") != "fetch-doc" {
		t.Fatalf("expected allowed tools header, got %q", seen.Header.Get("X-Lark-MCP-Allowed-Tools"))
	}
}

func TestNormalizeMCPToolResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		raw     interface{}
		wantKey string
		wantVal interface{}
		wantErr string
	}{
		{
			name:    "map result",
			raw:     map[string]interface{}{"ok": true},
			wantKey: "ok",
			wantVal: true,
		},
		{
			name:    "text result",
			raw:     "plain text",
			wantKey: "message",
			wantVal: "plain text",
		},
		{
			name:    "scalar result",
			raw:     42,
			wantKey: "result",
			wantVal: 42,
		},
		{
			name:    "map error field",
			raw:     map[string]interface{}{"error": "permission denied"},
			wantErr: "MCP: permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeMCPToolResult(tt.raw)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got[tt.wantKey] != tt.wantVal {
				t.Fatalf("unexpected result: %#v", got)
			}
		})
	}
}

func TestExtractMCPResult(t *testing.T) {
	t.Parallel()

	jsonResult := ExtractMCPResult(map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{
				"type": "text",
				"text": `{"doc_id":"doc_1"}`,
			},
		},
	})
	resultMap, ok := jsonResult.(map[string]interface{})
	if !ok || resultMap["doc_id"] != "doc_1" {
		t.Fatalf("unexpected parsed json result: %#v", jsonResult)
	}

	textResult := ExtractMCPResult(map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "line1"},
			map[string]interface{}{"type": "text", "text": "line2"},
		},
	})
	if textResult != "line1\nline2" {
		t.Fatalf("unexpected text result: %#v", textResult)
	}
}
