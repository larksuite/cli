// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
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

func TestGuardAgentWorkspace_BindRecoveryUsesBuildLocalSurface(t *testing.T) {
	t.Setenv("OPENCLAW_HOME", t.TempDir())

	source := guardAgentWorkspace(&ConfigInitOptions{})
	var original *errs.ConfigError
	if !errors.As(source, &original) {
		t.Fatalf("guardAgentWorkspace() error = %T, want *errs.ConfigError", source)
	}
	const visibleHint = "see `lark-cli config bind --help` to bind lark-cli to the Agent's existing app instead. Pass --force-init only if the user explicitly wants a separate app in this workspace."
	if original.Hint != visibleHint {
		t.Fatalf("producer hint = %q, want %q", original.Hint, visibleHint)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandConfigBind: surface.CommandConcealed,
	})
	var concealed *errs.ConfigError
	if rendered := recovery.Render(source, plan); !errors.As(rendered, &concealed) {
		t.Fatalf("rendered error = %T, want *errs.ConfigError", rendered)
	}
	const forceInitHint = "Pass --force-init only if the user explicitly wants a separate app in this workspace."
	if concealed.Hint != forceInitHint {
		t.Errorf("concealed hint = %q, want %q", concealed.Hint, forceInitHint)
	}
	if original.Hint != visibleHint {
		t.Errorf("concealed render mutated producer hint: %q", original.Hint)
	}
}

func TestProjectInitHelpPreservesDefaultAndProjectsConcealedBind(t *testing.T) {
	cmd := NewCmdConfigInit(nil, nil)
	forceInit := cmd.Flags().Lookup("force-init")
	if forceInit == nil {
		t.Fatal("config init command has no --force-init flag")
	}
	const defaultLong = `Initialize configuration (app-id / app-secret-stdin / brand).

For AI agents: use --new to create a new app. The command blocks until the user
completes setup in the browser. Run it in the background and retrieve the
verification URL from its output.

Inside an Agent context (OPENCLAW_HOME / HERMES_HOME set) this command
refuses by default — use 'lark-cli config bind' to bind to the Agent's
existing app instead of creating a parallel one. Pass --force-init only
if the user explicitly wants a separate app inside the Agent workspace.`
	const defaultForceInitUsage = "allow init inside an Agent workspace (OPENCLAW_HOME / HERMES_HOME); use config bind instead unless you really want a separate app"
	if cmd.Long != defaultLong || forceInit.Usage != defaultForceInitUsage {
		t.Fatalf("default help lost config bind recovery:\nLong:\n%s\n--force-init: %s", cmd.Long, forceInit.Usage)
	}

	ProjectInitHelp(cmd, false)
	const concealedLong = `Initialize configuration (app-id / app-secret-stdin / brand).

For AI agents: use --new to create a new app. The command blocks until the user
completes setup in the browser. Run it in the background and retrieve the
verification URL from its output.

Inside an Agent context (OPENCLAW_HOME / HERMES_HOME set) this command
refuses by default to avoid creating a parallel app alongside Agent-managed
credentials. Reuse the Agent's existing app through this distribution's
supported setup flow. Pass --force-init only if the user explicitly wants a
separate app inside the Agent workspace.`
	const concealedForceInitUsage = "allow init inside an Agent workspace (OPENCLAW_HOME / HERMES_HOME) only when the user explicitly wants a separate app"
	if cmd.Long != concealedLong || forceInit.Usage != concealedForceInitUsage {
		t.Fatalf("concealed help was not projected:\nLong:\n%s\n--force-init: %s", cmd.Long, forceInit.Usage)
	}
	if strings.Contains(cmd.Long, "config bind") || strings.Contains(forceInit.Usage, "config bind") {
		t.Fatalf("concealed help retained config bind:\nLong:\n%s\n--force-init: %s", cmd.Long, forceInit.Usage)
	}

	ProjectInitHelp(cmd, true)
	if cmd.Long != defaultLong || forceInit.Usage != defaultForceInitUsage {
		t.Fatalf("visible projection did not restore default help:\nLong:\n%s\n--force-init: %s", cmd.Long, forceInit.Usage)
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
