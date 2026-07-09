// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

func TestGatekeeperBlocksOnlyReproducibleAllowlistedFinding(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Outputs: []facts.OutputFact{{
			Command: "im messages list", IsList: true,
			HasDefaultLimit: false, HasFieldSelector: false,
		}},
	}
	r := Review{Findings: []Finding{{
		Category:        "default_output",
		Severity:        "major",
		Evidence:        []string{"facts.outputs[0]"},
		Message:         "default output is unbounded",
		SuggestedAction: "add default limit",
	}}}
	got := Decide(f, r, testBlockingPolicy("default_output"))
	if len(got.Blockers) != 1 {
		t.Fatalf("got %d blockers", len(got.Blockers))
	}
	if got.Blockers[0].ReviewAction != ReviewActionMustFix {
		t.Fatalf("review action = %q, want %q", got.Blockers[0].ReviewAction, ReviewActionMustFix)
	}
	if got.Blockers[0].Fingerprint == "" {
		t.Fatal("blocker fingerprint is empty")
	}
}

func TestGatekeeperDowngradesBadEvidence(t *testing.T) {
	got := Decide(facts.Facts{SchemaVersion: 1}, Review{Findings: []Finding{{
		Category: "default_output",
		Evidence: []string{"facts.outputs[99]"},
	}}}, testBlockingPolicy("default_output"))
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("bad evidence should warn only: %#v", got)
	}
	if got.Warnings[0].ReviewAction != ReviewActionObserve {
		t.Fatalf("review action = %q, want %q", got.Warnings[0].ReviewAction, ReviewActionObserve)
	}
}

func TestGatekeeperSetsStableFingerprintAcrossFactIndexChanges(t *testing.T) {
	findingA := Finding{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skills[0]"},
		Message:         "skill references an invalid command",
		SuggestedAction: "update the command reference",
	}
	findingB := findingA
	findingB.Evidence = []string{"facts.skills[1]"}
	factsA := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-doc/SKILL.md",
			Line:                     30,
			CommandPath:              "docs +fetch",
			ReferencesInvalidCommand: true,
		}},
	}
	factsB := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{
			{
				SourceFile:               "skills/lark-im/SKILL.md",
				Line:                     12,
				CommandPath:              "im +fetch",
				ReferencesInvalidCommand: true,
			},
			{
				SourceFile:               "skills/lark-doc/SKILL.md",
				Line:                     30,
				CommandPath:              "docs +fetch",
				ReferencesInvalidCommand: true,
			},
		},
	}

	gotA := Decide(factsA, Review{Findings: []Finding{findingA}}, testBlockingPolicy("skill_quality"))
	gotB := Decide(factsB, Review{Findings: []Finding{findingB}}, testBlockingPolicy("skill_quality"))

	if len(gotA.Blockers) != 1 || len(gotB.Blockers) != 1 {
		t.Fatalf("expected blockers, got %#v and %#v", gotA, gotB)
	}
	if gotA.Blockers[0].Fingerprint != gotB.Blockers[0].Fingerprint {
		t.Fatalf("fingerprint changed across fact reorder: %q != %q", gotA.Blockers[0].Fingerprint, gotB.Blockers[0].Fingerprint)
	}
}

func TestGatekeeperMergesDuplicateFindingsForSameDecisionUnit(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Outputs: []facts.OutputFact{{
			Command:          "im messages list",
			IsList:           true,
			HasDefaultLimit:  false,
			HasDecisionField: false,
		}},
	}
	review := Review{Findings: []Finding{
		{
			Category:        "default_output",
			Severity:        "major",
			Evidence:        []string{"facts.outputs[0]"},
			Message:         "list output has no default limit",
			SuggestedAction: "add a default limit",
		},
		{
			Category:        "default_output",
			Severity:        "major",
			Evidence:        []string{"facts.outputs[0]"},
			Message:         "list output has no decision field",
			SuggestedAction: "add a decision field",
		},
	}}

	got := Decide(f, review, testBlockingPolicy("default_output"))

	if len(got.Blockers) != 1 {
		t.Fatalf("duplicate decision unit should merge to one blocker, got %#v", got)
	}
	if got.Blockers[0].Fingerprint == "" {
		t.Fatal("merged blocker fingerprint is empty")
	}
}

func TestGatekeeperDuplicateFindingKeepsStrongestAction(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-doc/SKILL.md",
			Line:                     30,
			CommandPath:              "docs +fetch",
			ReferencesInvalidCommand: true,
		}},
	}
	invalidFirst := Finding{
		Category: "skill_quality",
		Evidence: []string{"facts.skills[0]"},
	}
	validSecond := Finding{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skills[0]"},
		Message:         "skill references an invalid command",
		SuggestedAction: "update the command reference",
	}

	got := Decide(f, Review{Findings: []Finding{invalidFirst, validSecond}}, testBlockingPolicy("skill_quality"))

	if len(got.Blockers) != 1 || len(got.Warnings) != 0 {
		t.Fatalf("valid blocker should replace earlier observe duplicate, got %#v", got)
	}
	if got.Blockers[0].ReviewAction != ReviewActionMustFix {
		t.Fatalf("review action = %q, want %q", got.Blockers[0].ReviewAction, ReviewActionMustFix)
	}
}

