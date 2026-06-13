// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestInputViewKeepsChangedFactsWithOriginalRefs(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "old noisy command", Source: "shortcut"},
			{Path: "docs +fetch", Changed: true, Source: "shortcut", NameConflictsExisting: true},
		},
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-old/SKILL.md", Line: 3, Raw: "old noisy skill"},
			{SourceFile: "skills/lark-doc/SKILL.md", Line: 9, Raw: "changed skill", Changed: true, ReferencesInvalidCommand: true},
		},
		SkillQuality: []facts.SkillQualityFact{
			{SourceFile: "skills/lark-old/SKILL.md", WordCount: 10},
			{SourceFile: "skills/lark-doc/SKILL.md", Changed: true, WordCount: 3000, CriticalOverBudget: true},
		},
		Errors: []facts.ErrorFact{
			{File: "old.go", Line: 10, Boundary: true, RequiredHint: true},
			{File: "cmd/docs.go", Line: 20, Changed: true, Boundary: true, RequiredHint: true},
		},
		Outputs: []facts.OutputFact{
			{Command: "old list", IsList: true},
			{Command: "docs list", Changed: true, IsList: true},
		},
		Examples: []facts.CommandExample{
			{Raw: "lark-cli old noisy command", SourceFile: "skills/lark-old/SKILL.md", Line: 12},
			{Raw: "lark-cli docs +fetch", SourceFile: "skills/lark-doc/SKILL.md", Line: 13, Changed: true},
		},
	}

	view := BuildInputView(f)
	if got := singleRef(t, view.Commands); got != "facts.commands[1]" {
		t.Fatalf("command ref = %q, want facts.commands[1]", got)
	}
	if got := singleRef(t, view.Skills); got != "facts.skills[1]" {
		t.Fatalf("skill ref = %q, want facts.skills[1]", got)
	}
	if got := singleRef(t, view.SkillQuality); got != "facts.skill_quality[1]" {
		t.Fatalf("skill quality ref = %q, want facts.skill_quality[1]", got)
	}
	if got := singleRef(t, view.Errors); got != "facts.errors[1]" {
		t.Fatalf("error ref = %q, want facts.errors[1]", got)
	}
	if got := singleRef(t, view.Outputs); got != "facts.outputs[1]" {
		t.Fatalf("output ref = %q, want facts.outputs[1]", got)
	}
	if got := singleRef(t, view.Examples); got != "facts.examples[1]" {
		t.Fatalf("example ref = %q, want facts.examples[1]", got)
	}

	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	if strings.Contains(string(data), "old noisy") {
		t.Fatalf("view leaked unchanged noisy facts: %s", data)
	}
}

func TestInputViewIncludesSemanticDiagnosticContext(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-old/SKILL.md", Line: 4, Raw: "unrelated"},
			{SourceFile: "skills/lark-doc/SKILL.md", Line: 17, Raw: "bad reference", ReferencesInvalidCommand: true},
		},
		Outputs: []facts.OutputFact{
			{Command: "docs list", IsList: true, HasDefaultLimit: false},
			{Command: "old list", IsList: true, HasDefaultLimit: false},
		},
		Diagnostics: []facts.DiagnosticFact{
			{
				Rule:       "skill_command_reference",
				Action:     report.ActionReject,
				File:       "skills/lark-doc/SKILL.md",
				Line:       17,
				Message:    "example references unknown command",
				Suggestion: "fix the command",
			},
			{
				Rule:       "default_output_contract",
				Action:     report.ActionReject,
				File:       "command-manifest",
				Message:    "docs list default output must include a default limit and agent decision fields",
				Suggestion: "add a default limit",
			},
		},
	}

	view := BuildInputView(f)
	if got := singleRef(t, view.Skills); got != "facts.skills[1]" {
		t.Fatalf("diagnostic skill ref = %q, want facts.skills[1]", got)
	}
	if got := singleRef(t, view.Outputs); got != "facts.outputs[0]" {
		t.Fatalf("diagnostic output ref = %q, want facts.outputs[0]", got)
	}
	if len(view.Diagnostics) != 2 {
		t.Fatalf("diagnostics len = %d, want 2", len(view.Diagnostics))
	}
}

func TestInputViewUsesDiagnosticCommandPath(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Outputs: []facts.OutputFact{
			{Command: "docs list", IsList: true, HasDefaultLimit: false},
			{Command: "old list", IsList: true, HasDefaultLimit: false},
		},
		Diagnostics: []facts.DiagnosticFact{{
			Rule:        "default_output_contract",
			Action:      report.ActionReject,
			File:        "command-manifest",
			Message:     "default output contract failed",
			CommandPath: "docs list",
			SubjectType: "output",
		}},
	}

	view := BuildInputView(f)
	if got := singleRef(t, view.Outputs); got != "facts.outputs[0]" {
		t.Fatalf("diagnostic output ref = %q, want facts.outputs[0]", got)
	}
	if len(view.Diagnostics) != 1 {
		t.Fatalf("diagnostics len = %d, want 1", len(view.Diagnostics))
	}
}

func TestInputViewDropsUnchangedWarningDiagnostics(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Outputs: []facts.OutputFact{{
			Command: "old list",
			IsList:  true,
		}},
		Diagnostics: []facts.DiagnosticFact{{
			Rule:       "default_output",
			Action:     report.ActionWarning,
			File:       "command-manifest",
			Message:    "old list looks like a list command without an explicit default limit flag",
			Suggestion: "add a default limit",
		}},
	}

	view := BuildInputView(f)
	if len(view.Outputs) != 0 {
		t.Fatalf("outputs len = %d, want 0 for unchanged warning", len(view.Outputs))
	}
	if len(view.Diagnostics) != 0 {
		t.Fatalf("diagnostics len = %d, want 0 for unchanged warning", len(view.Diagnostics))
	}
}

func TestBuildPromptUsesInputViewInsteadOfFullFacts(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "old noisy command", Source: "shortcut"},
			{Path: "docs +fetch", Changed: true, Source: "shortcut", NameConflictsExisting: true},
		},
	}

	messages := BuildPrompt(f)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if strings.Contains(messages[1].Content, "old noisy command") {
		t.Fatalf("prompt leaked full facts: %s", messages[1].Content)
	}
	var view InputView
	if err := json.Unmarshal([]byte(messages[1].Content), &view); err != nil {
		t.Fatalf("prompt user content is not input view JSON: %v", err)
	}
	if got := singleRef(t, view.Commands); got != "facts.commands[1]" {
		t.Fatalf("prompt command ref = %q, want facts.commands[1]", got)
	}
}

func TestBuildPromptDescribesErrorHintRubric(t *testing.T) {
	messages := BuildPrompt(facts.Facts{SchemaVersion: 1})
	system := messages[0].Content
	for _, want := range []string{"error_hint", "required_hint", "hint_action_count", "facts.errors"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt missing %q: %s", want, system)
		}
	}
}

type refItem interface {
	ref() string
}

func singleRef[T refItem](t *testing.T, items []T) string {
	t.Helper()
	if len(items) != 1 {
		t.Fatalf("items len = %d, want 1", len(items))
	}
	return items[0].ref()
}
