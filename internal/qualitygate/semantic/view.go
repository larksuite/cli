// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

type InputView struct {
	SchemaVersion        int                    `json:"schema_version"`
	ChangedSummary       ChangedSummary         `json:"changed_summary"`
	RuleSummary          []RuleSummaryItem      `json:"rule_summary,omitempty"`
	Commands             []CommandInput         `json:"commands,omitempty"`
	Skills               []SkillInput           `json:"skills,omitempty"`
	SkillQuality         []SkillQualityInput    `json:"skill_quality,omitempty"`
	Errors               []ErrorInput           `json:"errors,omitempty"`
	Outputs              []OutputInput          `json:"outputs,omitempty"`
	Examples             []ExampleInput         `json:"examples,omitempty"`
	PublicContentLeakage []PublicContentInput   `json:"public_content_leakage,omitempty"`
	Diagnostics          []facts.DiagnosticFact `json:"diagnostics,omitempty"`
}

type ChangedSummary struct {
	Commands      int      `json:"commands,omitempty"`
	Skills        int      `json:"skills,omitempty"`
	SkillQuality  int      `json:"skill_quality,omitempty"`
	Errors        int      `json:"errors,omitempty"`
	Outputs       int      `json:"outputs,omitempty"`
	Examples      int      `json:"examples,omitempty"`
	PublicContent int      `json:"public_content,omitempty"`
	Domains       []string `json:"domains,omitempty"`
	Sources       []string `json:"sources,omitempty"`
}

type RuleSummaryItem struct {
	Rule   string        `json:"rule"`
	Action report.Action `json:"action"`
	Count  int           `json:"count"`
}

type CommandInput struct {
	FactRef string `json:"fact_ref"`
	facts.CommandFact
}

func (i CommandInput) ref() string { return i.FactRef }

type SkillInput struct {
	FactRef string `json:"fact_ref"`
	facts.SkillFact
}

func (i SkillInput) ref() string { return i.FactRef }

type SkillQualityInput struct {
	FactRef string `json:"fact_ref"`
	facts.SkillQualityFact
}

type ErrorInput struct {
	FactRef string `json:"fact_ref"`
	facts.ErrorFact
}

func (i ErrorInput) ref() string { return i.FactRef }

type OutputInput struct {
	FactRef          string `json:"fact_ref"`
	Command          string `json:"command"`
	Domain           string `json:"domain,omitempty"`
	Changed          bool   `json:"changed,omitempty"`
	Source           string `json:"source,omitempty"`
	IsList           bool   `json:"is_list"`
	HasDefaultLimit  bool   `json:"has_default_limit"`
	HasDecisionField bool   `json:"has_decision_field"`
}

func (i OutputInput) ref() string { return i.FactRef }

type ExampleInput struct {
	FactRef string `json:"fact_ref"`
	facts.CommandExample
}

type PublicContentInput struct {
	FactRef string `json:"fact_ref"`
	facts.PublicContentFact
}

func (v InputView) HasReviewableFacts() bool {
	return len(v.Commands) > 0 ||
		len(v.Skills) > 0 ||
		len(v.SkillQuality) > 0 ||
		len(v.Errors) > 0 ||
		len(v.Outputs) > 0 ||
		len(v.Examples) > 0 ||
		len(v.PublicContentLeakage) > 0 ||
		len(v.Diagnostics) > 0
}

func BuildInputView(f facts.Facts) InputView {
	selected := newInputSelection(f)
	selected.addChangedReviewCandidates()

	var viewDiagnostics []facts.DiagnosticFact
	for _, diag := range f.Diagnostics {
		if !semanticDiagnosticRule(diag.Rule) {
			continue
		}
		context := selected.diagnosticContext(diag)
		if !includeDiagnosticInView(diag, selected, context) {
			continue
		}
		viewDiagnostics = append(viewDiagnostics, diag)
		selected.merge(context)
	}

	return InputView{
		SchemaVersion:        f.SchemaVersion,
		ChangedSummary:       changedSummary(f),
		RuleSummary:          ruleSummary(f.Diagnostics),
		Commands:             selected.commandInputs(),
		Skills:               selected.skillInputs(),
		SkillQuality:         selected.skillQualityInputs(),
		Errors:               selected.errorInputs(),
		Outputs:              selected.outputInputs(),
		Examples:             selected.exampleInputs(),
		PublicContentLeakage: selected.publicContentInputs(),
		Diagnostics:          viewDiagnostics,
	}
}

