// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package facts

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

type Facts struct {
	SchemaVersion int                 `json:"schema_version"`
	Commands      []CommandFact       `json:"commands,omitempty"`
	Skills        []SkillFact         `json:"skills,omitempty"`
	SkillQuality  []SkillQualityFact  `json:"skill_quality,omitempty"`
	Errors        []ErrorFact         `json:"errors,omitempty"`
	Outputs       []OutputFact        `json:"outputs,omitempty"`
	Examples      []CommandExample    `json:"examples,omitempty"`
	PublicContent []PublicContentFact `json:"public_content,omitempty"`
	Diagnostics   []DiagnosticFact    `json:"diagnostics,omitempty"`
}

type CommandFact struct {
	Path                  string           `json:"path"`
	CanonicalPath         string           `json:"canonical_path,omitempty"`
	Domain                string           `json:"domain,omitempty"`
	Changed               bool             `json:"changed,omitempty"`
	Source                string           `json:"source"`
	Generated             bool             `json:"generated,omitempty"`
	Flags                 []string         `json:"flags,omitempty"`
	Examples              []CommandExample `json:"examples,omitempty"`
	LegacyNaming          bool             `json:"legacy_naming,omitempty"`
	NameConflictsExisting bool             `json:"name_conflicts_existing,omitempty"`
	FlagAliasConflict     bool             `json:"flag_alias_conflict,omitempty"`
}

type SkillFact struct {
	SourceFile               string `json:"source_file"`
	Line                     int    `json:"line"`
	Raw                      string `json:"raw"`
	CommandPath              string `json:"command_path,omitempty"`
	Domain                   string `json:"domain,omitempty"`
	Changed                  bool   `json:"changed,omitempty"`
	Source                   string `json:"source,omitempty"`
	ReferencesInvalidCommand bool   `json:"references_invalid_command"`
	DestructiveWithoutGuard  bool   `json:"destructive_without_guard,omitempty"`
	ScopeConflict            bool   `json:"scope_conflict,omitempty"`
}

type SkillQualityFact struct {
	SourceFile         string `json:"source_file"`
	Domain             string `json:"domain,omitempty"`
	Changed            bool   `json:"changed,omitempty"`
	WordCount          int    `json:"word_count"`
	CriticalCount      int    `json:"critical_count"`
	DescriptionLength  int    `json:"description_length"`
	CriticalOverBudget bool   `json:"critical_over_budget,omitempty"`
}

type CommandExample struct {
	Raw          string `json:"raw"`
	SourceFile   string `json:"source_file"`
	Line         int    `json:"line"`
	CommandPath  string `json:"command_path,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Changed      bool   `json:"changed,omitempty"`
	Source       string `json:"source,omitempty"`
	Executable   bool   `json:"executable"`
	SkipReason   string `json:"skip_reason,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	StdoutBytes  int    `json:"stdout_bytes,omitempty"`
	APICallCount int    `json:"api_call_count,omitempty"`
	// Reserved for future request-shape producers; v1 does not emit it.
	ExpectedRequest *DryRunRequest `json:"expected_request,omitempty"`
	DryRun          *DryRunRequest `json:"dry_run,omitempty"`
}

type ErrorFact struct {
	File                string `json:"file"`
	Line                int    `json:"line"`
	Command             string `json:"command,omitempty"`
	CommandPath         string `json:"command_path,omitempty"`
	Domain              string `json:"domain,omitempty"`
	Changed             bool   `json:"changed,omitempty"`
	Source              string `json:"source,omitempty"`
	Boundary            bool   `json:"boundary"`
	UsesStructuredError bool   `json:"uses_structured_error"`
	HasHint             bool   `json:"has_hint"`
	HintActionCount     int    `json:"hint_action_count"`
	RequiredHint        bool   `json:"required_hint"`
	Code                string `json:"code,omitempty"`
	Message             string `json:"message,omitempty"`
	Hint                string `json:"hint,omitempty"`
	Retryable           bool   `json:"retryable"`
}

