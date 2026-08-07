// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core_test

import (
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/core"
)

// ParseRisk keeps three outcomes apart: absent, valid, invalid. Collapsing
// absent into invalid would make every unannotated command a declaration bug;
// collapsing invalid into absent is the fail-open this whole change exists to
// remove.
func TestParseRisk(t *testing.T) {
	for _, tc := range []struct {
		name    string
		in      string
		want    core.Risk
		wantErr bool
	}{
		{name: "absent", in: "", want: ""},
		{name: "read", in: "read", want: core.RiskRead},
		{name: "write", in: "write", want: core.RiskWrite},
		{name: "high risk write", in: "high-risk-write", want: core.RiskHighRiskWrite},
		{name: "transposed letters", in: "high-risk-wrtie", wantErr: true},
		{name: "capitalised", in: "Read", wantErr: true},
		{name: "upper case", in: "READ", wantErr: true},
		{name: "padded", in: " read ", wantErr: true},
		{name: "unknown tier", in: "danger", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := core.ParseRisk(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseRisk(%q) = (%q, nil), want an error", tc.in, got)
				}
				if got != "" {
					t.Errorf("ParseRisk(%q) returned %q alongside the error, want the zero value so a caller that ignores err cannot use it", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRisk(%q) returned error %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ParseRisk(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A misspelled level must never rank as a valid tier — Rank's ok=false is what
// makes callers fail closed instead of comparing against rank 0 (read).
func TestRiskRankAndValidity(t *testing.T) {
	ranks := map[core.Risk]int{core.RiskRead: 0, core.RiskWrite: 1, core.RiskHighRiskWrite: 2}
	for level, want := range ranks {
		if !level.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", level)
		}
		got, ok := level.Rank()
		if !ok || got != want {
			t.Errorf("%q.Rank() = (%d,%v), want (%d,true)", level, got, ok, want)
		}
	}
	for _, bad := range []core.Risk{"", "high-risk-wrtie", "READ", "danger"} {
		if bad.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", bad)
		}
		if _, ok := bad.Rank(); ok {
			t.Errorf("%q.Rank() reported ok=true, want false", bad)
		}
	}
}

// The taxonomy is declared in three places for three audiences: core (the
// truth), platform (the plugin SDK's own type, so the SDK does not export an
// internal one) and errs (the wire strings on the confirmation envelope,
// which cannot import core without an import cycle). They must not drift.
func TestRiskTaxonomyIsConsistentAcrossPackages(t *testing.T) {
	pairs := []struct {
		core     core.Risk
		platform platform.Risk
		wire     string
	}{
		{core.RiskRead, platform.RiskRead, errs.RiskRead},
		{core.RiskWrite, platform.RiskWrite, errs.RiskWrite},
		{core.RiskHighRiskWrite, platform.RiskHighRiskWrite, errs.RiskHighRiskWrite},
	}
	for _, p := range pairs {
		if string(p.platform) != string(p.core) {
			t.Errorf("platform risk %q != core risk %q", p.platform, p.core)
		}
		if p.wire != string(p.core) {
			t.Errorf("errs wire risk %q != core risk %q", p.wire, p.core)
		}
		if p.platform.Core() != p.core {
			t.Errorf("%q.Core() = %q, want %q", p.platform, p.platform.Core(), p.core)
		}
		if platform.FromCore(p.core) != p.platform {
			t.Errorf("FromCore(%q) = %q, want %q", p.core, platform.FromCore(p.core), p.platform)
		}
	}

	// Value sets must be identical, not merely overlapping: a value one side
	// accepts and the other rejects is exactly the gap a plugin could fall
	// into.
	for _, s := range []string{"read", "write", "high-risk-write", "high-risk-wrtie", "unknown", ""} {
		coreRisk, coreErr := core.ParseRisk(s)
		platformRisk, platformErr := platform.ParseRisk(s)
		if (coreErr == nil) != (platformErr == nil) {
			t.Errorf("ParseRisk(%q): core err=%v, platform err=%v — the two taxonomies disagree", s, coreErr, platformErr)
			continue
		}
		if string(coreRisk) != string(platformRisk) {
			t.Errorf("ParseRisk(%q) = core %q, platform %q", s, coreRisk, platformRisk)
		}
	}
}