func (s *inputSelection) addChangedReviewCandidates() {
	for i, cmd := range s.f.Commands {
		if cmd.Changed && commandReviewCandidate(cmd) {
			s.commands[i] = true
		}
	}
	for i, skill := range s.f.Skills {
		if skill.Changed && skillReviewCandidate(skill) {
			s.skills[i] = true
		}
	}
	for i, errFact := range s.f.Errors {
		if errFact.Changed && errorReviewCandidate(errFact) {
			s.errors[i] = true
		}
	}
	for i, output := range s.f.Outputs {
		if output.Changed && outputReviewCandidate(output) {
			s.outputs[i] = true
		}
	}
	for i, item := range s.f.PublicContent {
		if publicContentReviewCandidate(item) {
			s.publicContent[i] = true
		}
	}
}

func commandReviewCandidate(cmd facts.CommandFact) bool {
	return cmd.NameConflictsExisting || cmd.FlagAliasConflict
}

func skillReviewCandidate(skill facts.SkillFact) bool {
	return skill.ReferencesInvalidCommand
}

func errorReviewCandidate(errFact facts.ErrorFact) bool {
	return errFact.Boundary && errFact.RequiredHint && errFact.HintActionCount == 0
}

func outputReviewCandidate(_ facts.OutputFact) bool {
	// default_output is observe-first in the current rollout; reject diagnostics add exact output context.
	return false
}

func publicContentReviewCandidate(item facts.PublicContentFact) bool {
	return item.Rule == "public_content_semantic_candidate"
}

type inputSelection struct {
	f             facts.Facts
	commands      []bool
	skills        []bool
	skillQuality  []bool
	errors        []bool
	outputs       []bool
	examples      []bool
	publicContent []bool
}

func newInputSelection(f facts.Facts) *inputSelection {
	return &inputSelection{
		f:             f,
		commands:      make([]bool, len(f.Commands)),
		skills:        make([]bool, len(f.Skills)),
		skillQuality:  make([]bool, len(f.SkillQuality)),
		errors:        make([]bool, len(f.Errors)),
		outputs:       make([]bool, len(f.Outputs)),
		examples:      make([]bool, len(f.Examples)),
		publicContent: make([]bool, len(f.PublicContent)),
	}
}

func (s *inputSelection) diagnosticContext(diag facts.DiagnosticFact) *inputSelection {
	out := newInputSelection(s.f)
	switch {
	case diag.Rule == "command_naming" || diag.Rule == "flag_naming":
		s.addDiagnosticCommands(out, diag)
	case strings.HasPrefix(diag.Rule, "default_output"):
		s.addDiagnosticOutputs(out, diag)
	case strings.HasPrefix(diag.Rule, "skill_"):
		s.addDiagnosticSkills(out, diag)
		s.addDiagnosticSkillQuality(out, diag)
		s.addDiagnosticExamples(out, diag)
	case strings.HasPrefix(diag.Rule, "example_dry_run"):
		s.addDiagnosticExamples(out, diag)
	case diag.Rule == "no_bare_helper_error":
		s.addDiagnosticErrors(out, diag)
	case strings.HasPrefix(diag.Rule, "public_content_"):
		s.addDiagnosticPublicContent(out, diag)
	}
	return out
}

