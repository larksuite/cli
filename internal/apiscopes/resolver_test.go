// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apiscopes

import (
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/registry"
	"github.com/larksuite/cli/shortcuts"
	"github.com/larksuite/cli/shortcuts/common"
)

func testResolver(t *testing.T) Resolver {
	t.Helper()
	snap, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatalf("OpenSnapshot: %v", err)
	}
	cat := snap.Catalog()
	if err := cat.Preload(cat.Names()...); err != nil {
		t.Fatalf("Preload: %v", err)
	}
	if len(cat.Names()) == 0 {
		t.Fatal("embedded catalog is empty")
	}
	return NewResolver(cat, shortcuts.AllShortcuts())
}

// Brand differentiation flows only through the shortcuts layer: feishu carries
// the apps domain, lark does not.
func TestSorted_BrandDifference_AppsDomain(t *testing.T) {
	r := testResolver(t)
	has := func(brand core.LarkBrand, domain string) bool {
		for _, d := range r.Sorted(brand) {
			if d == domain {
				return true
			}
		}
		return false
	}
	if !has(core.BrandFeishu, "apps") {
		t.Error("feishu should include the apps domain")
	}
	if has(core.BrandLark, "apps") {
		t.Error("lark should not include the apps domain")
	}
}

// FilterBatchExcludedScopes drops im:message.send_as_user and keeps the rest in
// order.
func TestFilterBatchExcludedScopes(t *testing.T) {
	in := []string{"a", "im:message.send_as_user", "b"}
	got := FilterBatchExcludedScopes(in)
	want := []string{"a", "b"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestBatchExcludedScopes_ContainsSendAsUser(t *testing.T) {
	if !batchExcludedScopes["im:message.send_as_user"] {
		t.Fatal("batchExcludedScopes must contain im:message.send_as_user")
	}
}

// ScopesFor returns a deduplicated, sorted slice.
func TestScopesFor_SortedAndDeduped(t *testing.T) {
	r := testResolver(t)
	scopes := r.ScopesFor(r.Sorted(core.BrandFeishu), "user", core.BrandFeishu)
	for i := 1; i < len(scopes); i++ {
		if scopes[i-1] > scopes[i] {
			t.Fatalf("scopes not sorted: %q > %q", scopes[i-1], scopes[i])
		}
		if scopes[i-1] == scopes[i] {
			t.Fatalf("scopes not deduped: %q", scopes[i])
		}
	}
}

// Sorted excludes domains that have auth_domain set (they fold into their
// parent).
func TestSorted_ExcludesAuthDomainChildren(t *testing.T) {
	r := testResolver(t)
	for _, d := range r.Sorted(core.BrandFeishu) {
		if registry.HasAuthDomain(d) {
			t.Errorf("Sorted must not include auth_domain child: %q", d)
		}
	}
}

func TestShortcutSupportsIdentity_DefaultUser(t *testing.T) {
	// Empty AuthTypes defaults to ["user"]
	sc := common.Shortcut{AuthTypes: nil}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected default to support 'user'")
	}
	if shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected default to NOT support 'bot'")
	}
}

func TestShortcutSupportsIdentity_ExplicitTypes(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"user", "bot"}}
	if !shortcutSupportsIdentity(sc, "user") {
		t.Error("expected to support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected to support 'bot'")
	}
	if shortcutSupportsIdentity(sc, "tenant") {
		t.Error("expected to NOT support 'tenant'")
	}
}

func TestShortcutSupportsIdentity_BotOnly(t *testing.T) {
	sc := common.Shortcut{AuthTypes: []string{"bot"}}
	if shortcutSupportsIdentity(sc, "user") {
		t.Error("expected bot-only to NOT support 'user'")
	}
	if !shortcutSupportsIdentity(sc, "bot") {
		t.Error("expected bot-only to support 'bot'")
	}
}

// Complete filters the sorted domain set by prefix.
func TestComplete_PrefixFilter(t *testing.T) {
	r := testResolver(t)
	all := r.Sorted(core.BrandFeishu)
	if len(all) == 0 {
		t.Skip("empty domain set")
	}
	prefix := all[0][:1]
	got := r.Complete(prefix, core.BrandFeishu)
	for _, d := range got {
		if len(d) < len(prefix) || d[:len(prefix)] != prefix {
			t.Errorf("Complete(%q) returned non-matching %q", prefix, d)
		}
	}
}
