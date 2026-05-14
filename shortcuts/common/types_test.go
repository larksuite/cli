// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"reflect"
	"testing"
)

func TestScopesForIdentity_FallbackToScopes(t *testing.T) {
	s := Shortcut{Scopes: []string{"a", "b"}}
	for _, id := range []string{"user", "bot", "tenant", ""} {
		got := s.ScopesForIdentity(id)
		if !reflect.DeepEqual(got, s.Scopes) {
			t.Errorf("identity=%q: expected %v, got %v", id, s.Scopes, got)
		}
	}
}

func TestScopesForIdentity_UserScopesOverride(t *testing.T) {
	s := Shortcut{
		Scopes:     []string{"default"},
		UserScopes: []string{"user-only"},
	}
	if got := s.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"user-only"}) {
		t.Errorf("expected UserScopes, got %v", got)
	}
	// bot should still fall back
	if got := s.ScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"default"}) {
		t.Errorf("expected Scopes fallback for bot, got %v", got)
	}
}

func TestScopesForIdentity_BotScopesOverride(t *testing.T) {
	s := Shortcut{
		Scopes:    []string{"default"},
		BotScopes: []string{"bot-only"},
	}
	if got := s.ScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"bot-only"}) {
		t.Errorf("expected BotScopes, got %v", got)
	}
	// user should still fall back
	if got := s.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"default"}) {
		t.Errorf("expected Scopes fallback for user, got %v", got)
	}
}

func TestScopesForIdentity_BothOverrides(t *testing.T) {
	s := Shortcut{
		Scopes:     []string{"default"},
		UserScopes: []string{"u1", "u2"},
		BotScopes:  []string{"b1"},
	}
	if got := s.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"u1", "u2"}) {
		t.Errorf("expected UserScopes, got %v", got)
	}
	if got := s.ScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"b1"}) {
		t.Errorf("expected BotScopes, got %v", got)
	}
	// unknown identity falls back
	if got := s.ScopesForIdentity("tenant"); !reflect.DeepEqual(got, []string{"default"}) {
		t.Errorf("expected Scopes fallback for tenant, got %v", got)
	}
}

func TestScopesForIdentity_NilScopes(t *testing.T) {
	s := Shortcut{}
	got := s.ScopesForIdentity("user")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestConditionalScopesForIdentity_FallbackAndOverrides(t *testing.T) {
	s := Shortcut{
		ConditionalScopes:     []string{"c-default"},
		ConditionalUserScopes: []string{"c-user"},
		ConditionalBotScopes:  []string{"c-bot"},
	}
	if got := s.ConditionalScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"c-user"}) {
		t.Errorf("expected user conditional scopes, got %v", got)
	}
	if got := s.ConditionalScopesForIdentity("bot"); !reflect.DeepEqual(got, []string{"c-bot"}) {
		t.Errorf("expected bot conditional scopes, got %v", got)
	}
	if got := s.ConditionalScopesForIdentity("tenant"); !reflect.DeepEqual(got, []string{"c-default"}) {
		t.Errorf("expected default conditional scopes for unknown identity, got %v", got)
	}
}

func TestDeclaredScopesForIdentity_MergesAndDeduplicates(t *testing.T) {
	s := Shortcut{
		Scopes:            []string{"base-a", "shared"},
		ConditionalScopes: []string{"shared", "cond-b"},
	}
	if got := s.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"base-a", "shared", "cond-b"}) {
		t.Errorf("expected merged declared scopes, got %v", got)
	}
}

func TestDeclaredScopesForIdentity_ConditionalOnly(t *testing.T) {
	s := Shortcut{ConditionalScopes: []string{"cond-only"}}
	if got := s.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"cond-only"}) {
		t.Errorf("expected conditional-only declared scopes, got %v", got)
	}
}
