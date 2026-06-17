// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestInputViewKeepsChangedReviewCandidatesWithOriginalRefs(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{
			{Path: "old noisy command", Source: "shortcut"},
			{Path: "docs +clean", Changed: true, Source: "shortcut"},
			{Path: "docs +fetch", Changed: true, Source: "shortcut", NameConflictsExisting: true},
		},
		Skills: []facts.SkillFact{
			{SourceFile: "skills/lark-old/SKILL.md", Line: 3, Raw: "old noisy skill"},
			{SourceFile: "skills/lark-doc/SKILL.md", Line: 8, Raw: "changed clean skill", Changed: true},
			{SourceFile: "skills/lark-doc/SKILL.md", Line: 9, Raw: "changed skill", Changed: true, ReferencesInvalidCommand: true},
		},
		SkillQuality: []facts.SkillQualityFact{
			{SourceFile: "skills/lark-old/SKILL.md", WordCount: 10},
			{SourceFile: "skills/lark-doc/SKILL.md", Changed: true, WordCount: 3000, CriticalOverBudget: true},
		},
		Errors: []facts.ErrorFact{
			{File: "old.go", Line: 10, Boundary: true, RequiredHint: true},
			{File: "cmd/docs.go", Line: 19, Changed: true, Boundary: true, RequiredHint: true, HintActionCount: 1},
			{File: "cmd/docs.go", Line: 20, Changed: true, Boundary: true, RequiredHint: true},
		},
		Outputs: []facts.OutputFact{
			{Command: "old list", IsList: true},
			{Command: "docs clean-list", Changed: true, IsList: true, HasDefaultLimit: true, HasDecisionField: true},
			{Command: "docs list", Changed: true, IsList: true},
		},
		Examples: []facts.CommandExample{
			{Raw: "lark-cli old noisy command", SourceFile: "skills/lark-old/SKILL.md", Line: 12},
			{Raw: "lark-cli docs +fetch", SourceFile: "skills/lark-doc/SKILL.md", Line: 13, Changed: true},
		},
	}

	view := BuildInputView(f)
	if got := singleRef(t, view.Commands); got != "facts.commands[2]" {
		t.Fatalf("command ref = %q, want facts.commands[2]", got)
	}
	if got := singleRef(t, view.Skills); got != "facts.skills[2]" {
		t.Fatalf("skill ref = %q, want facts.skills[2]", got)
	}
	if len(view.SkillQuality) != 0 {
		t.Fatalf("skill quality len = %d, want 0 without diagnostics", len(view.SkillQuality))
	}
	if got := singleRef(t, view.Errors); got != "facts.errors[2]" {
		t.Fatalf("error ref = %q, want facts.errors[2]", got)
	}
	if len(view.Outputs) != 0 {
		t.Fatalf("outputs len = %d, want 0 without reject diagnostics", len(view.Outputs))
	}
	if len(view.Examples) != 0 {
		t.Fatalf("examples len = %d, want 0 without diagnostics", len(view.Examples))
	}

	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	for _, forbidden := range []string{"old noisy", "docs +clean", "changed clean skill", "docs clean-list"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("view leaked non-candidate fact %q: %s", forbidden, data)
		}
	}
}

func TestInputViewSummarizesBroadChangedCommandSurface(t *testing.T) {
	f := broadChangedFacts(434, 44)

	view := BuildInputView(f)
	if view.ChangedSummary.Commands != 434 || view.ChangedSummary.Outputs != 44 {
		t.Fatalf("changed summary = %#v", view.ChangedSummary)
	}
	if len(view.Commands) != 0 || len(view.Outputs) != 0 {
		t.Fatalf("broad clean surface leaked details: commands=%d outputs=%d", len(view.Commands), len(view.Outputs))
	}

	messages := BuildPrompt(f)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	if got := len(messages[1].Content); got > 8192 {
		t.Fatalf("prompt user content bytes = %d, want <= 8192", got)
	}
	if strings.Contains(messages[1].Content, "service command 433") {
		t.Fatalf("prompt leaked broad command details: %s", messages[1].Content)
	}
}

func TestInputViewKeepsSemanticCandidateInsideBroadChangedSurface(t *testing.T) {
	f := broadChangedFacts(200, 20)
	f.Commands[137].NameConflictsExisting = true
	f.Outputs[11].HasDefaultLimit = false

	view := BuildInputView(f)
	if got := singleRef(t, view.Commands); got != "facts.commands[137]" {
		t.Fatalf("command ref = %q, want facts.commands[137]", got)
	}
	if len(view.Outputs) != 0 {
		t.Fatalf("outputs len = %d, want 0 without reject diagnostics", len(view.Outputs))
	}
}

