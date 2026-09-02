// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
)

// NotConfiguredError carries a semantic config/init recovery target. Rendering
// for one concealed tree filters only a clone and leaves the producer value
// reusable by a tree where config/init remains referenceable.
func TestNotConfiguredError_hintUsesBuildLocalSurface(t *testing.T) {
	previous := CurrentWorkspace()
	SetCurrentWorkspace(WorkspaceLocal)
	t.Cleanup(func() { SetCurrentWorkspace(previous) })

	source := NotConfiguredError()
	var original *errs.ConfigError
	if !errors.As(source, &original) || !strings.Contains(original.Hint, "config init") {
		t.Fatalf("producer must hint at config init, got %+v", source)
	}

	plan := surface.NewPlan(map[surface.CommandID]surface.CommandState{
		surface.CommandConfigInit: surface.CommandConcealed,
	})
	rendered := recovery.Render(source, plan)
	var concealed *errs.ConfigError
	if !errors.As(rendered, &concealed) {
		t.Fatalf("rendered error = %T, want *errs.ConfigError", rendered)
	}
	if concealed == original {
		t.Fatal("Render must clone the typed error")
	}
	if concealed.Subtype != errs.SubtypeNotConfigured {
		t.Errorf("subtype = %q, want not_configured", concealed.Subtype)
	}
	if strings.Contains(concealed.Hint, "config init") ||
		!strings.Contains(concealed.Hint, "configure this distribution") {
		t.Errorf("concealed hint = %q, want target-free configuration fallback", concealed.Hint)
	}
	var visible *errs.ConfigError
	if !errors.As(recovery.Render(source, nil), &visible) || !strings.Contains(visible.Hint, "config init") {
		t.Errorf("visible render must keep config init, got %+v", visible)
	}
	if !strings.Contains(original.Hint, "config init") {
		t.Errorf("concealed render mutated source hint: %q", original.Hint)
	}
}
