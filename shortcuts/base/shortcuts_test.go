// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "testing"

// TestShortcuts_BaseTokenFlagCarriesAppTokenAlias is the contract test for the
// domain-level alias: Lark's Bitable API names the resource app_token in its
// URL path, so every base "base-token" flag must accept --app-token, unless the
// shortcut already owns a real "app-token" flag (e.g. BaseApp operations). If a
// new shortcut adds a base-token flag without going through withAppTokenAlias
// (e.g. Shortcuts stops wrapping), this test fails.
func TestShortcuts_BaseTokenFlagCarriesAppTokenAlias(t *testing.T) {
	shortcuts := Shortcuts()
	if len(shortcuts) == 0 {
		t.Fatal("Shortcuts() returned no shortcuts")
	}
	seen := 0
	for _, s := range shortcuts {
		hasAppToken := false
		for _, fl := range s.Flags {
			if fl.Name == "app-token" {
				hasAppToken = true
				break
			}
		}
		if hasAppToken {
			continue
		}
		for _, fl := range s.Flags {
			if fl.Name != "base-token" {
				continue
			}
			seen++
			found := false
			for _, a := range fl.Aliases {
				if a == "app-token" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("shortcut %s %s: flag base-token missing app-token alias", s.Service, s.Command)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no base-token flags found — alias wiring untestable")
	}
}

// TestShortcuts_AliasNotDuplicated guards against running the wrapper twice
// appending duplicate aliases.
func TestShortcuts_AliasNotDuplicated(t *testing.T) {
	for _, s := range Shortcuts() {
		for _, fl := range s.Flags {
			count := 0
			for _, a := range fl.Aliases {
				if a == "app-token" {
					count++
				}
			}
			if count > 1 {
				t.Errorf("shortcut %s %s: flag %s has duplicate app-token alias", s.Service, s.Command, fl.Name)
			}
		}
	}
}
