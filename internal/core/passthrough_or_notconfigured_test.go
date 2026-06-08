// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// R2 *ConfigError must pass through verbatim so the upgrade hint reaches
// the operator unchanged — wrapping it here would route AI agents to
// `config init`, overwriting fields the newer-binary file populated.
func TestPassThroughOrNotConfigured_R2ConfigErrorPassesThrough(t *testing.T) {
	r2 := &ConfigError{
		Code:    3,
		Type:    "config",
		Message: "config.json was written by a newer lark-cli (schemaVersion 99 > supported 1)",
		Hint:    "upgrade lark-cli, or use a different --profile to avoid overwriting fields the newer binary populated",
	}
	got := PassThroughOrNotConfigured(r2)
	if got != r2 {
		t.Fatalf("R2 ConfigError must pass through identical pointer; got %#v", got)
	}
	var cfgErr *ConfigError
	if !errors.As(got, &cfgErr) || cfgErr.Hint != r2.Hint {
		t.Errorf("R2 hint lost or rewrapped: %#v", got)
	}
}

// Wrapped *ConfigError (errors.As-discoverable) must also pass through —
// callers may have wrapped via fmt.Errorf("...: %w", cfgErr).
func TestPassThroughOrNotConfigured_WrappedConfigErrorPassesThrough(t *testing.T) {
	r2 := &ConfigError{Code: 3, Type: "config", Message: "newer schema", Hint: "upgrade"}
	wrapped := fmt.Errorf("load: %w", r2)
	got := PassThroughOrNotConfigured(wrapped)
	var cfgErr *ConfigError
	if !errors.As(got, &cfgErr) {
		t.Fatalf("wrapped ConfigError lost: %T", got)
	}
	if cfgErr.Hint != "upgrade" {
		t.Errorf("hint lost through unwrap: %q", cfgErr.Hint)
	}
}

func TestPassThroughOrNotConfigured_FileMissing_LocalReturnsNotConfigured(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceLocal)

	got := PassThroughOrNotConfigured(os.ErrNotExist)
	var cfgErr *ConfigError
	if !errors.As(got, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", got)
	}
	if cfgErr.Message != "not configured" {
		t.Errorf("message = %q, want \"not configured\"", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "config init --new") {
		t.Errorf("local missing-file hint should mention config init --new; got %q", cfgErr.Hint)
	}
}

func TestPassThroughOrNotConfigured_FileMissing_AgentHintsBind(t *testing.T) {
	saveAndRestoreWorkspace(t)
	SetCurrentWorkspace(WorkspaceOpenClaw)

	got := PassThroughOrNotConfigured(os.ErrNotExist)
	var cfgErr *ConfigError
	if !errors.As(got, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", got)
	}
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("agent missing-file hint must point to config bind --help; got %q", cfgErr.Hint)
	}
}

// A non-ConfigError, non-NotExist error (parse failure, permission denied)
// must surface its real cause so the operator can fix the broken file —
// NOT be coerced to "not configured", which sends users in circles.
func TestPassThroughOrNotConfigured_ParseError_WrapsAsConfigErrorWithCause(t *testing.T) {
	got := PassThroughOrNotConfigured(fmt.Errorf("invalid config format: unexpected EOF"))
	var cfgErr *ConfigError
	if !errors.As(got, &cfgErr) {
		t.Fatalf("expected *ConfigError, got %T", got)
	}
	if !strings.Contains(cfgErr.Message, "failed to load config") {
		t.Errorf("parse-error message must say 'failed to load config'; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Message, "unexpected EOF") {
		t.Errorf("parse-error message must surface the original cause; got %q", cfgErr.Message)
	}
	if cfgErr.Message == "not configured" {
		t.Errorf("parse error must not be coerced to 'not configured'")
	}
}

func TestPassThroughOrNotConfigured_Nil_ReturnsNil(t *testing.T) {
	if got := PassThroughOrNotConfigured(nil); got != nil {
		t.Errorf("nil input should pass through nil, got %v", got)
	}
}
