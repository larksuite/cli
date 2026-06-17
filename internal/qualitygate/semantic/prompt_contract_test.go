// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
)

func TestBuildPromptContainsSemanticReviewContract(t *testing.T) {
	messages := BuildPrompt(facts.Facts{SchemaVersion: 1})
	if len(messages) == 0 {
		t.Fatal("BuildPrompt returned no messages")
	}
	system := messages[0].Content
	for _, want := range []string{
		"Report an error_hint finding for any facts.errors item where boundary is true, required_hint is true, and hint_action_count is 0.",
		"Report a default_output finding for any facts.outputs item where is_list is true and either has_default_limit is false or has_decision_field is false.",
		"The default_output rule is an OR rule: missing either has_default_limit or has_decision_field is enough to report the finding.",
		"A facts.outputs item with is_list true, has_default_limit false, and has_decision_field true must still produce a default_output finding.",
		"Report a naming finding for any facts.commands item where name_conflicts_existing is true or flag_alias_conflict is true.",
		"Report a skill_quality finding for any facts.skills item where references_invalid_command is true.",
		"Only facts.commands, facts.skills, facts.errors, and facts.outputs fact_ref values may be blocker evidence.",
		"Evidence entries must be exact fact_ref strings such as \"facts.commands[0]\" with no explanations, labels, or suffix text.",
		"facts.examples and facts.skill_quality entries are context only.",
		"Report each distinct issue as a separate finding.",
		"The verdict value must be \"pass\" when findings is empty and \"warn\" when findings is non-empty; never use \"fail\".",
		"Severity must be one of \"minor\", \"major\", or \"critical\"; never use \"error\", \"warning\", \"medium\", or \"high\".",
		"Every finding must include non-empty severity, message, and suggested_action fields.",
		"Return strict JSON with verdict and findings only.",
	} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing contract %q\nprompt:\n%s", want, system)
		}
	}
	for _, forbidden := range []string{"destructive_without_guard", "scope_conflict"} {
		if strings.Contains(system, forbidden) {
			t.Fatalf("system prompt must not mention uncollected skill_quality predicate %q\nprompt:\n%s", forbidden, system)
		}
	}
}

func TestBuildInputViewSelectsChangedReviewCandidatesWithStableRefs(t *testing.T) {
	view := BuildInputView(facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "base +old", Source: "shortcut"},
			{Path: "base +new", Domain: "base", Changed: true, Source: "shortcut", NameConflictsExisting: true},
		},
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-base/SKILL.md", Line: 10, Raw: "unchanged", CommandPath: "base +old"},
			{SourceFile: "skills/lark-base/SKILL.md", Line: 20, Raw: "changed", CommandPath: "base +new", Changed: true, ReferencesInvalidCommand: true},
		},
		SkillQuality: []facts.SkillQualityFact{
			{SourceFile: "skills/lark-base/SKILL.md", WordCount: 100},
			{SourceFile: "skills/lark-base/SKILL.md", Changed: true, WordCount: 420, CriticalCount: 3, CriticalOverBudget: true},
		},
		Errors: []facts.ErrorFact{
			{File: "shortcuts/base/old.go", Line: 8, Boundary: true, RequiredHint: true, HintActionCount: 0},
			{File: "shortcuts/base/new.go", Line: 18, Command: "base +new", CommandPath: "base +new", Changed: true, Boundary: true, RequiredHint: true, HintActionCount: 0},
		},
		Outputs: []facts.OutputFact{
			{Command: "base +old", IsList: true, HasDefaultLimit: true, HasDecisionField: true},
			{Command: "base +new", Changed: true, IsList: true, HasDefaultLimit: false, HasDecisionField: true},
		},
		Examples: []facts.CommandExample{
			{Raw: "lark-cli base +old", SourceFile: "skills/lark-base/SKILL.md", Line: 30, Executable: true},
			{Raw: "lark-cli base +new", SourceFile: "skills/lark-base/SKILL.md", Line: 40, CommandPath: "base +new", Changed: true, Executable: true},
		},
	})

	if got := singleRef(t, view.Commands); got != "facts.commands[1]" {
		t.Fatalf("command ref = %q, want facts.commands[1]", got)
	}
	if got := singleRef(t, view.Skills); got != "facts.skills[1]" {
		t.Fatalf("skill ref = %q, want facts.skills[1]", got)
	}
	if len(view.SkillQuality) != 0 {
		t.Fatalf("skill quality len = %d, want 0 without diagnostics", len(view.SkillQuality))
	}
	if got := singleRef(t, view.Errors); got != "facts.errors[1]" {
		t.Fatalf("error ref = %q, want facts.errors[1]", got)
	}
	if len(view.Outputs) != 0 {
		t.Fatalf("outputs len = %d, want 0 without reject diagnostics", len(view.Outputs))
	}
	if len(view.Examples) != 0 {
		t.Fatalf("examples len = %d, want 0 without diagnostics", len(view.Examples))
	}
	if view.ChangedSummary.Commands != 1 ||
		view.ChangedSummary.Skills != 1 ||
		view.ChangedSummary.SkillQuality != 1 ||
		view.ChangedSummary.Errors != 1 ||
		view.ChangedSummary.Outputs != 1 ||
		view.ChangedSummary.Examples != 1 {
		t.Fatalf("changed summary = %#v", view.ChangedSummary)
	}
}
