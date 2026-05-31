// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform_test

import (
	"context"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/hook"
	internalplatform "github.com/larksuite/cli/internal/platform"
)

func TestBuildInventory_groupsByPluginName(t *testing.T) {
	plugins := []internalplatform.PluginInventorySource{
		{Name: "a", Version: "1.0", Capabilities: platform.Capabilities{
			Restricts: true, FailurePolicy: platform.FailClosed,
		}},
		{Name: "b", Version: "2.0"},
	}

	r := hook.NewRegistry()
	obs := func(context.Context, platform.Invocation) {}
	wrap := func(next platform.Handler) platform.Handler { return next }
	lc := func(context.Context, *platform.LifecycleContext) error { return nil }

	r.AddObserver(hook.ObserverEntry{Name: "a.pre", When: platform.Before, Selector: platform.All(), Fn: obs})
	r.AddObserver(hook.ObserverEntry{Name: "a.post", When: platform.After, Selector: platform.All(), Fn: obs})
	r.AddObserver(hook.ObserverEntry{Name: "b.audit", When: platform.Before, Selector: platform.All(), Fn: obs})
	r.AddWrapper(hook.WrapperEntry{Name: "a.approval", Selector: platform.All(), Fn: wrap})
	r.AddLifecycle(hook.LifecycleEntry{Name: "a.boot", Event: platform.Startup, Fn: lc})
	r.AddLifecycle(hook.LifecycleEntry{Name: "b.bye", Event: platform.Shutdown, Fn: lc})

	rules := []internalplatform.RuleInventorySource{
		{PluginName: "a", RuleName: "a-rule", Allow: []string{"docs/**"}, MaxRisk: "read"},
	}

	inv := internalplatform.BuildInventory(plugins, r, rules)

	if got := len(inv.Plugins); got != 2 {
		t.Fatalf("Plugins len = %d, want 2", got)
	}
	a := findPlugin(inv, "a")
	b := findPlugin(inv, "b")
	if a == nil || b == nil {
		t.Fatalf("missing entries: a=%v b=%v", a, b)
	}

	if got := len(a.Observers); got != 2 {
		t.Errorf("a.Observers = %d, want 2", got)
	}
	if got := len(a.Wrappers); got != 1 {
		t.Errorf("a.Wrappers = %d, want 1", got)
	}
	if got := len(a.Lifecycles); got != 1 {
		t.Errorf("a.Lifecycles = %d, want 1", got)
	}
	if len(a.Rules) != 1 || a.Rules[0].Name != "a-rule" {
		t.Errorf("a.Rules = %+v, want single rule name a-rule", a.Rules)
	}
	if a.Capabilities.FailurePolicy != "FailClosed" {
		t.Errorf("a.Capabilities.FailurePolicy = %q, want FailClosed", a.Capabilities.FailurePolicy)
	}

	if got := len(b.Observers); got != 1 {
		t.Errorf("b.Observers = %d, want 1 (only b.audit)", got)
	}
	if len(b.Rules) != 0 {
		t.Errorf("b.Rules = %+v, want empty (b did not call Restrict)", b.Rules)
	}
	if b.Capabilities.FailurePolicy != "FailOpen" {
		t.Errorf("b.Capabilities.FailurePolicy = %q, want FailOpen (zero value)", b.Capabilities.FailurePolicy)
	}
}

// A plugin contributing several rules (same PluginName, multiple
// RuleInventorySource entries) must surface ALL of them under Rules, in
// order -- not silently overwrite down to the last one. Pins the
// multi-rule inventory fix.
func TestBuildInventory_multipleRulesPerPlugin(t *testing.T) {
	plugins := []internalplatform.PluginInventorySource{
		{Name: "a", Version: "1.0", Capabilities: platform.Capabilities{
			Restricts: true, FailurePolicy: platform.FailClosed,
		}},
	}
	rules := []internalplatform.RuleInventorySource{
		{PluginName: "a", RuleName: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: "read"},
		{PluginName: "a", RuleName: "im-rw", Allow: []string{"im/**"}, MaxRisk: "write"},
	}

	inv := internalplatform.BuildInventory(plugins, nil, rules)
	a := findPlugin(inv, "a")
	if a == nil {
		t.Fatalf("missing entry a")
	}
	if len(a.Rules) != 2 {
		t.Fatalf("a.Rules = %d, want 2 (both rules preserved, no overwrite)", len(a.Rules))
	}
	if a.Rules[0].Name != "docs-ro" || a.Rules[1].Name != "im-rw" {
		t.Errorf("rules out of order: %q, %q", a.Rules[0].Name, a.Rules[1].Name)
	}
}

func TestBuildInventory_empty(t *testing.T) {
	inv := internalplatform.BuildInventory(nil, nil, nil)
	if got := len(inv.Plugins); got != 0 {
		t.Errorf("Plugins len = %d, want 0", got)
	}
}

func findPlugin(inv *internalplatform.Inventory, name string) *internalplatform.PluginEntry {
	for i := range inv.Plugins {
		if inv.Plugins[i].Name == name {
			return &inv.Plugins[i]
		}
	}
	return nil
}
