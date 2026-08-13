// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package runtimebootstrap

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
)

func TestResolveEstablishesWorkspaceBeforeReadingProfile(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("OPENCLAW_CLI", "1")
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	result := Resolve("")
	if result == nil || result.Plan == nil {
		t.Fatal("Resolve() returned no runtime plan")
	}
	if got := core.CurrentWorkspace(); got != core.WorkspaceOpenClaw {
		t.Fatalf("workspace = %q, want %q", got, core.WorkspaceOpenClaw)
	}
}
