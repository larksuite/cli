// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	qdiff "github.com/larksuite/cli/internal/qualitygate/diff"
	manifestexamples "github.com/larksuite/cli/internal/qualitygate/examples"
	"github.com/larksuite/cli/internal/qualitygate/facts"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/publiccontent"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
	"github.com/larksuite/cli/internal/vfs"
)

type Options struct {
	Repo                      string
	CLIBin                    string
	ChangedFrom               string
	FactsOut                  string
	ManifestPath              string
	CommandIndexPath          string
	PublicContentMetadataPath string
}

func Run(ctx context.Context, opts Options) ([]report.Diagnostic, facts.Facts, error) {
	m, err := readManifestInput(opts.ManifestPath, manifest.KindCommandManifest, "--manifest")
	if err != nil {
		return nil, facts.Facts{}, err
	}
	commandIndex, err := readManifestInput(opts.CommandIndexPath, manifest.KindCommandIndex, "--command-index")
	if err != nil {
		return nil, facts.Facts{}, err
	}
	if err := validateCommandIndexCoversManifest(m, commandIndex); err != nil {
		return nil, facts.Facts{}, err
	}
	changed, err := qdiff.ChangedFiles(ctx, opts.Repo, opts.ChangedFrom)
	if err != nil {
		return nil, facts.Facts{}, err
	}
	scope := qdiff.FromChangedFiles(changed)
	runNaming := shouldRunNaming(opts.ChangedFrom, scope)
	commandSurfaceAffected, _ := referenceCommandSurface(scope.Files)

	var diags []report.Diagnostic
	if runNaming {
		allow, allowDiags, err := LoadNamingAllowlist(opts.Repo)
		if err != nil {
			return nil, facts.Facts{}, err
		}
		diags = append(diags, allowDiags...)
		diags = append(diags, CheckNaming(m, allow)...)
	}

	examples, err := skillscan.Harvest(filepath.Join(opts.Repo, "skills"))
	if err != nil {
		return nil, facts.Facts{}, err
	}
	if opts.ChangedFrom != "" && !scope.Global && !commandSurfaceAffected {
		examples = skillscan.FilterExamples(examples, scope.AllSkills)
	}
	if opts.ChangedFrom == "" || scope.Global || runNaming {
		examples = append(examples, manifestexamples.FromManifest(m)...)
	}
	skillDocs, err := LoadSkillDocs(filepath.Join(opts.Repo, "skills"))
	if err != nil {
		return nil, facts.Facts{}, err
	}
	skillQualityDiags, skillQualityFacts := CheckSkillQuality(skillDocs)
	diags = append(diags, skillQualityDiags...)
	baseManifest, baseManifestComplete, err := loadBaseReferenceManifest(ctx, opts.Repo, opts.ChangedFrom)
	if err != nil {
		return nil, facts.Facts{}, err
	}
	refDiags, skillFacts := CheckReferencesWithPolicy(commandIndex, examples, ReferencePolicy{
		Incremental:            opts.ChangedFrom != "",
		ChangedFiles:           scope.Files,
		CommandSurfaceAffected: commandSurfaceAffected,
		BaseManifest:           baseManifest,
		BaseManifestComplete:   baseManifestComplete,
	})
	diags = append(diags, refDiags...)
	dryDiags, exampleFacts := RunDryRuns(ctx, opts.CLIBin, commandIndex, examples)
	diags = append(diags, dryDiags...)
	outputDiags, outputFacts := CheckDefaultOutput(m)
	diags = append(diags, outputDiags...)
	errorFacts, errorDiags, err := CollectRepoErrorFacts(opts.Repo, changed, opts.ChangedFrom != "")
	if err != nil {
		return nil, facts.Facts{}, err
	}
	if opts.ChangedFrom != "" {
		diags = append(diags, errorDiags...)
	}
	publicContent, err := publiccontent.Collect(ctx, publiccontent.Options{
		Repo:         opts.Repo,
		ChangedFrom:  opts.ChangedFrom,
		MetadataPath: opts.PublicContentMetadataPath,
	})
	if err != nil {
		return nil, facts.Facts{}, err
	}
	diags = append(diags, publicContentDiagnostics(publicContent)...)
	diags = filterPRDiagnostics(opts.Repo, opts.ChangedFrom, scope, m, diags)

	builtFacts := facts.BuildWithCommandLookup(m, commandIndex, skillFacts, skillQualityFacts, errorFacts, exampleFacts, outputFacts, diags, scope.Files)
	return diags, facts.WithPublicContent(builtFacts, publicContentFacts(publicContent)), nil
}

