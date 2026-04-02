// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestJqFilter(t *testing.T) {
	data := map[string]interface{}{
		"ok":       true,
		"identity": "user",
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"name": "Alice", "age": 30},
				map[string]interface{}{"name": "Bob", "age": 25},
				map[string]interface{}{"name": "Charlie", "age": 35},
			},
			"total": 3,
		},
		"meta": map[string]interface{}{
			"count": 3,
		},
	}

	tests := []struct {
		name    string
		expr    string
		want    string
		wantErr bool
	}{
		{
			name: "identity expression",
			expr: ".",
			want: `"ok"`,
		},
		{
			name: "field access .ok",
			expr: ".ok",
			want: "true\n",
		},
		{
			name: "string field raw output",
			expr: ".identity",
			want: "user\n",
		},
		{
			name: "nested field access",
			expr: ".data.total",
			want: "3\n",
		},
		{
			name: "meta count",
			expr: ".meta.count",
			want: "3\n",
		},
		{
			name: "array iteration",
			expr: ".data.items[].name",
			want: "Alice\nBob\nCharlie\n",
		},
		{
			name: "pipe and select",
			expr: `.data.items[] | select(.age > 28) | .name`,
			want: "Alice\nCharlie\n",
		},
		{
			name: "length builtin",
			expr: ".data.items | length",
			want: "3\n",
		},
		{
			name: "keys builtin",
			expr: ".data | keys",
			want: "[\n  \"items\",\n  \"total\"\n]\n",
		},
		{
			name: "null for missing field",
			expr: ".nonexistent",
			want: "null\n",
		},
		{
			name: "complex value output",
			expr: ".data.items[0]",
			want: "{\n  \"age\": 30,\n  \"name\": \"Alice\"\n}\n",
		},
		{
			name:    "invalid expression",
			expr:    "invalid[",
			wantErr: true,
		},
		{
			name: "multiple outputs",
			expr: ".ok, .identity",
			want: "true\nuser\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := JqFilter(&buf, data, tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.name == "identity expression" {
				// For identity, just verify it contains the key fields
				if !strings.Contains(buf.String(), `"ok"`) {
					t.Errorf("identity output missing 'ok' key")
				}
				return
			}
			if buf.String() != tt.want {
				t.Errorf("got %q, want %q", buf.String(), tt.want)
			}
		})
	}
}

func TestJqFilter_WithStruct(t *testing.T) {
	// Test that toGeneric normalizes structs properly
	type inner struct {
		Name string `json:"name"`
	}
	data := struct {
		OK   bool   `json:"ok"`
		Item *inner `json:"item"`
	}{
		OK:   true,
		Item: &inner{Name: "test"},
	}

	var buf bytes.Buffer
	err := JqFilter(&buf, data, ".item.name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "test" {
		t.Errorf("got %q, want %q", got, "test")
	}
}

func TestValidateJqFlags(t *testing.T) {
	tests := []struct {
		name       string
		jqExpr     string
		outputFlag string
		format     string
		wantErr    string
	}{
		{name: "empty jq is noop", jqExpr: "", outputFlag: "file.json", format: "csv", wantErr: ""},
		{name: "jq only", jqExpr: ".data", outputFlag: "", format: "", wantErr: ""},
		{name: "jq with json format", jqExpr: ".data", outputFlag: "", format: "json", wantErr: ""},
		{name: "jq and output conflict", jqExpr: ".data", outputFlag: "out.json", format: "", wantErr: "--jq and --output are mutually exclusive"},
		{name: "jq and csv conflict", jqExpr: ".data", outputFlag: "", format: "csv", wantErr: "--jq and --format csv are mutually exclusive"},
		{name: "jq and ndjson conflict", jqExpr: ".data", outputFlag: "", format: "ndjson", wantErr: "--jq and --format ndjson are mutually exclusive"},
		{name: "invalid expression", jqExpr: "invalid[", outputFlag: "", format: "", wantErr: "invalid jq expression"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateJqFlags(tt.jqExpr, tt.outputFlag, tt.format)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidateJqExpression(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{".", false},
		{".data", false},
		{".data.items[].name", false},
		{`.data.items[] | select(.name == "Alice")`, false},
		{"length", false},
		{"keys", false},
		{"invalid[", true},
		{".foo | invalid_func", true},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			err := ValidateJqExpression(tt.expr)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