func TestInputViewOmitsVerboseOutputFields(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Outputs: []facts.OutputFact{{
			Command:          "base +record-list",
			Domain:           "base",
			Changed:          true,
			Source:           "shortcut",
			Fields:           []string{"items", "has_more", strings.Repeat("verbose_output_field_", 200)},
			IsList:           true,
			HasDefaultLimit:  true,
			HasDecisionField: false,
		}},
		Diagnostics: []facts.DiagnosticFact{{
			Rule:        "default_output_contract",
			Action:      report.ActionReject,
			File:        "command-manifest",
			CommandPath: "base +record-list",
		}},
	}

	view := BuildInputView(f)
	if got := singleRef(t, view.Outputs); got != "facts.outputs[0]" {
		t.Fatalf("output ref = %q, want facts.outputs[0]", got)
	}
	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal view: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "verbose_output_field_") || strings.Contains(text, "fields") {
		t.Fatalf("semantic view leaked verbose output fields: %s", text)
	}
	if !strings.Contains(text, `"has_decision_field":false`) {
		t.Fatalf("semantic view should make missing decision field explicit: %s", text)
	}
}

func TestBuildPromptKeepsManyOutputCandidatesWithinRequestLimit(t *testing.T) {
	f := broadOutputCandidateFacts(40)
	for i := 0; i < 80; i++ {
		f.Commands = append(f.Commands, facts.CommandFact{
			Path:    "shortcut list " + strconv.Itoa(i),
			Domain:  "shortcut",
			Changed: true,
			Source:  "shortcut",
			Flags:   []string{strings.Repeat("verbose_flag_", 100)},
		})
		f.Skills = append(f.Skills, facts.SkillFact{
			SourceFile:  "skills/lark-shortcut/SKILL.md",
			Line:        i + 1,
			Raw:         strings.Repeat("verbose skill guidance ", 100),
			CommandPath: "shortcut list " + strconv.Itoa(i),
			Changed:     true,
		})
	}
	f.Diagnostics = append(f.Diagnostics, facts.DiagnosticFact{
		Rule:        "default_output",
		Action:      report.ActionWarning,
		File:        "command-manifest",
		Message:     "shortcut list 1 looks like a list command without an explicit default limit flag",
		CommandPath: "shortcut list 1",
	})

	messages := BuildPrompt(f)
	var view InputView
	if err := json.Unmarshal([]byte(messages[1].Content), &view); err != nil {
		t.Fatalf("prompt user content is not input view JSON: %v", err)
	}
	if len(view.Commands) != 0 || len(view.Skills) != 0 {
		t.Fatalf("default-output view leaked unrelated context: commands=%d skills=%d", len(view.Commands), len(view.Skills))
	}
	if len(view.Outputs) != 0 {
		t.Fatalf("default-output warnings should not enter semantic view without reject diagnostics: outputs=%d", len(view.Outputs))
	}
	if got := len(messages[1].Content); got > 16*1024 {
		t.Fatalf("prompt user content bytes = %d, want <= 16384", got)
	}
	if strings.Contains(messages[1].Content, "verbose_output_field_") ||
		strings.Contains(messages[1].Content, "verbose_flag_") ||
		strings.Contains(messages[1].Content, "verbose skill guidance") {
		t.Fatalf("prompt leaked verbose output fields: %s", messages[1].Content)
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

func TestInputViewDropsUnselectedLabelDiagnostics(t *testing.T) {
	f := facts.Facts{
		SchemaVersion: 1,
		Commands: []facts.CommandFact{{
			Path:    "drive +task_result",
			Changed: true,
			Source:  "shortcut",
		}},
		Diagnostics: []facts.DiagnosticFact{{
			Rule:        "command_naming",
			Action:      report.ActionLabel,
			File:        "command-manifest",
			Message:     "drive +task_result has non-kebab-case command segments",
			CommandPath: "drive +task_result",
		}},
	}

	view := BuildInputView(f)
	if len(view.Commands) != 0 {
		t.Fatalf("commands len = %d, want 0 for label-only diagnostic", len(view.Commands))
	}
	if len(view.Diagnostics) != 0 {
		t.Fatalf("diagnostics len = %d, want 0 for label-only diagnostic", len(view.Diagnostics))
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

func broadChangedFacts(commands, outputs int) facts.Facts {
	f := facts.Facts{SchemaVersion: 1}
	for i := 0; i < commands; i++ {
		f.Commands = append(f.Commands, facts.CommandFact{
			Path:    "service command " + strconv.Itoa(i),
			Domain:  "service",
			Changed: true,
			Source:  "service",
			Flags:   []string{"tenant_key", "page_token", "page_size", "user_id_type"},
		})
	}
	for i := 0; i < outputs; i++ {
		f.Outputs = append(f.Outputs, facts.OutputFact{
			Command:          "service command " + strconv.Itoa(i),
			Domain:           "service",
			Changed:          true,
			Source:           "service",
			Fields:           []string{"items", "has_more", "page_token"},
			IsList:           true,
			HasDefaultLimit:  true,
			HasDecisionField: true,
		})
	}
	return f
}

func broadOutputCandidateFacts(outputs int) facts.Facts {
	f := facts.Facts{SchemaVersion: 1}
	for i := 0; i < outputs; i++ {
		f.Outputs = append(f.Outputs, facts.OutputFact{
			Command:          "shortcut list " + strconv.Itoa(i),
			Domain:           "shortcut",
			Changed:          true,
			Source:           "shortcut",
			Fields:           []string{"items", "has_more", strings.Repeat("verbose_output_field_", 200)},
			IsList:           true,
			HasDefaultLimit:  i%2 == 0,
			HasDecisionField: false,
		})
	}
	return f
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