func (s *inputSelection) addDiagnosticCommands(out *inputSelection, diag facts.DiagnosticFact) {
	for i, cmd := range s.f.Commands {
		if diagnosticCommandMatches(diag, cmd.Path, cmd.CanonicalPath) ||
			diagnosticMentions(diag, cmd.Path) ||
			diagnosticMentions(diag, cmd.CanonicalPath) {
			out.commands[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticSkills(out *inputSelection, diag facts.DiagnosticFact) {
	for i, skill := range s.f.Skills {
		if diagnosticLocationMatches(diag.File, diag.Line, skill.SourceFile, skill.Line) ||
			diagnosticCommandMatches(diag, skill.CommandPath) ||
			diagnosticMentions(diag, skill.CommandPath) {
			out.skills[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticSkillQuality(out *inputSelection, diag facts.DiagnosticFact) {
	for i, skill := range s.f.SkillQuality {
		if samePath(diag.File, skill.SourceFile) {
			out.skillQuality[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticErrors(out *inputSelection, diag facts.DiagnosticFact) {
	for i, errFact := range s.f.Errors {
		if diagnosticLocationMatches(diag.File, diag.Line, errFact.File, errFact.Line) ||
			diagnosticCommandMatches(diag, errFact.CommandPath, errFact.Command) ||
			diagnosticMentions(diag, errFact.CommandPath) ||
			diagnosticMentions(diag, errFact.Command) {
			out.errors[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticOutputs(out *inputSelection, diag facts.DiagnosticFact) {
	for i, output := range s.f.Outputs {
		if diagnosticCommandMatches(diag, output.Command) ||
			diagnosticMentions(diag, output.Command) {
			out.outputs[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticExamples(out *inputSelection, diag facts.DiagnosticFact) {
	for i, example := range s.f.Examples {
		if diagnosticLocationMatches(diag.File, diag.Line, example.SourceFile, example.Line) ||
			diagnosticCommandMatches(diag, example.CommandPath) ||
			diagnosticMentions(diag, example.CommandPath) {
			out.examples[i] = true
		}
	}
}

func (s *inputSelection) addDiagnosticPublicContent(out *inputSelection, diag facts.DiagnosticFact) {
	for i, item := range s.f.PublicContent {
		if diagnosticLocationMatches(diag.File, diag.Line, item.File, item.Line) ||
			diag.Rule == item.Rule {
			out.publicContent[i] = true
		}
	}
}

func includeDiagnosticInView(diag facts.DiagnosticFact, selected, context *inputSelection) bool {
	if diag.Action == report.ActionReject {
		return true
	}
	return selected.intersects(context)
}

func (s *inputSelection) merge(other *inputSelection) {
	mergeSelections(s.commands, other.commands)
	mergeSelections(s.skills, other.skills)
	mergeSelections(s.skillQuality, other.skillQuality)
	mergeSelections(s.errors, other.errors)
	mergeSelections(s.outputs, other.outputs)
	mergeSelections(s.examples, other.examples)
	mergeSelections(s.publicContent, other.publicContent)
}

func (s *inputSelection) intersects(other *inputSelection) bool {
	return selectionsIntersect(s.commands, other.commands) ||
		selectionsIntersect(s.skills, other.skills) ||
		selectionsIntersect(s.skillQuality, other.skillQuality) ||
		selectionsIntersect(s.errors, other.errors) ||
		selectionsIntersect(s.outputs, other.outputs) ||
		selectionsIntersect(s.examples, other.examples) ||
		selectionsIntersect(s.publicContent, other.publicContent)
}

func (s *inputSelection) commandInputs() []CommandInput {
	out := make([]CommandInput, 0, countSelected(s.commands))
	for i, ok := range s.commands {
		if ok {
			out = append(out, CommandInput{FactRef: factRef("commands", i), CommandFact: s.f.Commands[i]})
		}
	}
	return out
}

func (s *inputSelection) skillInputs() []SkillInput {
	out := make([]SkillInput, 0, countSelected(s.skills))
	for i, ok := range s.skills {
		if ok {
			out = append(out, SkillInput{FactRef: factRef("skills", i), SkillFact: s.f.Skills[i]})
		}
	}
	return out
}

func (s *inputSelection) skillQualityInputs() []SkillQualityInput {
	out := make([]SkillQualityInput, 0, countSelected(s.skillQuality))
	for i, ok := range s.skillQuality {
		if ok {
			out = append(out, SkillQualityInput{FactRef: factRef("skill_quality", i), SkillQualityFact: s.f.SkillQuality[i]})
		}
	}
	return out
}

func (s *inputSelection) errorInputs() []ErrorInput {
	out := make([]ErrorInput, 0, countSelected(s.errors))
	for i, ok := range s.errors {
		if ok {
			out = append(out, ErrorInput{FactRef: factRef("errors", i), ErrorFact: s.f.Errors[i]})
		}
	}
	return out
}

func (s *inputSelection) outputInputs() []OutputInput {
	out := make([]OutputInput, 0, countSelected(s.outputs))
	for i, ok := range s.outputs {
		if ok {
			output := s.f.Outputs[i]
			out = append(out, OutputInput{
				FactRef:          factRef("outputs", i),
				Command:          output.Command,
				Domain:           output.Domain,
				Changed:          output.Changed,
				Source:           output.Source,
				IsList:           output.IsList,
				HasDefaultLimit:  output.HasDefaultLimit,
				HasDecisionField: output.HasDecisionField,
			})
		}
	}
	return out
}

func (s *inputSelection) exampleInputs() []ExampleInput {
	out := make([]ExampleInput, 0, countSelected(s.examples))
	for i, ok := range s.examples {
		if ok {
			out = append(out, ExampleInput{FactRef: factRef("examples", i), CommandExample: s.f.Examples[i]})
		}
	}
	return out
}

func (s *inputSelection) publicContentInputs() []PublicContentInput {
	out := make([]PublicContentInput, 0, countSelected(s.publicContent))
	for i, ok := range s.publicContent {
		if ok {
			out = append(out, PublicContentInput{FactRef: factRef("public_content", i), PublicContentFact: s.f.PublicContent[i]})
		}
	}
	return out
}

func changedSummary(f facts.Facts) ChangedSummary {
	domains := map[string]bool{}
	sources := map[string]bool{}
	var out ChangedSummary
	for _, cmd := range f.Commands {
		if !cmd.Changed {
			continue
		}
		out.Commands++
		addNonEmpty(domains, cmd.Domain)
		addNonEmpty(sources, cmd.Source)
	}
	for _, skill := range f.Skills {
		if !skill.Changed {
			continue
		}
		out.Skills++
		addNonEmpty(domains, skill.Domain)
		addNonEmpty(sources, skill.Source)
	}
	for _, skill := range f.SkillQuality {
		if !skill.Changed {
			continue
		}
		out.SkillQuality++
		addNonEmpty(domains, skill.Domain)
	}
	for _, errFact := range f.Errors {
		if !errFact.Changed {
			continue
		}
		out.Errors++
		addNonEmpty(domains, errFact.Domain)
		addNonEmpty(sources, errFact.Source)
	}
	for _, output := range f.Outputs {
		if !output.Changed {
			continue
		}
		out.Outputs++
		addNonEmpty(domains, output.Domain)
		addNonEmpty(sources, output.Source)
	}
	for _, example := range f.Examples {
		if !example.Changed {
			continue
		}
		out.Examples++
		addNonEmpty(domains, example.Domain)
		addNonEmpty(sources, example.Source)
	}
	for _, item := range f.PublicContent {
		out.PublicContent++
		addNonEmpty(sources, item.Source)
	}
	out.Domains = sortedViewSetKeys(domains)
	out.Sources = sortedViewSetKeys(sources)
	return out
}

func ruleSummary(diags []facts.DiagnosticFact) []RuleSummaryItem {
	counts := map[string]int{}
	actions := map[string]report.Action{}
	for _, diag := range diags {
		key := string(diag.Action) + "\x00" + diag.Rule
		counts[key]++
		actions[key] = diag.Action
	}
	keys := sortedKeysInt(counts)
	out := make([]RuleSummaryItem, 0, len(keys))
	for _, key := range keys {
		_, rule, _ := strings.Cut(key, "\x00")
		out = append(out, RuleSummaryItem{
			Rule:   rule,
			Action: actions[key],
			Count:  counts[key],
		})
	}
	return out
}

func semanticDiagnosticRule(rule string) bool {
	return rule == "command_naming" ||
		rule == "flag_naming" ||
		strings.HasPrefix(rule, "default_output") ||
		strings.HasPrefix(rule, "skill_") ||
		strings.HasPrefix(rule, "example_dry_run") ||
		rule == "no_bare_helper_error" ||
		strings.HasPrefix(rule, "public_content_")
}

func diagnosticCommandMatches(diag facts.DiagnosticFact, values ...string) bool {
	if diag.CommandPath == "" {
		return false
	}
	for _, value := range values {
		if value != "" && diag.CommandPath == value {
			return true
		}
	}
	return false
}

func diagnosticLocationMatches(diagFile string, diagLine int, factFile string, factLine int) bool {
	if !samePath(diagFile, factFile) {
		return false
	}
	return diagLine == 0 || factLine == 0 || diagLine == factLine
}

func diagnosticMentions(diag facts.DiagnosticFact, value string) bool {
	if value == "" {
		return false
	}
	return strings.Contains(diag.Message, value) ||
		strings.Contains(diag.Suggestion, value)
}

func samePath(a, b string) bool {
	return normalizeViewPath(a) == normalizeViewPath(b)
}

func normalizeViewPath(path string) string {
	return strings.TrimPrefix(strings.ReplaceAll(path, "\\", "/"), "./")
}

func factRef(kind string, idx int) string {
	return fmt.Sprintf("facts.%s[%d]", kind, idx)
}

func addNonEmpty(set map[string]bool, value string) {
	if value != "" {
		set[value] = true
	}
}

func countSelected(items []bool) int {
	var count int
	for _, item := range items {
		if item {
			count++
		}
	}
	return count
}

func mergeSelections(dst, src []bool) {
	for i := range dst {
		dst[i] = dst[i] || src[i]
	}
}

func selectionsIntersect(a, b []bool) bool {
	for i := range a {
		if a[i] && b[i] {
			return true
		}
	}
	return false
}

func sortedViewSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysInt(set map[string]int) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