type OutputFact struct {
	Command          string   `json:"command"`
	Domain           string   `json:"domain,omitempty"`
	Changed          bool     `json:"changed,omitempty"`
	Source           string   `json:"source,omitempty"`
	Fields           []string `json:"fields,omitempty"`
	IsList           bool     `json:"is_list,omitempty"`
	HasDefaultLimit  bool     `json:"has_default_limit,omitempty"`
	HasFieldSelector bool     `json:"has_field_selector,omitempty"`
	HasDecisionField bool     `json:"has_decision_field,omitempty"`
}

type PublicContentFact struct {
	Rule       string        `json:"rule"`
	Action     report.Action `json:"action"`
	File       string        `json:"file"`
	Line       int           `json:"line"`
	Source     string        `json:"source,omitempty"`
	Excerpt    string        `json:"excerpt,omitempty"`
	Message    string        `json:"message,omitempty"`
	Suggestion string        `json:"suggestion,omitempty"`
}

type DryRunRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Query  map[string][]string `json:"query,omitempty"`
	Params map[string]any      `json:"params,omitempty"`
	Body   json.RawMessage     `json:"body,omitempty"`
}

type DiagnosticFact struct {
	Rule        string        `json:"rule"`
	Action      report.Action `json:"action"`
	File        string        `json:"file"`
	Line        int           `json:"line"`
	Message     string        `json:"message"`
	Suggestion  string        `json:"suggestion,omitempty"`
	SubjectType string        `json:"subject_type,omitempty"`
	CommandPath string        `json:"command_path,omitempty"`
	FlagName    string        `json:"flag_name,omitempty"`
}

func DiagnosticsFromReport(ds []report.Diagnostic) []DiagnosticFact {
	if len(ds) == 0 {
		return nil
	}
	out := make([]DiagnosticFact, 0, len(ds))
	for _, d := range ds {
		out = append(out, DiagnosticFact{
			Rule:        d.Rule,
			Action:      d.Action,
			File:        d.File,
			Line:        d.Line,
			Message:     d.Message,
			Suggestion:  d.Suggestion,
			SubjectType: d.SubjectType,
			CommandPath: d.CommandPath,
			FlagName:    d.FlagName,
		})
	}
	return out
}

func Build(m manifest.Manifest, skillFacts []SkillFact, skillQualityFacts []SkillQualityFact, errorFacts []ErrorFact, exampleFacts []CommandExample, outputFacts []OutputFact, diags []report.Diagnostic, changedFiles ...map[string]bool) Facts {
	return BuildWithCommandLookup(m, m, skillFacts, skillQualityFacts, errorFacts, exampleFacts, outputFacts, diags, changedFiles...)
}

func BuildWithCommandLookup(m manifest.Manifest, commandLookup manifest.Manifest, skillFacts []SkillFact, skillQualityFacts []SkillQualityFact, errorFacts []ErrorFact, exampleFacts []CommandExample, outputFacts []OutputFact, diags []report.Diagnostic, changedFiles ...map[string]bool) Facts {
	naming := commandNamingFacts(m, diags)
	changed := map[string]bool{}
	if len(changedFiles) > 0 && changedFiles[0] != nil {
		changed = changedFiles[0]
	}
	commandChanges := commandChangeScopeFromFiles(changed)
	commandMeta := commandScopeIndex(commandLookup)
	handCommandMeta := commandScopeIndex(m)
	changedCommands := map[string]bool{}
	commandFacts := make([]CommandFact, 0, len(m.Commands))
	for _, cmd := range m.Commands {
		flags := make([]string, 0, len(cmd.Flags))
		for _, fl := range cmd.Flags {
			flags = append(flags, fl.Name)
		}
		namingFact := naming[cmd.Path]
		commandChanged := commandChanges.changed(cmd)
		changedCommands[cmd.Path] = commandChanged
		if cmd.CanonicalPath != "" {
			changedCommands[cmd.CanonicalPath] = commandChanged
		}
		commandFacts = append(commandFacts, CommandFact{
			Path:                  cmd.Path,
			CanonicalPath:         cmd.CanonicalPath,
			Domain:                cmd.Domain,
			Changed:               commandChanged,
			Source:                string(cmd.Source),
			Generated:             cmd.Generated,
			Flags:                 flags,
			LegacyNaming:          namingFact.LegacyNaming,
			NameConflictsExisting: namingFact.NameConflictsExisting,
			FlagAliasConflict:     namingFact.FlagAliasConflict,
		})
	}
	enrichSkillFacts(skillFacts, commandMeta, changed)
	enrichSkillQualityFacts(skillQualityFacts, commandMeta.domains, changed)
	enrichErrorFacts(errorFacts, commandMeta, changed)
	enrichExampleFacts(exampleFacts, commandMeta, changed)
	enrichOutputFacts(outputFacts, handCommandMeta, changedCommands)
	return Facts{
		SchemaVersion: 1,
		Commands:      commandFacts,
		Skills:        skillFacts,
		SkillQuality:  skillQualityFacts,
		Errors:        errorFacts,
		Outputs:       outputFacts,
		Examples:      exampleFacts,
		Diagnostics:   DiagnosticsFromReport(diags),
	}
}

