// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// saveAndRestoreWorkspace ensures package-level currentWorkspace is reset
// between subtests so cross-test pollution can't make assertions pass by
// accident.
func saveAndRestoreWorkspace(t *testing.T) {
	t.Helper()
	prev := CurrentWorkspace()
	t.Cleanup(func() { SetCurrentWorkspace(prev) })
}

func TestNotConfiguredError_Local(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)

	err := NotConfiguredError()
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Type != "config" || cfgErr.Message != "not configured" {
		t.Errorf("unexpected detail: %+v", cfgErr)
	}
	if !strings.Contains(cfgErr.Hint, "config init --new") {
		t.Errorf("local hint should suggest config init --new; got %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "config bind") {
		t.Errorf("local hint must not mention config bind; got %q", cfgErr.Hint)
	}
}

func TestNotConfiguredError_OpenClaw(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceOpenClaw)

	err := NotConfiguredError()
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Type != "openclaw" {
		t.Errorf("type = %q, want %q", cfgErr.Type, "openclaw")
	}
	// Hint must point at --help (read first, confirm with user, then bind),
	// NOT a directly-executable bind command — binding is policy-laden
	// (identity preset, may overwrite existing binding).
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("agent hint must point to `config bind --help`; got %q", cfgErr.Hint)
	}
	if strings.Contains(cfgErr.Hint, "config init") {
		t.Errorf("agent hint must NOT mention config init (would cause AI to create a new app); got %q", cfgErr.Hint)
	}
}

func TestNotConfiguredError_Hermes(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceHermes)

	err := NotConfiguredError()
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Type != "hermes" {
		t.Errorf("type = %q, want %q", cfgErr.Type, "hermes")
	}
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("hermes hint must point to `config bind --help`; got %q", cfgErr.Hint)
	}
}

func TestNoActiveProfileError_Local(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)

	err := NoActiveProfileError()
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Message != "no active profile" {
		t.Errorf("message = %q, want %q", cfgErr.Message, "no active profile")
	}
}

func TestNoActiveProfileError_AgentSuggestsBind(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceOpenClaw)

	err := NoActiveProfileError()
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("agent hint must point to `config bind --help`; got %q", cfgErr.Hint)
	}
}

func TestReconfigureHint_Local(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)

	got := reconfigureHint()
	if !strings.Contains(got, "config init") {
		t.Errorf("local reconfigure hint must mention config init; got %q", got)
	}
}

func TestReconfigureHint_Agent(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceHermes)

	got := reconfigureHint()
	if !strings.Contains(got, "config bind --help") {
		t.Errorf("agent reconfigure hint must point to `config bind --help`; got %q", got)
	}
}

func TestLoadOrNotConfigured_FileMissing_ReturnsNotConfigured(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	_, err := LoadOrNotConfigured()
	if err == nil {
		t.Fatal("expected error")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if cfgErr.Message != "not configured" {
		t.Errorf("message = %q, want \"not configured\"", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "config init --new") {
		t.Errorf("missing-file in local must hint `config init --new`; got %q", cfgErr.Hint)
	}
}

// TestLoadOrNotConfigured_CorruptFile_PreservesCause is the regression guard
// for the previous "every load error → not configured" coercion: a malformed
// config.json must surface its real failure cause so the user can fix it,
// not get sent in circles by an init/bind hint that wouldn't help here.
func TestLoadOrNotConfigured_CorruptFile_PreservesCause(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	// Write garbage that will fail JSON parsing.
	if err := os.WriteFile(dir+"/config.json", []byte("{not valid json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := LoadOrNotConfigured()
	if err == nil {
		t.Fatal("expected error for corrupt config")
	}
	var cfgErr *ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *ConfigError", err)
	}
	if !strings.Contains(cfgErr.Message, "failed to load config") {
		t.Errorf("corrupt-file message must say 'failed to load config'; got %q", cfgErr.Message)
	}
	// And it must NOT pretend the user just hasn't initialised yet.
	if cfgErr.Message == "not configured" {
		t.Errorf("corrupt-file must not be coerced to 'not configured'")
	}
	if strings.Contains(cfgErr.Hint, "config init") || strings.Contains(cfgErr.Hint, "config bind") {
		t.Errorf("corrupt-file hint must not redirect to init/bind; got %q", cfgErr.Hint)
	}
}
