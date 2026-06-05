// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform_test

import (
	"testing"

	"github.com/larksuite/cli/extension/platform"
)

func TestRisk_Rank_orderedTaxonomy(t *testing.T) {
	cases := []struct {
		level platform.Risk
		want  int
	}{
		{platform.RiskRead, 0},
		{platform.RiskWrite, 1},
		{platform.RiskHighRiskWrite, 2},
	}
	for _, c := range cases {
		got, ok := c.level.Rank()
		if !ok || got != c.want {
			t.Errorf("Risk(%q).Rank() = (%d,%v), want (%d,true)", c.level, got, ok, c.want)
		}
	}

	if _, ok := platform.Risk("unknown-level").Rank(); ok {
		t.Fatalf("unknown-level.Rank() ok should be false")
	}
	if _, ok := platform.Risk("").Rank(); ok {
		t.Fatalf("empty.Rank() ok should be false (signals 'no risk annotation')")
	}
}

// The Risk ordering must be strict: read < write < high-risk-write. The
// policy engine compares ranks; a regression that swaps the order would
// silently let high-risk commands pass under MaxRisk=write.
func TestRisk_Rank_strictlyMonotonic(t *testing.T) {
	r1, _ := platform.RiskRead.Rank()
	r2, _ := platform.RiskWrite.Rank()
	r3, _ := platform.RiskHighRiskWrite.Rank()
	if !(r1 < r2 && r2 < r3) {
		t.Fatalf("Risk ranks not monotonic: read=%d write=%d high=%d", r1, r2, r3)
	}
}

func TestRisk_IsValid(t *testing.T) {
	valid := []platform.Risk{platform.RiskRead, platform.RiskWrite, platform.RiskHighRiskWrite}
	for _, r := range valid {
		if !r.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", r)
		}
	}
	invalid := []platform.Risk{"", "wrtie", "Read", "READ", " read "}
	for _, r := range invalid {
		if r.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", r)
		}
	}
}

// ParseRisk distinguishes absent (empty input) from invalid (typo).
// The absent / invalid split mirrors the cmdpolicy engine's
// risk_not_annotated vs risk_invalid reason codes.
func TestParseRisk(t *testing.T) {
	// Empty -> ("", nil) — "not specified"
	got, err := platform.ParseRisk("")
	if err != nil || got != "" {
		t.Errorf(`ParseRisk("") = (%q,%v), want ("",nil)`, got, err)
	}

	// Valid values pass through
	for _, want := range []platform.Risk{platform.RiskRead, platform.RiskWrite, platform.RiskHighRiskWrite} {
		got, err := platform.ParseRisk(string(want))
		if err != nil || got != want {
			t.Errorf("ParseRisk(%q) = (%q,%v), want (%q,nil)", want, got, err, want)
		}
	}

	// Typo -> error, strict matching (case-sensitive, no trim)
	bad := []string{"wrtie", "Read", "READ", " read ", "high_risk_write"}
	for _, s := range bad {
		got, err := platform.ParseRisk(s)
		if err == nil {
			t.Errorf("ParseRisk(%q) succeeded (got %q), want error", s, got)
		}
		if got != "" {
			t.Errorf("ParseRisk(%q) returned %q, want empty Risk on error", s, got)
		}
	}
}

func TestParseIdentity(t *testing.T) {
	got, err := platform.ParseIdentity("")
	if err != nil || got != "" {
		t.Errorf(`ParseIdentity("") = (%q,%v), want ("",nil)`, got, err)
	}
	for _, want := range []platform.Identity{platform.IdentityUser, platform.IdentityBot} {
		got, err := platform.ParseIdentity(string(want))
		if err != nil || got != want {
			t.Errorf("ParseIdentity(%q) = (%q,%v)", want, got, err)
		}
	}
	if _, err := platform.ParseIdentity("admin"); err == nil {
		t.Fatalf(`ParseIdentity("admin") want error`)
	}
}

func TestIdentity_IsValid(t *testing.T) {
	if !platform.IdentityUser.IsValid() {
		t.Error("user.IsValid() = false")
	}
	if !platform.IdentityBot.IsValid() {
		t.Error("bot.IsValid() = false")
	}
	if platform.Identity("admin").IsValid() {
		t.Error("admin.IsValid() = true")
	}
}