func WithPublicContent(f Facts, publicContent []PublicContentFact) Facts {
	f.PublicContent = publicContent
	return f
}

type commandScope struct {
	Domain string
	Source string
}

type commandScopeLookup struct {
	byPath  map[string]commandScope
	domains map[string]bool
}

func commandScopeIndex(m manifest.Manifest) commandScopeLookup {
	lookup := commandScopeLookup{
		byPath:  map[string]commandScope{},
		domains: map[string]bool{},
	}
	for _, cmd := range m.Commands {
		scope := commandScope{Domain: cmd.Domain, Source: string(cmd.Source)}
		lookup.byPath[cmd.Path] = scope
		if cmd.CanonicalPath != "" {
			lookup.byPath[cmd.CanonicalPath] = scope
		}
		if cmd.Domain != "" {
			lookup.domains[cmd.Domain] = true
		}
	}
	return lookup
}

func enrichSkillFacts(items []SkillFact, lookup commandScopeLookup, changed map[string]bool) {
	for i := range items {
		items[i].SourceFile = normalizeFactPath(items[i].SourceFile)
		items[i].Changed = changed[items[i].SourceFile]
		if scope, ok := lookup.byPath[items[i].CommandPath]; ok {
			items[i].Domain = scope.Domain
			items[i].Source = scope.Source
			continue
		}
		items[i].Domain = domainFromSkillPath(items[i].SourceFile, lookup.domains)
	}
}

func enrichSkillQualityFacts(items []SkillQualityFact, knownDomains map[string]bool, changed map[string]bool) {
	for i := range items {
		items[i].SourceFile = normalizeFactPath(items[i].SourceFile)
		items[i].Changed = changed[items[i].SourceFile]
		items[i].Domain = domainFromSkillPath(items[i].SourceFile, knownDomains)
	}
}

func enrichErrorFacts(items []ErrorFact, lookup commandScopeLookup, changed map[string]bool) {
	for i := range items {
		items[i].File = normalizeFactPath(items[i].File)
		items[i].Changed = changed[items[i].File]
		if items[i].CommandPath == "" {
			items[i].CommandPath = items[i].Command
		}
		if scope, ok := lookup.byPath[items[i].CommandPath]; ok {
			items[i].Domain = scope.Domain
			items[i].Source = scope.Source
		}
	}
}

func enrichExampleFacts(items []CommandExample, lookup commandScopeLookup, changed map[string]bool) {
	for i := range items {
		items[i].SourceFile = normalizeFactPath(items[i].SourceFile)
		items[i].Changed = changed[items[i].SourceFile]
		if scope, ok := lookup.byPath[items[i].CommandPath]; ok {
			items[i].Domain = scope.Domain
			items[i].Source = scope.Source
		}
	}
}

func enrichOutputFacts(items []OutputFact, lookup commandScopeLookup, changedCommands map[string]bool) {
	for i := range items {
		if scope, ok := lookup.byPath[items[i].Command]; ok {
			items[i].Domain = scope.Domain
			items[i].Source = scope.Source
		}
		items[i].Changed = changedCommands[items[i].Command]
	}
}

type commandChangeScope struct {
	global          bool
	service         bool
	shortcutGlobal  bool
	shortcutDomains map[string]bool
	builtinDomains  map[string]bool
}