func publicContentDiagnostics(items []publiccontent.Finding) []report.Diagnostic {
	if len(items) == 0 {
		return nil
	}
	out := make([]report.Diagnostic, 0, len(items))
	for _, item := range items {
		if item.Rule == "public_content_semantic_candidate" {
			continue
		}
		out = append(out, report.Diagnostic{
			Rule:       item.Rule,
			Action:     item.Action,
			File:       item.File,
			Line:       item.Line,
			Message:    item.Message,
			Suggestion: item.Suggestion,
		})
	}
	return out
}

func publicContentFacts(items []publiccontent.Finding) []facts.PublicContentFact {
	if len(items) == 0 {
		return nil
	}
	out := make([]facts.PublicContentFact, 0, len(items))
	for _, item := range items {
		out = append(out, facts.PublicContentFact{
			Rule:       item.Rule,
			Action:     item.Action,
			File:       item.File,
			Line:       item.Line,
			Source:     item.Source,
			Excerpt:    item.Excerpt,
			Message:    item.Message,
			Suggestion: item.Suggestion,
		})
	}
	return out
}

func readManifestInput(path, kind, flag string) (manifest.Manifest, error) {
	if path == "" {
		return manifest.Manifest{}, fmt.Errorf("%s is required", flag)
	}
	m, err := manifest.ReadFile(path, kind)
	if err != nil {
		return manifest.Manifest{}, fmt.Errorf("%s: %w", flag, err)
	}
	return m, nil
}

func validateCommandIndexCoversManifest(m, commandIndex manifest.Manifest) error {
	byPath := make(map[string]manifest.Command, len(commandIndex.Commands))
	for _, cmd := range commandIndex.Commands {
		byPath[cmd.Path] = cmd
	}
	for _, cmd := range m.Commands {
		indexed, ok := byPath[cmd.Path]
		if !ok {
			return fmt.Errorf("--command-index is incomplete: missing %q from --manifest", cmd.Path)
		}
		wantCanonical := cmd.CanonicalPath
		if wantCanonical == "" {
			wantCanonical = manifest.CanonicalCommandPath(cmd.Path)
		}
		gotCanonical := indexed.CanonicalPath
		if gotCanonical == "" {
			gotCanonical = manifest.CanonicalCommandPath(indexed.Path)
		}
		if gotCanonical != wantCanonical {
			return fmt.Errorf("--command-index canonical path for %q is %q, want %q from --manifest", cmd.Path, gotCanonical, wantCanonical)
		}
	}
	return nil
}

func shouldRunNaming(changedFrom string, scope qdiff.Scope) bool {
	if changedFrom == "" || scope.Global {
		return true
	}
	if scope.Files["cmd/service/service.go"] ||
		scope.Files["shortcuts/common/runner.go"] ||
		scope.Files["internal/cmdmeta/meta.go"] {
		return true
	}
	return qdiff.ChangedUnder(scope.Files, "cmd/") ||
		qdiff.ChangedUnder(scope.Files, "shortcuts/")
}

