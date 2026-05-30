// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package yaml_test

import (
	"reflect"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	pyaml "github.com/larksuite/cli/internal/cmdpolicy/yaml"
)

func TestParse_validRule(t *testing.T) {
	data := []byte(`
name: agent-docs-readonly
description: only-read docs
allow:
  - docs/**
  - contact/**
deny:
  - docs/+update
max_risk: read
identities:
  - user
`)
	rules, err := pyaml.Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := &platform.Rule{
		Name:        "agent-docs-readonly",
		Description: "only-read docs",
		Allow:       []string{"docs/**", "contact/**"},
		Deny:        []string{"docs/+update"},
		MaxRisk:     "read",
		Identities:  []platform.Identity{"user"},
	}
	// A flat top-level rule yields exactly one element (backward compat).
	if !reflect.DeepEqual(rules, []*platform.Rule{want}) {
		t.Fatalf("rules = %+v, want single %+v", rules, want)
	}
}

// A "rules:" list yields one platform.Rule per entry, in order. This is
// the multi-rule layout: each rule is a scoped grant the engine
// OR-combines.
func TestParse_rulesList(t *testing.T) {
	data := []byte(`
rules:
  - name: docs-ro
    allow: [docs/**]
    max_risk: read
  - name: im-rw
    allow: [im/**]
    max_risk: write
    identities: [user, bot]
`)
	rules, err := pyaml.Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	want := []*platform.Rule{
		{Name: "docs-ro", Allow: []string{"docs/**"}, MaxRisk: "read"},
		{Name: "im-rw", Allow: []string{"im/**"}, MaxRisk: "write", Identities: []platform.Identity{"user", "bot"}},
	}
	if !reflect.DeepEqual(rules, want) {
		t.Fatalf("rules = %+v, want %+v", rules, want)
	}
}

// A "rules:" key that is present but empty is a foot-gun: an empty list
// would otherwise fall through to a single all-zero Rule that allows
// every annotated command ("looks like a policy, enforces almost
// nothing"). Parse must reject it outright instead.
func TestParse_rejectsEmptyRulesList(t *testing.T) {
	if _, err := pyaml.Parse([]byte("rules: []\n")); err == nil {
		t.Fatalf("Parse should reject a present-but-empty 'rules:' list")
	}
}

// Mixing top-level flat rule fields with a rules: list is ambiguous and
// must be rejected rather than silently picking one.
func TestParse_rejectsFlatPlusRulesMix(t *testing.T) {
	data := []byte(`
name: top-level
rules:
  - name: nested
`)
	if _, err := pyaml.Parse(data); err == nil {
		t.Fatalf("Parse should reject mixing top-level fields with a rules: list")
	}
}

// allow_unannotated is documented in the README / author guide as the
// gradual-adoption opt-in. The yaml schema must carry it through to
// platform.Rule, otherwise a user following the docs would either hit
// "unknown field" (under KnownFields strict mode) or silently lose the
// opt-in and end up with a safer-but-broken policy.
func TestParse_allowUnannotatedPassesThrough(t *testing.T) {
	data := []byte(`
name: agent-readonly
max_risk: read
allow_unannotated: true
`)
	rules, err := pyaml.Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if !rules[0].AllowUnannotated {
		t.Fatalf("AllowUnannotated = false, want true (yaml field must propagate)")
	}
	if rules[0].MaxRisk != "read" || rules[0].Name != "agent-readonly" {
		t.Errorf("other fields lost: %+v", rules[0])
	}
}

// Default is false when the key is absent: pin the fail-closed default so
// future schema edits cannot accidentally flip it.
func TestParse_allowUnannotatedDefaultsFalse(t *testing.T) {
	data := []byte(`
name: x
max_risk: read
`)
	rules, err := pyaml.Parse(data)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if rules[0].AllowUnannotated {
		t.Fatalf("AllowUnannotated must default to false when key is absent")
	}
}

// Unknown fields must be rejected so the old binary cannot silently ignore
// new schema additions (forward-compat safeguard).
func TestParse_rejectsUnknownFields(t *testing.T) {
	data := []byte(`
name: x
mystery_field: oh no
`)
	if _, err := pyaml.Parse(data); err == nil {
		t.Fatalf("Parse should reject unknown yaml field 'mystery_field'")
	}
}

// Semantic validation lives in cmdpolicy.ValidateRule. Parse only checks
// structural yaml; an invalid max_risk passes through (validation happens
// downstream).
func TestParse_doesNotValidateSemantics(t *testing.T) {
	rules, err := pyaml.Parse([]byte("max_risk: nuclear\n"))
	if err != nil {
		t.Fatalf("structural parse should succeed, got %v", err)
	}
	if rules[0].MaxRisk != "nuclear" {
		t.Fatalf("MaxRisk = %q, want passed through as-is", rules[0].MaxRisk)
	}
}

// An entirely empty file is rejected: the resolver should fall back to
// "no rule" by skipping the file in the first place, not by feeding empty
// bytes through Parse.
func TestParse_emptyIsError(t *testing.T) {
	if _, err := pyaml.Parse([]byte{}); err == nil {
		t.Fatalf("Parse should reject empty input; the resolver handles 'no file' separately")
	}
}

// A stray "---" separator followed by another document would silently
// drop the trailing rule if yaml.v3 stopped after the first Decode.
// Parse must reject multi-document input so the operator can't typo a
// separator and end up with an unintentionally empty policy.
func TestParse_rejectsMultipleDocuments(t *testing.T) {
	data := []byte(`name: first
max_risk: read
---
name: second
max_risk: write
`)
	if _, err := pyaml.Parse(data); err == nil {
		t.Fatalf("Parse should reject multi-document YAML input")
	}
}