func commandChangeScopeFromFiles(files map[string]bool) commandChangeScope {
	scope := commandChangeScope{
		shortcutDomains: map[string]bool{},
		builtinDomains:  map[string]bool{},
	}
	for file := range files {
		file = normalizeFactPath(file)
		switch {
		case isTopLevelCommandFile(file), file == "internal/cmdmeta/meta.go":
			scope.global = true
		case file == "cmd/service/service.go":
			scope.service = true
		case isTopLevelShortcutCommandFile(file), strings.HasPrefix(file, "shortcuts/common/"):
			scope.shortcutGlobal = true
		case strings.HasPrefix(file, "shortcuts/"):
			if domain := changedPathDomain(file, "shortcuts/"); domain != "" {
				scope.shortcutDomains[domain] = true
			}
		case strings.HasPrefix(file, "cmd/"):
			if domain := changedPathDomain(file, "cmd/"); domain != "" && domain != "service" {
				scope.builtinDomains[domain] = true
			}
		}
	}
	return scope
}

func (s commandChangeScope) changed(cmd manifest.Command) bool {
	if s.global {
		return true
	}
	switch cmd.Source {
	case manifest.SourceService:
		return s.service
	case manifest.SourceShortcut:
		return s.shortcutGlobal || s.shortcutDomains[cmd.Domain]
	case manifest.SourceBuiltin:
		return s.builtinDomains[firstCommandSegment(cmd.Path)]
	default:
		return false
	}
}

func changedPathDomain(file, prefix string) string {
	rest := strings.TrimPrefix(file, prefix)
	domain, _, ok := strings.Cut(rest, "/")
	if !ok || domain == "" || strings.HasSuffix(domain, ".go") {
		return ""
	}
	return normalizeCommandDomain(domain)
}

func firstCommandSegment(path string) string {
	first, _, _ := strings.Cut(path, " ")
	return first
}

func normalizeCommandDomain(domain string) string {
	switch domain {
	case "doc":
		return "docs"
	default:
		return domain
	}
}

func isTopLevelCommandFile(file string) bool {
	return strings.HasPrefix(file, "cmd/") &&
		strings.Count(file, "/") == 1 &&
		strings.HasSuffix(file, ".go") &&
		!strings.HasSuffix(file, "_test.go")
}

func isTopLevelShortcutCommandFile(file string) bool {
	return strings.HasPrefix(file, "shortcuts/") &&
		strings.Count(file, "/") == 1 &&
		strings.HasSuffix(file, ".go") &&
		!strings.HasSuffix(file, "_test.go")
}

func normalizeFactPath(value string) string {
	return strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
}

func domainFromSkillPath(file string, knownDomains map[string]bool) string {
	const prefix = "skills/lark-"
	if !strings.HasPrefix(file, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(file, prefix)
	domain, _, ok := strings.Cut(rest, "/")
	if !ok || domain == "" {
		return ""
	}
	domain = normalizeCommandDomain(domain)
	if knownDomains[domain] {
		return domain
	}
	return ""
}

func commandNamingFacts(m manifest.Manifest, diags []report.Diagnostic) map[string]CommandFact {
	out := map[string]CommandFact{}
	commands := append([]manifest.Command(nil), m.Commands...)
	sort.Slice(commands, func(i, j int) bool {
		return len(commands[i].Path) > len(commands[j].Path)
	})
	for _, diag := range diags {
		if diag.File != "command-manifest" || (diag.Rule != "command_naming" && diag.Rule != "flag_naming") {
			continue
		}
		cmd, ok := commandForNamingDiagnostic(commands, diag)
		if !ok {
			continue
		}
		fact := out[cmd.Path]
		switch diag.Action {
		case report.ActionLabel:
			fact.LegacyNaming = true
		case report.ActionReject:
			if diag.Rule == "flag_naming" {
				fact.FlagAliasConflict = true
			} else {
				fact.NameConflictsExisting = true
			}
		}
		out[cmd.Path] = fact
	}
	return out
}

func commandForNamingDiagnostic(commands []manifest.Command, diag report.Diagnostic) (manifest.Command, bool) {
	if diag.CommandPath != "" {
		for _, cmd := range commands {
			if cmd.Path == diag.CommandPath || cmd.CanonicalPath == diag.CommandPath {
				return cmd, true
			}
		}
		return manifest.Command{}, false
	}
	for _, cmd := range commands {
		if diag.Message == cmd.Path || strings.HasPrefix(diag.Message, cmd.Path+" ") {
			return cmd, true
		}
	}
	return manifest.Command{}, false
}