func filterPRDiagnostics(repo, changedFrom string, scope qdiff.Scope, m manifest.Manifest, diags []report.Diagnostic) []report.Diagnostic {
	if changedFrom == "" || len(diags) == 0 {
		return diags
	}
	commandScope := diagnosticCommandScopeFromFiles(scope.Files)
	var out []report.Diagnostic
	for _, diag := range diags {
		if prDiagnosticRelevant(repo, scope.Files, commandScope, m, diag) {
			out = append(out, diag)
		}
	}
	return out
}

func prDiagnosticRelevant(repo string, changedFiles map[string]bool, commandScope diagnosticCommandScope, m manifest.Manifest, diag report.Diagnostic) bool {
	if strings.HasPrefix(diag.Rule, "public_content_") {
		return true
	}
	file := normalizeDiagnosticFile(repo, diag.File)
	if file != "" && changedFiles[file] {
		return true
	}
	if diag.File == "command-manifest" {
		if diag.CommandPath != "" {
			if cmd, ok := commandByPath(m, diag.CommandPath); ok {
				return commandScope.changed(cmd)
			}
			return false
		}
		if cmd, ok := commandForDiagnostic(m, diag.Message); ok {
			return commandScope.changed(cmd)
		}
		return false
	}
	if diag.Rule == "skill_command_reference" && diag.Action == report.ActionReject {
		return true
	}
	return false
}

func normalizeDiagnosticFile(repo, file string) string {
	if file == "" {
		return ""
	}
	if filepath.IsAbs(file) {
		if absRepo := absoluteRepoPath(repo); absRepo != "" {
			if rel, relErr := filepath.Rel(absRepo, file); relErr == nil && !strings.HasPrefix(rel, "..") {
				file = rel
			}
		}
	}
	return normalizeReferencePath(file)
}

func absoluteRepoPath(repo string) string {
	if repo == "" {
		return ""
	}
	if filepath.IsAbs(repo) {
		return filepath.Clean(repo)
	}
	wd, err := vfs.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(wd, repo)
}

func commandForDiagnostic(m manifest.Manifest, message string) (manifest.Command, bool) {
	commands := append([]manifest.Command(nil), m.Commands...)
	sort.Slice(commands, func(i, j int) bool {
		return len(commands[i].Path) > len(commands[j].Path)
	})
	for _, cmd := range commands {
		if message == cmd.Path || strings.HasPrefix(message, cmd.Path+" ") {
			return cmd, true
		}
	}
	return manifest.Command{}, false
}

func commandByPath(m manifest.Manifest, path string) (manifest.Command, bool) {
	for _, cmd := range m.Commands {
		if cmd.Path == path || cmd.CanonicalPath == path {
			return cmd, true
		}
	}
	return manifest.Command{}, false
}

type diagnosticCommandScope struct {
	service         bool
	shortcutGlobal  bool
	shortcutStems   map[string]bool
	shortcutDomains map[string]bool
	builtinDomains  map[string]bool
}