func TestGatekeeperDuplicateFindingPromotesObserveToConfirmWhenWaived(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{{
			SourceFile:               "skills/lark-doc/SKILL.md",
			Line:                     30,
			CommandPath:              "docs +fetch",
			ReferencesInvalidCommand: true,
		}},
	}
	invalidFirst := Finding{
		Category: "skill_quality",
		Evidence: []string{"facts.skills[0]"},
	}
	validSecond := Finding{
		Category:        "skill_quality",
		Severity:        "major",
		Evidence:        []string{"facts.skills[0]"},
		Message:         "skill references an invalid command",
		SuggestedAction: "update the command reference",
	}
	waivers := Waivers{Items: []Waiver{{
		ID:         "skill-doc-waiver",
		Category:   "skill_quality",
		FactKind:   "skill",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       30,
	}}}

	got := DecideWithWaivers(f, Review{Findings: []Finding{invalidFirst, validSecond}}, testBlockingPolicy("skill_quality"), waivers)

	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("waived finding should replace earlier observe duplicate with confirm, got %#v", got)
	}
	if got.Warnings[0].ReviewAction != ReviewActionConfirm {
		t.Fatalf("review action = %q, want %q", got.Warnings[0].ReviewAction, ReviewActionConfirm)
	}
}

func TestGatekeeperDowngradesEmptyFindingText(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Errors:        []facts.ErrorFact{{Boundary: true, RequiredHint: true, HintActionCount: 0}},
	}
	got := Decide(f, Review{Findings: []Finding{{
		Category: "error_hint",
		Evidence: []string{"facts.errors[0]"},
	}}}, testBlockingPolicy("error_hint"))
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("empty finding text should warn only: %#v", got)
	}
}

func TestGatekeeperDowngradesEmptyFindingSeverity(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Errors:        []facts.ErrorFact{{Boundary: true, RequiredHint: true, HintActionCount: 0}},
	}
	got := Decide(f, Review{Findings: []Finding{{
		Category:        "error_hint",
		Evidence:        []string{"facts.errors[0]"},
		Message:         "hint is not actionable",
		SuggestedAction: "add a concrete recovery command",
	}}}, testBlockingPolicy("error_hint"))
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("empty finding severity should warn only: %#v", got)
	}
}

func TestGatekeeperBlockerMatrix(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Errors:        []facts.ErrorFact{{Code: "invalid_input", Boundary: true, RequiredHint: true, HintActionCount: 0}},
		Outputs:       []facts.OutputFact{{Command: "im messages list", IsList: true, HasDefaultLimit: false, HasDecisionField: false}},
		Commands:      []facts.CommandFact{{Path: "docs fetch", NameConflictsExisting: true}},
		Skills:        []facts.SkillFact{{SourceFile: "skills/lark-doc/SKILL.md", Line: 3, ReferencesInvalidCommand: true}},
		PublicContent: []facts.PublicContentFact{{Rule: "public_content_generic_credential", Action: "REJECT", File: "docs/public.md", Line: 4, Source: "metadata"}},
	}
	for _, tc := range []struct {
		category string
		evidence string
	}{
		{"error_hint", "facts.errors[0]"},
		{"default_output", "facts.outputs[0]"},
		{"naming", "facts.commands[0]"},
		{"skill_quality", "facts.skills[0]"},
		{"public_content_leakage", "facts.public_content[0]"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			r := Review{Findings: []Finding{{
				Category:        tc.category,
				Severity:        "major",
				Evidence:        []string{tc.evidence},
				Message:         "bad",
				SuggestedAction: "fix",
			}}}
			d := Decide(f, r, DefaultPolicy())
			if len(d.Blockers) != 1 {
				t.Fatalf("expected blocker for %s, got %#v", tc.category, d)
			}
		})
	}
}

func TestGatekeeperDoesNotPromotePublicContentWarningsToBlockers(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		PublicContent: []facts.PublicContentFact{{
			Rule:   "public_content_" + "pri" + "vate_ipv4",
			Action: "WARNING",
			File:   "docs/network.md",
			Line:   1,
			Source: "file",
		}},
	}
	review := Review{Findings: []Finding{{
		Category:        "public_content_leakage",
		Severity:        "minor",
		Evidence:        []string{"facts.public_content[0]"},
		Message:         "pri" + "vate network address appears in public docs",
		SuggestedAction: "confirm the public docs do not expose pri" + "vate deployment details",
	}}}

	got := Decide(f, review, DefaultPolicy())
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("public content warning should not become a blocker: %#v", got)
	}
	if got.Warnings[0].ReviewAction != ReviewActionObserve {
		t.Fatalf("review action = %q, want %q", got.Warnings[0].ReviewAction, ReviewActionObserve)
	}
}

