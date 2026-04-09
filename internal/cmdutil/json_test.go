// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestParseOptionalBody(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		data    string
		wantNil bool
		wantErr bool
	}{
		{"GET ignored", "GET", `{"a":1}`, true, false},
		{"POST empty data", "POST", "", true, false},
		{"POST valid", "POST", `{"key":"val"}`, false, false},
		{"PUT valid", "PUT", `[1,2,3]`, false, false},
		{"PATCH valid", "PATCH", `"hello"`, false, false},
		{"DELETE valid", "DELETE", `{"id":"1"}`, false, false},
		{"POST invalid json", "POST", `{bad}`, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOptionalBody(tt.method, tt.data, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseOptionalBody() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && got != nil {
				t.Errorf("ParseOptionalBody() = %v, want nil", got)
			}
			if !tt.wantNil && !tt.wantErr && got == nil {
				t.Error("ParseOptionalBody() = nil, want non-nil")
			}
		})
	}
}

func TestParseJSONMap(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		label   string
		wantLen int
		wantErr bool
	}{
		{"empty input", "", "--params", 0, false},
		{"valid json", `{"a":"1","b":"2"}`, "--params", 2, false},
		{"invalid json", `{bad}`, "--params", 0, true},
		{"json array", `[1,2]`, "--data", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONMap(tt.input, tt.label, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseJSONMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("ParseJSONMap() returned map with %d keys, want %d", len(got), tt.wantLen)
			}
		})
	}
}

func TestParseJSONMap_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	TestChdir(t, tmpDir)
	if err := vfs.WriteFile("params.json", []byte(`{"a":"1","b":"2"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ParseJSONMap("@params.json", "--params", nil)
	if err != nil {
		t.Fatalf("ParseJSONMap(@file) error = %v", err)
	}
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Fatalf("ParseJSONMap(@file) = %#v, want parsed map", got)
	}
}

func TestParseJSONMap_FromStdin(t *testing.T) {
	got, err := ParseJSONMap("-", "--params", strings.NewReader(`{"a":"1"}`))
	if err != nil {
		t.Fatalf("ParseJSONMap(-) error = %v", err)
	}
	if got["a"] != "1" {
		t.Fatalf("ParseJSONMap(-) = %#v, want a=1", got)
	}
}

func TestParseOptionalBody_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	TestChdir(t, tmpDir)
	if err := vfs.WriteFile("data.json", []byte(`{"id":"1"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ParseOptionalBody("POST", "@data.json", nil)
	if err != nil {
		t.Fatalf("ParseOptionalBody(@file) error = %v", err)
	}
	body, ok := got.(map[string]any)
	if !ok || body["id"] != "1" {
		t.Fatalf("ParseOptionalBody(@file) = %#v, want parsed object", got)
	}
}

func TestResolveStructuredInput_EmptyFilePath(t *testing.T) {
	_, err := ResolveStructuredInput("@", "--params", nil)
	if err == nil {
		t.Fatal("expected error for empty @file path")
	}
}