func diagnosticCommandScopeFromFiles(files map[string]bool) diagnosticCommandScope {
	scope := diagnosticCommandScope{
		shortcutStems:   map[string]bool{},
		shortcutDomains: map[string]bool{},
		builtinDomains:  map[string]bool{},
	}
	for file := range files {
		file = normalizeReferencePath(file)
		switch {
		case file == "cmd/service/service.go":
			scope.service = true
		case isTopLevelShortcutCommandFile(file), strings.HasPrefix(file, "shortcuts/common/"):
			scope.shortcutGlobal = true
		case strings.HasPrefix(file, "shortcuts/"):
			if stem := changedShortcutFileStem(file); stem != "" {
				scope.shortcutStems[stem] = true
			}
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

func (s diagnosticCommandScope) changed(cmd manifest.Command) bool {
	switch cmd.Source {
	case manifest.SourceService:
		return s.service
	case manifest.SourceShortcut:
		return s.shortcutGlobal || s.shortcutDomains[cmd.Domain] || s.shortcutCommandChanged(cmd)
	case manifest.SourceBuiltin:
		return s.builtinDomains[diagnosticFirstCommandSegment(cmd.Path)]
	default:
		return false
	}
}

func (s diagnosticCommandScope) shortcutCommandChanged(cmd manifest.Command) bool {
	if len(s.shortcutStems) == 0 {
		return false
	}
	for _, part := range strings.Fields(cmd.Path) {
		part = strings.TrimPrefix(part, "+")
		part = strings.ReplaceAll(part, "_", "-")
		if s.shortcutStems[part] {
			return true
		}
	}
	return false
}

func diagnosticFirstCommandSegment(path string) string {
	first, _, _ := strings.Cut(path, " ")
	return first
}

func loadBaseReferenceManifest(ctx context.Context, repo, changedFrom string) (*manifest.Manifest, bool, error) {
	if changedFrom == "" {
		return nil, false, nil
	}
	for _, source := range []struct {
		path     string
		kind     string
		complete bool
	}{
		{path: "internal/qualitygate/config/contracts/command_index.golden.json", kind: manifest.KindCommandIndex, complete: true},
		{path: "internal/qualitygate/config/contracts/command_manifest.golden.json", kind: manifest.KindCommandManifest, complete: false},
	} {
		data, err := qdiff.FileAtRevision(ctx, repo, changedFrom, source.path)
		if err != nil {
			if errors.Is(err, qdiff.ErrFileAtRevisionMissing) {
				continue
			}
			return nil, false, err
		}
		golden, err := manifest.ReadBytes(data, source.kind)
		if err != nil {
			return nil, false, err
		}
		return &golden, source.complete, nil
	}
	return nil, false, nil
}

func referenceCommandSurface(files map[string]bool) (bool, map[string]bool) {
	domains := map[string]bool{}
	for file := range files {
		file = normalizeReferencePath(file)
		switch {
		case file == "internal/cmdmeta/meta.go",
			file == "cmd/service/service.go",
			file == "internal/registry/meta_data.json",
			file == "internal/registry/meta_data_default.json",
			isTopLevelCommandFile(file),
			isTopLevelShortcutCommandFile(file),
			strings.HasPrefix(file, "shortcuts/common/"):
			return true, nil
		case strings.HasPrefix(file, "shortcuts/"):
			if domain := changedPathDomain(file, "shortcuts/"); domain != "" {
				domains[domain] = true
			}
		case strings.HasPrefix(file, "cmd/"):
			if domain := changedPathDomain(file, "cmd/"); domain != "" && domain != "service" {
				domains[domain] = true
			}
		}
	}
	return len(domains) > 0, domains
}

func changedShortcutFileStem(file string) string {
	if !strings.HasPrefix(file, "shortcuts/") || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
		return ""
	}
	name := filepath.Base(file)
	name = strings.TrimSuffix(name, ".go")
	return strings.ReplaceAll(name, "_", "-")
}

func changedPathDomain(file, prefix string) string {
	rest := strings.TrimPrefix(file, prefix)
	domain, _, ok := strings.Cut(rest, "/")
	if !ok || domain == "" || strings.HasSuffix(domain, ".go") {
		return ""
	}
	return normalizeCommandDomain(domain)
}

func isTopLevelShortcutCommandFile(file string) bool {
	return strings.HasPrefix(file, "shortcuts/") &&
		strings.Count(file, "/") == 1 &&
		strings.HasSuffix(file, ".go") &&
		!strings.HasSuffix(file, "_test.go")
}

func isTopLevelCommandFile(file string) bool {
	return strings.HasPrefix(file, "cmd/") &&
		strings.Count(file, "/") == 1 &&
		strings.HasSuffix(file, ".go") &&
		!strings.HasSuffix(file, "_test.go")
}

func normalizeCommandDomain(domain string) string {
	switch domain {
	case "doc":
		return "docs"
	default:
		return domain
	}
}
