// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

func TestResolveInput_Stdin(t *testing.T) {
	got, err := ResolveInput("-", strings.NewReader(`{"key":"value"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"key":"value"}` {
		t.Errorf("got %q, want %q", got, `{"key":"value"}`)
	}
}

func TestResolveInput_Stdin_TrimNewline(t *testing.T) {
	got, err := ResolveInput("-", strings.NewReader("{\"k\":\"v\"}\n"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"k":"v"}` {
		t.Errorf("got %q, want %q", got, `{"k":"v"}`)
	}
}

func TestResolveInput_Stdin_Empty(t *testing.T) {
	_, err := ResolveInput("-", strings.NewReader(""), nil)
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
	_, err := ResolveInput("-", errorReader{}, nil)
	if err == nil || !strings.Contains(err.Error(), "failed to read stdin") {
		t.Errorf("expected read error, got: %v", err)
	}
}

func TestResolveInput_Stdin_WhitespaceOnly(t *testing.T) {
	_, err := ResolveInput("-", strings.NewReader("  \n\t\n  "), nil)
	if err == nil {
		t.Error("expected error for whitespace-only stdin")
	}
}

func TestResolveInput_Stdin_Nil(t *testing.T) {
	_, err := ResolveInput("-", nil, nil)
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
			got, err := ResolveInput(tt.in, nil, nil)
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
	got, err := ResolveInput("", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestResolveInput_PlainValue(t *testing.T) {
	got, err := ResolveInput(`{"already":"valid"}`, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"already":"valid"}` {
		t.Errorf("got %q, want %q", got, `{"already":"valid"}`)
	}
}

func TestResolveInput_AtFile(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	if err := os.WriteFile("params.json", []byte(`{"folder_token":"abc123"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveInput("@params.json", nil, fio)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"folder_token":"abc123"}` {
		t.Errorf("got %q", got)
	}
}

func TestResolveInput_AtFile_TrimsWhitespace(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	if err := os.WriteFile("p.json", []byte("\n  {\"k\":\"v\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveInput("@p.json", nil, fio)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != `{"k":"v"}` {
		t.Errorf("got %q", got)
	}
}

func TestResolveInput_AtFile_NotFound(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	_, err := ResolveInput("@missing.json", nil, fio)
	if err == nil || !strings.Contains(err.Error(), "cannot read file") {
		t.Errorf("expected read error, got: %v", err)
	}
}

func TestResolveInput_AtFile_PathValidation(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	// Absolute paths are rejected by SafeInputPath; the error must surface
	// as an invalid-path message, not a generic read failure.
	_, err := ResolveInput("@/etc/passwd", nil, fio)
	if err == nil || !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("expected path-validation error, got: %v", err)
	}
}

func TestResolveInput_AtFile_EmptyPath(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	_, err := ResolveInput("@", nil, fio)
	if err == nil || !strings.Contains(err.Error(), "file path cannot be empty after @") {
		t.Errorf("expected empty-path error, got: %v", err)
	}
}

func TestResolveInput_AtFile_EmptyContent(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	if err := os.WriteFile("empty.json", []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveInput("@empty.json", nil, fio)
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("expected empty-file error, got: %v", err)
	}
}

func TestResolveInput_AtFile_NoFileIO(t *testing.T) {
	// When fileIO is nil, @path must error rather than silently fall back.
	_, err := ResolveInput("@params.json", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Errorf("expected unavailable error, got: %v", err)
	}
}

func TestResolveInput_DoubleAtEscape(t *testing.T) {
	got, err := ResolveInput("@@literal", nil, nil)
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
	got, err := ParseJSONMap("-", "--params", stdin, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d keys, want 2", len(got))
	}
}

// Integration: @file flows through ParseJSONMap correctly.
func TestParseJSONMap_WithAtFile(t *testing.T) {
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	if err := os.WriteFile("params.json", []byte(`{"folder_token":"abc123","type":"folder"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseJSONMap("@params.json", "--params", nil, fio)
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
	fio := &localfileio.LocalFileIO{}
	dir := t.TempDir()
	TestChdir(t, dir)
	if err := os.WriteFile("data.json", []byte(`{"text":"hello"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ParseOptionalBody("POST", "@data.json", nil, fio)
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
	got, err := ParseJSONMap(`'{"key":"value"}'`, "--params", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["key"] != "value" {
		t.Errorf("got %v, want key=value", got)
	}
}

func TestParseOptionalBody_WithStdin(t *testing.T) {
	stdin := strings.NewReader(`{"text":"hello"}`)
	got, err := ParseOptionalBody("POST", "-", stdin, nil)
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
			got, err := ParseJSONMap(tt.input, "--params", nil, nil)
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
