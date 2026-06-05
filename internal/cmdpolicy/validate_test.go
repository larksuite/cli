// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
)

// nil rule is "no restriction" everywhere -- validation must agree.
func TestValidateRule_nilIsOk(t *testing.T) {
	if err := cmdpolicy.ValidateRule(nil); err != nil {
		t.Fatalf("nil rule should validate, got %v", err)
	}
}

func TestValidateRule_validRule(t *testing.T) {
	r := &platform.Rule{
		Allow:      []string{"docs/**", "contact/+search-*"},
		Deny:       []string{"docs/+delete-doc"},
		MaxRisk:    "write",
		Identities: []platform.Identity{"user", "bot"},
	}
	if err := cmdpolicy.ValidateRule(r); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
}

// A typo in MaxRisk must abort the load; otherwise the engine would skip
// the risk check entirely and let high-risk-write commands pass under
// what the operator thought was a "read" cap.
func TestValidateRule_badMaxRisk(t *testing.T) {
	cases := []string{"readd", "Read", "high_risk_write", "anything"}
	for _, bad := range cases {
		r := &platform.Rule{MaxRisk: platform.Risk(bad)}
		err := cmdpolicy.ValidateRule(r)
		if err == nil {
			t.Errorf("ValidateRule should reject MaxRisk=%q", bad)
			continue
		}
		if !strings.Contains(err.Error(), "max_risk") {
			t.Errorf("error should mention max_risk for MaxRisk=%q, got %v", bad, err)
		}
	}
}

// Identities must come from the closed taxonomy {"user","bot"}. A typo
// like "users" would silently lock out everyone (no command intersects
// the typo), so it must abort.
func TestValidateRule_badIdentity(t *testing.T) {
	r := &platform.Rule{Identities: []platform.Identity{"user", "admin"}}
	err := cmdpolicy.ValidateRule(r)
	if err == nil {
		t.Fatalf("ValidateRule should reject identity 'admin'")
	}
	if !strings.Contains(err.Error(), "identities") {
		t.Fatalf("error should mention identities, got %v", err)
	}
}

// Malformed doublestar globs are silent fail-open if not caught here
// (doublestar.Match returns an error which matchesAny() ignores).
func TestValidateRule_malformedGlob(t *testing.T) {
	cases := []struct {
		name string
		rule *platform.Rule
	}{
		{"bad allow", &platform.Rule{Allow: []string{"docs/[abc"}}},
		{"bad deny", &platform.Rule{Deny: []string{"docs/[abc"}}},
		{"empty allow entry", &platform.Rule{Allow: []string{"", "docs/**"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := cmdpolicy.ValidateRule(c.rule)
			if err == nil {
				t.Fatalf("ValidateRule should reject %+v", c.rule)
			}
		})
	}
}

// Empty MaxRisk and Empty Identities slices are both "no restriction" --
// not an error.
func TestValidateRule_emptyFieldsAreOk(t *testing.T) {
	r := &platform.Rule{
		Allow:      []string{"docs/**"},
		MaxRisk:    "",
		Identities: nil,
	}
	if err := cmdpolicy.ValidateRule(r); err != nil {
		t.Fatalf("empty optional fields should validate, got %v", err)
	}
}