func TestGatekeeperAllowsPublicContentSemanticCandidatesAsBlockers(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		PublicContent: []facts.PublicContentFact{{
			Rule:   "public_content_semantic_candidate",
			Action: "WARNING",
			File:   "docs/public.md",
			Line:   1,
			Source: "file",
		}},
	}
	review := Review{Findings: []Finding{{
		Category:        "public_content_leakage",
		Severity:        "major",
		Evidence:        []string{"facts.public_content[0]"},
		Message:         "semantic review found pri" + "vate rollout detail",
		SuggestedAction: "remove pri" + "vate rollout detail from public docs",
	}}}

	got := Decide(f, review, DefaultPolicy())
	if len(got.Blockers) != 1 {
		t.Fatalf("semantic candidate should remain blockable, got %#v", got)
	}
}

func TestGatekeeperSkillQualityOnlyBlocksInvalidCommandReferences(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-doc/SKILL.md", Line: 3, DestructiveWithoutGuard: true},
			{SourceFile: "skills/lark-drive/SKILL.md", Line: 4, ScopeConflict: true},
		},
	}
	r := Review{Findings: []Finding{
		{Category: "skill_quality", Severity: "major", Evidence: []string{"facts.skills[0]"}, Message: "destructive action", SuggestedAction: "add guard"},
		{Category: "skill_quality", Severity: "major", Evidence: []string{"facts.skills[1]"}, Message: "scope conflict", SuggestedAction: "fix scope"},
	}}
	d := Decide(f, r, DefaultPolicy())
	if len(d.Blockers) != 0 || len(d.Warnings) != 2 {
		t.Fatalf("uncollected skill_quality predicates should warn only: %#v", d)
	}
}

func TestGatekeeperDoesNotBlockHelperErrorHint(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Errors: []facts.ErrorFact{{
			File:            "shortcuts/common/runner.go",
			Line:            97,
			Changed:         true,
			Boundary:        false,
			RequiredHint:    true,
			HintActionCount: 0,
		}},
	}
	review := Review{Findings: []Finding{{
		Category:        "error_hint",
		Severity:        "major",
		Evidence:        []string{"facts.errors[0]"},
		Message:         "helper error lacks actionable hint",
		SuggestedAction: "wrap at command boundary",
	}}}
	got := Decide(f, review, Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    []string{"error_hint"},
		RolloutGroups: []RolloutGroup{{
			ID:          "changed-only",
			Enforcement: "blocking",
			Scope:       ScopeSelector{ChangedOnly: true},
			Categories:  []string{"error_hint"},
			Owner:       "test",
			Reason:      "test",
		}},
	})
	if len(got.Blockers) != 0 || len(got.Warnings) != 1 {
		t.Fatalf("helper error_hint should warn only: %#v", got)
	}
}

func TestGatekeeperDoesNotBlockLegacyNamingLabels(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "drive +task_result", Source: "shortcut", LegacyNaming: true},
			{Path: "im messages list", Source: "shortcut", Flags: []string{"sort_type"}, LegacyNaming: true},
		},
	}
	r := Review{Findings: []Finding{
		{Category: "naming", Severity: "major", Evidence: []string{"facts.commands[0]"}, Message: "legacy naming", SuggestedAction: "rename"},
		{Category: "naming", Severity: "major", Evidence: []string{"facts.commands[1]"}, Message: "legacy flag naming", SuggestedAction: "rename"},
	}}
	d := Decide(f, r, DefaultPolicy())
	if len(d.Blockers) != 0 || len(d.Warnings) != 2 {
		t.Fatalf("legacy naming labels must not block: %#v", d)
	}
}

func TestGatekeeperNamingRejectBitsOverrideLegacyLabels(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "docs bad_cmd", Source: "shortcut", LegacyNaming: true, NameConflictsExisting: true},
			{Path: "im messages list", Source: "shortcut", LegacyNaming: true, FlagAliasConflict: true},
		},
	}
	r := Review{Findings: []Finding{
		{Category: "naming", Severity: "major", Evidence: []string{"facts.commands[0]"}, Message: "command conflict", SuggestedAction: "rename"},
		{Category: "naming", Severity: "major", Evidence: []string{"facts.commands[1]"}, Message: "flag conflict", SuggestedAction: "rename flag"},
	}}
	d := Decide(f, r, DefaultPolicy())
	if len(d.Blockers) != 2 {
		t.Fatalf("naming reject bits must block even with legacy labels: %#v", d)
	}
}

func testBlockingPolicy(categories ...string) Policy {
	return Policy{
		SchemaVersion:      1,
		DefaultEnforcement: "observe",
		BlockCategories:    categories,
		RolloutGroups: []RolloutGroup{{
			ID:          "all",
			Enforcement: "blocking",
			Categories:  categories,
			Owner:       "test",
			Reason:      "test",
		}},
	}
}
