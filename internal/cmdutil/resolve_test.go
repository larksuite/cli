// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInput_Stdin(t *testing.T) {
	got, err := ResolveInput("-", strings.NewReader(`{"key":"value"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"key":"value"}` {
		t.Errorf("got %q, want %q", got, `{"key":"value"}`)
	}
}

func TestResolveInput_Stdin_TrimNewline(t *testing.T) {
	got, err := ResolveInput("-", strings.NewReader("{\"k\":\"v\"}\n"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"k":"v"}` {
		t.Errorf("got %q, want %q", got, `{"k":"v"}`)
	}
}

func TestResolveInput_Stdin_Empty(t *testing.T) {
	_, err := ResolveInput("-", strings.NewReader(""))
	if err == nil {
		t.Error("expected error for empty stdin")
	}
	if !strings.Contains(err.Error(), "stdin is empty") {
		t.Errorf("expected 'stdin is empty' error, got: %v", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, fmt.Errorf("disk failure") }

func TestResolveInput_Stdin_ReadError(t *testing.T) {
	_, err := ResolveInput("-", errorReader{})
	if err == nil || !strings.Contains(err.Error(), "failed to read stdin") {
		t.Errorf("expected read error, got: %v", err)
	}
}

func TestResolveInput_Stdin_WhitespaceOnly(t *testing.T) {
	_, err := ResolveInput("-", strings.NewReader("  \n\t\n  "))
	if err == nil {
		t.Error("expected error for whitespace-only stdin")
	}
}

func TestResolveInput_Stdin_Nil(t *testing.T) {
	_, err := ResolveInput("-", nil)
	if err == nil {
		t.Error("expected error for nil stdin")
	}
}

func TestResolveInput_StripSingleQuotes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"cmd.exe JSON", `'{"key":"value"}'`, `{"key":"value"}`},
		{"cmd.exe empty", `'{}'`, `{}`},
		{"no quotes", `{"key":"value"}`, `{"key":"value"}`},
		{"just quotes", `''`, ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveInput(tt.in, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveInput_Empty(t *testing.T) {
	got, err := ResolveInput("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveInput_PlainValue(t *testing.T) {
	got, err := ResolveInput(`{"already":"valid"}`, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"already":"valid"}` {
		t.Errorf("got %q, want %q", got, `{"already":"valid"}`)
	}
}

func TestResolveInput_AtFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")
	if err := os.WriteFile(path, []byte(`{"folder_token":"abc123"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveInput("@"+path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"folder_token":"abc123"}` {
		t.Errorf("got %q", got)
	}
}

func TestResolveInput_AtFile_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	if err := os.WriteFile(path, []byte("\n  {\"k\":\"v\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveInput("@"+path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"k":"v"}` {
		t.Errorf("got %q", got)
	}
}

func TestResolveInput_AtFile_NotFound(t *testing.T) {
	_, err := ResolveInput("@/no/such/file.json", nil)
	if err == nil || !strings.Contains(err.Error(), "cannot read file") {
		t.Errorf("expected read error, got: %v", err)
	}
}

func TestResolveInput_AtFile_EmptyPath(t *testing.T) {
	_, err := ResolveInput("@", nil)
	if err == nil || !strings.Contains(err.Error(), "file path cannot be empty") {
		t.Errorf("expected empty-path error, got: %v", err)
	}
}

func TestResolveInput_AtFile_EmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveInput("@"+path, nil)
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("expected empty-file error, got: %v", err)
	}
}

func TestResolveInput_DoubleAtEscape(t *testing.T) {
	got, err := ResolveInput("@@literal", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "@literal" {
		t.Errorf("got %q, want %q", got, "@literal")
	}
}

// Integration: ResolveInput flows through ParseJSONMap correctly.
func TestParseJSONMap_WithStdin(t *testing.T) {
	stdin := strings.NewReader(`{"message_id":"om_xxx","user_id_type":"open_id"}`)
	got, err := ParseJSONMap("-", "--params", stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d keys, want 2", len(got))
	}
}

// Integration: @file flows through ParseJSONMap correctly.
func TestParseJSONMap_WithAtFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "params.json")
	if err := os.WriteFile(path, []byte(`{"folder_token":"abc123","type":"folder"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseJSONMap("@"+path, "--params", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d keys, want 2", len(got))
	}
	if got["folder_token"] != "abc123" {
		t.Errorf("got %v, want folder_token=abc123", got)
	}
}

func TestParseOptionalBody_WithAtFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"text":"hello"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseOptionalBody("POST", "@"+path, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["text"] != "hello" {
		t.Errorf("got %v, want text=hello", m)
	}
}

func TestParseJSONMap_StripSingleQuotes_CmdExe(t *testing.T) {
	got, err := ParseJSONMap(`'{"key":"value"}'`, "--params", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("got %v, want key=value", got)
	}
}

func TestParseOptionalBody_WithStdin(t *testing.T) {
	stdin := strings.NewReader(`{"text":"hello"}`)
	got, err := ParseOptionalBody("POST", "-", stdin)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil body")
	}
	m, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["text"] != "hello" {
		t.Errorf("got %v, want text=hello", m)
	}
}

// Simulates exact strings Go receives on different Windows shells.
func TestParseJSONMap_WindowsShellScenarios(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantLen int
		wantErr bool
	}{
		{"bash: normal JSON", `{"a":"1","b":"2"}`, 2, false},
		{"cmd.exe: single-quoted", `'{"a":"1","b":"2"}'`, 2, false}, // strip ' fix
		{"PS 5.x: mangled", `{a:1,b:2}`, 0, true},                   // unrecoverable
		{"PS 5.x: empty JSON OK", `{}`, 0, false},                   // no inner "
		{"PS 7.3+: normal JSON", `{"a":"1"}`, 1, false},             // already fixed
		{"PS escaped: correct", `{"a":"1"}`, 1, false},              // after CommandLineToArgvW
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSONMap(tt.input, "--params", nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(got) != tt.wantLen {
				t.Errorf("got %d keys, want %d", len(got), tt.wantLen)
			}
		})
	}
}
