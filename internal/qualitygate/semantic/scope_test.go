// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

func TestGatekeeperUsesChangedOnlyRollout(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-wiki/SKILL.md",
			Line:                     30,
			Domain:                   "wiki",
			Changed:                  true,
			ReferencesInvalidCommand: true,
		}},
	}
	review := Review{Findings: []Finding{{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skills[0]"},
		Message:         "invalid command reference",
		SuggestedAction: "fix command reference",
	}}}
	got := Decide(f, review, Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    []string{"skill_quality"},
		RolloutGroups: []RolloutGroup{{
			ID:          "changed-only",
			Enforcement: "blocking",
			Scope:       ScopeSelector{ChangedOnly: true},
			Categories:  []string{"skill_quality"},
			Owner:       "cli-owner",
			Reason:      "rollout",
		}},
	})
	if len(got.Blockers) != 1 || got.Blockers[0].RolloutGroups[0] != "changed-only" {
		t.Fatalf("expected changed-only blocker, got %#v", got)
	}

	f.Skills[0].Changed = false
	got = Decide(f, review, Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    []string{"skill_quality"},
		RolloutGroups: []RolloutGroup{{
			ID:          "changed-only",
			Enforcement: "blocking",
			Scope:       ScopeSelector{ChangedOnly: true},
			Categories:  []string{"skill_quality"},
			Owner:       "cli-owner",
			Reason:      "rollout",
		}},
	})
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("unchanged evidence should warn only: %#v", got)
	}
}

func TestGatekeeperSkillQualityUsesSkillEvidence(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		SkillQuality:  []facts.SkillQualityFact{{SourceFile: "skills/lark-wiki/SKILL.md", CriticalOverBudget: true}},
	}
	review := Review{Findings: []Finding{{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skill_quality[0]"},
		Message:         "critical budget",
		SuggestedAction: "trim docs",
	}}}
	got := Decide(f, review, DefaultPolicy())
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("facts.skill_quality should not be v1 blocker evidence: %#v", got)
	}
}

func TestGatekeeperUsesPublicContentEvidence(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		PublicContent: []facts.PublicContentFact{{
			Rule:   "public_content_generic_credential",
			Action: "REJECT",
			File:   "docs/public.md",
			Line:   12,
			Source: "metadata",
		}},
	}
	review := Review{Findings: []Finding{{
		Category:        "public_content_leakage",
		Severity:        "critical",
		Evidence:        []string{"facts.public_content[0]"},
		Message:         "public content finding needs review",
		SuggestedAction: "remove the sensitive public content",
	}}}
	got := Decide(f, review, DefaultPolicy())
	if len(got.Blockers) != 1 || got.Blockers[0].RolloutGroups[0] != "all" {
		t.Fatalf("expected public content blocker, got %#v", got)
	}
}

func TestGatekeeperAppliesSharedWaiverID(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-wiki/SKILL.md", Line: 30, Domain: "wiki", Changed: true, ReferencesInvalidCommand: true},
			{SourceFile: "skills/lark-wiki/references/move.md", Line: 12, Domain: "wiki", Changed: true, ReferencesInvalidCommand: true},
		},
	}
	review := Review{Findings: []Finding{{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skills[0]", "facts.skills[1]"},
		Message:         "skill issues",
		SuggestedAction: "fix docs",
	}}}
	policy := Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    []string{"skill_quality"},
		RolloutGroups: []RolloutGroup{{
			ID:          "changed-only",
			Enforcement: "blocking",
			Scope:       ScopeSelector{ChangedOnly: true},
			Categories:  []string{"skill_quality"},
			Owner:       "cli-owner",
			Reason:      "rollout",
		}},
	}
	waivers := Waivers{Items: []Waiver{
		{ID: "wiki-move", Category: "skill_quality", FactKind: "skill", SourceFile: "skills/lark-wiki/SKILL.md", Line: 30},
		{ID: "wiki-move", Category: "skill_quality", FactKind: "skill", SourceFile: "skills/lark-wiki/references/move.md", Line: 12},
	}}
	got := DecideWithWaivers(f, review, policy, waivers)
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 || got.Warnings[0].WaiverID != "wiki-move" {
		t.Fatalf("expected waived warning, got %#v", got)
	}
	if got.Warnings[0].ReviewAction != ReviewActionConfirm {
		t.Fatalf("review action = %q, want %q", got.Warnings[0].ReviewAction, ReviewActionConfirm)
	}

	waivers.Items[1].ID = "other"
	got = DecideWithWaivers(f, review, policy, waivers)
	if len(got.Blockers) != 1 {
		t.Fatalf("split waiver ids should not waive multi-evidence finding: %#v", got)
	}
	if got.Blockers[0].ReviewAction != ReviewActionMustFix {
		t.Fatalf("review action = %q, want %q", got.Blockers[0].ReviewAction, ReviewActionMustFix)
	}
}

func TestWaiverMatchFindingChoosesDeterministicWaiverID(t *testing.T) {
	scopes := []FactScope{{
		FactKind:   "skill",
		SourceFile: "skills/lark-wiki/SKILL.md",
		Line:       30,
	}}
	waivers := Waivers{Items: []Waiver{
		{ID: "wiki-z", Category: "skill_quality", FactKind: "skill", SourceFile: "skills/lark-wiki/SKILL.md", Line: 30},
		{ID: "wiki-a", Category: "skill_quality", FactKind: "skill", SourceFile: "skills/lark-wiki/SKILL.md", Line: 30},
	}}
	got, _, ok := waivers.MatchFinding("skill_quality", scopes)
	if !ok || got != "wiki-a" {
		t.Fatalf("waiver id = %q, ok=%v", got, ok)
	}
}
