// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

type stubPlugin struct{ name string }

func (s stubPlugin) Name() string                        { return s.name }
func (s stubPlugin) Version() string                     { return "0.0.1" }
func (s stubPlugin) Capabilities() platform.Capabilities { return platform.Capabilities{} }
func (s stubPlugin) Install(platform.Registrar) error    { return nil }

// Tests should always reset the global registry to keep them
// independent. Verifies the reset hook is functional.
func TestRegister_preservesInsertionOrder(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	platform.Register(stubPlugin{name: "a"})
	platform.Register(stubPlugin{name: "b"})
	platform.Register(stubPlugin{name: "c"})

	got := platform.RegisteredPlugins()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %d plugins, want %d", len(got), len(want))
	}
	for i, p := range got {
		if p.Name() != want[i] {
			t.Errorf("plugins[%d] = %q, want %q", i, p.Name(), want[i])
		}
	}
}

func TestRegister_resetClears(t *testing.T) {
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)
	platform.Register(stubPlugin{name: "a"})
	if len(platform.RegisteredPlugins()) != 1 {
		t.Fatalf("expected 1 plugin")
	}
	platform.ResetForTesting()
	if len(platform.RegisteredPlugins()) != 0 {
		t.Fatalf("expected reset to clear")
	}
}
