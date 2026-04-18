// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/envvars"
)

// --- EmitShortcut tests ---

// TestEmitShortcut_JSON_Clean: MODE=warn, cleanProvider, no alert in output.
func TestEmitShortcut_JSON_Clean(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &cleanProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stdout.String(), `"ok": true`) {
		t.Errorf("expected stdout to contain %q, got: %s", `"ok": true`, stdout.String())
	}
	if strings.Contains(stdout.String(), "_content_safety_alert") {
		t.Errorf("expected stdout NOT to contain _content_safety_alert, got: %s", stdout.String())
	}
}

// TestEmitShortcut_JSON_WarnHit: MODE=warn, hitProvider, alert in output.
func TestEmitShortcut_JSON_WarnHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stdout.String(), "_content_safety_alert") {
		t.Errorf("expected stdout to contain _content_safety_alert, got: %s", stdout.String())
	}
}

// TestEmitShortcut_JSON_BlockHit: MODE=block, hitProvider, stdout empty, err is *ExitError.
func TestEmitShortcut_JSON_BlockHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "block")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail.Type != "content_safety_blocked" {
		t.Errorf("expected Detail.Type=%q, got %q", "content_safety_blocked", exitErr.Detail.Type)
	}
}

// TestEmitShortcut_Pretty_WithFn: MODE=warn, hitProvider, Format="pretty", PrettyFn writes custom text.
func TestEmitShortcut_Pretty_WithFn(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Format:      "pretty",
		PrettyFn: func(w io.Writer) {
			w.Write([]byte("PRETTY OUTPUT\n"))
		},
		Out:    &stdout,
		ErrOut: &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stdout.String(), "PRETTY OUTPUT") {
		t.Errorf("expected stdout to contain %q, got: %s", "PRETTY OUTPUT", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert") {
		t.Errorf("expected stderr to contain %q, got: %s", "warning: content safety alert", stderr.String())
	}
}

// TestEmitShortcut_Pretty_NilFn: MODE=off, cleanProvider, Format="pretty", PrettyFn=nil -> JSON fallback.
func TestEmitShortcut_Pretty_NilFn(t *testing.T) {
	// MODE not set → modeOff
	t.Setenv(envvars.CliContentSafetyMode, "off")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &cleanProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Format:      "pretty",
		PrettyFn:    nil,
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	// Should fall back to JSON envelope
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Errorf("expected stdout to contain JSON envelope, got: %s", stdout.String())
	}
}

// TestEmitShortcut_Table_WarnHit: MODE=warn, hitProvider, Format="table".
func TestEmitShortcut_Table_WarnHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"items": []any{map[string]any{"id": "1"}}},
		Format:      "table",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert") {
		t.Errorf("expected stderr to contain %q, got: %s", "warning: content safety alert", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected non-empty stdout from FormatValue")
	}
}

// TestEmitShortcut_UnknownFormat: cleanProvider, Format="garbage" -> warning + JSON fallback.
func TestEmitShortcut_UnknownFormat(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "off")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &cleanProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Format:      "garbage",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "unknown format") {
		t.Errorf("expected stderr to contain %q, got: %s", "unknown format", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"ok"`) {
		t.Errorf("expected stdout to contain JSON envelope fallback, got: %s", stdout.String())
	}
}

// TestEmitShortcut_ModeOff: MODE not set (default off), no alert even with hitProvider.
func TestEmitShortcut_ModeOff(t *testing.T) {
	// Do not set MODE — defaults to off
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	var stdout, stderr bytes.Buffer
	err := EmitShortcut(ShortcutEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        map[string]any{"text": "hello"},
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if strings.Contains(stdout.String(), "_content_safety_alert") {
		t.Errorf("expected no alert in output when mode is off, got: %s", stdout.String())
	}
}

// --- EmitLarkResponse tests ---

// TestEmitLarkResponse_JSON_WarnInject: MODE=warn, hitProvider, lark-shaped map -> alert injected into map.
func TestEmitLarkResponse_JSON_WarnInject(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	data := map[string]any{
		"code": json.Number("0"),
		"msg":  "success",
		"data": map[string]any{"x": "y"},
	}

	var stdout, stderr bytes.Buffer
	err := EmitLarkResponse(LarkResponseEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        data,
		Format:      "json",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := data["_content_safety_alert"]; !ok {
		t.Error("expected _content_safety_alert to be injected into data map")
	}
}

// TestEmitLarkResponse_JSON_NonLarkMap: MODE=warn, hitProvider, map without "code" -> no injection, warning to stderr.
func TestEmitLarkResponse_JSON_NonLarkMap(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	data := map[string]any{"items": []any{}}

	var stdout, stderr bytes.Buffer
	err := EmitLarkResponse(LarkResponseEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        data,
		Format:      "json",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := data["_content_safety_alert"]; ok {
		t.Error("expected _content_safety_alert NOT to be injected for non-lark map")
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert") {
		t.Errorf("expected stderr to contain warning, got: %s", stderr.String())
	}
}

// TestEmitLarkResponse_JSON_WithJq: MODE=warn, hitProvider, jq active -> no injection, warning to stderr.
func TestEmitLarkResponse_JSON_WithJq(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	data := map[string]any{
		"code": json.Number("0"),
		"msg":  "success",
		"data": map[string]any{"x": "y"},
	}

	var stdout, stderr bytes.Buffer
	err := EmitLarkResponse(LarkResponseEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        data,
		Format:      "json",
		JqExpr:      ".code",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, ok := data["_content_safety_alert"]; ok {
		t.Error("expected _content_safety_alert NOT to be injected when jq is active")
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert") {
		t.Errorf("expected stderr to contain warning, got: %s", stderr.String())
	}
}

// TestEmitLarkResponse_Table_WarnHit: MODE=warn, hitProvider, Format="table" -> warning on stderr, content on stdout.
func TestEmitLarkResponse_Table_WarnHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "warn")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	data := map[string]any{
		"code": json.Number("0"),
		"msg":  "success",
		"data": map[string]any{"items": []any{map[string]any{"id": "1"}}},
	}

	var stdout, stderr bytes.Buffer
	err := EmitLarkResponse(LarkResponseEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        data,
		Format:      "table",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: content safety alert") {
		t.Errorf("expected stderr to contain warning, got: %s", stderr.String())
	}
	if stdout.Len() == 0 {
		t.Error("expected non-empty stdout from FormatValue")
	}
}

// TestEmitLarkResponse_BlockHit: MODE=block, hitProvider -> stdout empty, err is *ExitError.
func TestEmitLarkResponse_BlockHit(t *testing.T) {
	t.Setenv(envvars.CliContentSafetyMode, "block")
	t.Setenv(envvars.CliContentSafetyAllowlist, "all")
	withProvider(t, &hitProvider{})

	data := map[string]any{
		"code": json.Number("0"),
		"msg":  "success",
	}

	var stdout, stderr bytes.Buffer
	err := EmitLarkResponse(LarkResponseEmitRequest{
		CommandPath: "lark-cli im +messages-search",
		Data:        data,
		Format:      "json",
		Out:         &stdout,
		ErrOut:      &stderr,
	})

	if stdout.Len() != 0 {
		t.Errorf("expected empty stdout, got: %s", stdout.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Detail.Type != "content_safety_blocked" {
		t.Errorf("expected Detail.Type=%q, got %q", "content_safety_blocked", exitErr.Detail.Type)
	}
}
