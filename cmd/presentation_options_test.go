// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import "testing"

// Preserve callers that store the original entrypoint as a function value.
// Making Execute variadic would compile at ordinary call sites but break this
// established source contract.
var _ func() int = Execute

func TestConcealRestrictedCommandsDefaults(t *testing.T) {
	cfg := &buildConfig{}
	ConcealRestrictedCommands()(cfg)

	if !cfg.presentation.enabled {
		t.Fatal("concealment must be explicitly enabled by the BuildOption")
	}
	if cfg.presentation.hidePolicyDiagnostics {
		t.Fatal("policy diagnostics must remain available by default")
	}
	if got := cfg.presentation.effectiveUnavailableMessage(); got != defaultRestrictedCommandUnavailableMessage {
		t.Errorf("message = %q, want %q", got, defaultRestrictedCommandUnavailableMessage)
	}
}

func TestConcealRestrictedCommandsOptions(t *testing.T) {
	cfg := &buildConfig{}
	ConcealRestrictedCommands(
		UnavailableMessage("not part of acme-cli"),
		HidePolicyDiagnostics(),
	)(cfg)

	if !cfg.presentation.enabled {
		t.Fatal("concealment must be enabled")
	}
	if !cfg.presentation.hidePolicyDiagnostics {
		t.Fatal("HidePolicyDiagnostics option was not applied")
	}
	if got := cfg.presentation.effectiveUnavailableMessage(); got != "not part of acme-cli" {
		t.Errorf("message = %q, want custom message", got)
	}
}

func TestConcealRestrictedCommandsIsBuildLocal(t *testing.T) {
	concealed := &buildConfig{}
	ordinary := &buildConfig{}

	ConcealRestrictedCommands(
		UnavailableMessage("acme only"),
		HidePolicyDiagnostics(),
	)(concealed)

	if ordinary.presentation.enabled {
		t.Fatal("applying an option to one build must not enable another")
	}
	if ordinary.presentation.hidePolicyDiagnostics {
		t.Fatal("applying an option to one build must not mutate another")
	}
	if got := ordinary.presentation.effectiveUnavailableMessage(); got != defaultRestrictedCommandUnavailableMessage {
		t.Errorf("ordinary message = %q, want default", got)
	}
}

func TestUnavailableMessageEmptyUsesDefault(t *testing.T) {
	cfg := &buildConfig{}
	ConcealRestrictedCommands(UnavailableMessage(""))(cfg)

	if got := cfg.presentation.effectiveUnavailableMessage(); got != defaultRestrictedCommandUnavailableMessage {
		t.Errorf("message = %q, want %q", got, defaultRestrictedCommandUnavailableMessage)
	}
}
