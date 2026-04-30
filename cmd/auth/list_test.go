// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

// TestAuthListRun_NotConfigured_ReturnsExitZero pins the contract that
// `lark-cli auth list` is a read-only probe and must not fail-hard when no
// config exists yet — scripts and AI agents use it as an idempotent "do I
// have any users?" check, so the exit code carries semantic weight. Pair
// that with the existing "configured but no logged-in users" branch (also
// exit 0) and both empty states are consistent.
func TestAuthListRun_NotConfigured_ReturnsExitZero(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("auth list should succeed when not configured (exit 0); got: %v", err)
	}
	// Local workspace → hint must mention init, not bind.
	out := stderr.String()
	if !strings.Contains(out, "config init") {
		t.Errorf("local hint missing config init: %s", out)
	}
	if strings.Contains(out, "config bind") {
		t.Errorf("local hint must not mention config bind: %s", out)
	}
}

// TestAuthListRun_NotConfigured_AgentWorkspace_RoutesToBindHelp covers the
// reason this hint exists workspace-aware in the first place: an AI agent
// in OpenClaw / Hermes that probes auth list before binding gets routed to
// `config bind --help` instead of the local-only `config init`.
func TestAuthListRun_NotConfigured_AgentWorkspace_RoutesToBindHelp(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	prev := core.CurrentWorkspace()
	t.Cleanup(func() { core.SetCurrentWorkspace(prev) })
	core.SetCurrentWorkspace(core.WorkspaceOpenClaw)

	f, _, stderr, _ := cmdutil.TestFactory(t, nil)
	if err := authListRun(&ListOptions{Factory: f}); err != nil {
		t.Fatalf("auth list should still succeed under agent workspace; got: %v", err)
	}
	out := stderr.String()
	if !strings.Contains(out, "config bind --help") {
		t.Errorf("agent hint must point at config bind --help: %s", out)
	}
	if strings.Contains(out, "config init") {
		t.Errorf("agent hint must not mention config init: %s", out)
	}
}
