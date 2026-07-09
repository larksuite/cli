// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestGuardAgentWorkspace_LocalAllows(t *testing.T) {
	clearAgentEnv(t)

	if err := guardAgentWorkspace(&ConfigInitOptions{}); err != nil {
		t.Errorf("local workspace should allow init, got: %v", err)
	}
}

func TestGuardAgentWorkspace_OpenClawRefuses(t *testing.T) {
	t.Setenv("OPENCLAW_HOME", t.TempDir())

	err := guardAgentWorkspace(&ConfigInitOptions{})
	if err == nil {
		t.Fatal("expected refusal in OpenClaw context, got nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *errs.ConfigError", err)
	}
	if cfgErr.Subtype != errs.SubtypeNotConfigured {
		t.Errorf("subtype = %q, want not_configured", cfgErr.Subtype)
	}
	if !strings.Contains(cfgErr.Message, "openclaw") {
		t.Errorf("message must name the openclaw workspace; got %q", cfgErr.Message)
	}
	if !strings.Contains(cfgErr.Hint, "config bind --help") {
		t.Errorf("hint must point to config bind --help; got %q", cfgErr.Hint)
	}
	if !strings.Contains(cfgErr.Hint, "--force-init") {
		t.Errorf("hint must mention --force-init escape hatch; got %q", cfgErr.Hint)
	}
}

func TestGuardAgentWorkspace_HermesRefuses(t *testing.T) {
	t.Setenv("HERMES_HOME", t.TempDir())

	err := guardAgentWorkspace(&ConfigInitOptions{})
	if err == nil {
		t.Fatal("expected refusal in Hermes context, got nil")
	}
	var cfgErr *errs.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("error type = %T, want *errs.ConfigError", err)
	}
	if cfgErr.Subtype != errs.SubtypeNotConfigured {
		t.Errorf("subtype = %q, want not_configured", cfgErr.Subtype)
	}
	if !strings.Contains(cfgErr.Message, "hermes") {
		t.Errorf("message must name the hermes workspace; got %q", cfgErr.Message)
	}
}

func TestGuardAgentWorkspace_ForceInitOverride(t *testing.T) {
	t.Setenv("OPENCLAW_HOME", t.TempDir())

	// --force-init must let the user proceed even inside an Agent context.
	if err := guardAgentWorkspace(&ConfigInitOptions{ForceInit: true}); err != nil {
		t.Errorf("--force-init should bypass the guard, got: %v", err)
	}
}
