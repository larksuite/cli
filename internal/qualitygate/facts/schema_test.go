// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package facts

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/vfs"
)

func TestFactsWriteFileCreatesParentAndValidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "facts.json")
	f := Facts{SchemaVersion: 1}
	if err := f.WriteFile(path); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatalf("read facts: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("facts is not valid JSON: %s", data)
	}
}

func TestFactsSchemaCarriesGatekeeperFields(t *testing.T) {
	f := Facts{
		SchemaVersion: 1,
		Errors:        []ErrorFact{{Code: "invalid_input", Message: "bad path", Hint: "pass --file", Retryable: false, HintActionCount: 1, RequiredHint: true}},
		Outputs:       []OutputFact{{Command: "im messages list", Fields: []string{"message_id", "sender", "create_time"}, IsList: true, HasDefaultLimit: true, HasDecisionField: true}},
		Skills:        []SkillFact{{SourceFile: "skills/lark-doc/SKILL.md", Line: 1, DestructiveWithoutGuard: true, ScopeConflict: true}},
		PublicContent: []PublicContentFact{{Rule: "public_content_generic_credential", Action: report.ActionReject, File: "docs/public.md", Line: 4, Excerpt: "api_key = <redacted>"}},
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal facts: %v", err)
	}
	var got Facts
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal facts: %v", err)
	}
	if !got.Errors[0].RequiredHint ||
		got.Outputs[0].Fields[0] != "message_id" ||
		!got.Skills[0].ScopeConflict ||
		got.PublicContent[0].Rule != "public_content_generic_credential" {
		t.Fatalf("facts lost gatekeeper fields: %#v", got)
	}
}

func TestBuildCarriesNamingFactsFromDiagnostics(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "drive +task_result", Source: manifest.SourceShortcut},
		{Path: "docs bad_cmd", Source: manifest.SourceShortcut},
		{Path: "im messages list", Source: manifest.SourceShortcut},
	}}
	diags := []report.Diagnostic{
		{Rule: "command_naming", Action: report.ActionLabel, File: "command-manifest", Message: "drive +task_result has non-kebab-case command segments: +task_result"},
		{Rule: "command_naming", Action: report.ActionReject, File: "command-manifest", Message: "docs bad_cmd has non-kebab-case command segments: bad_cmd"},
		{Rule: "flag_naming", Action: report.ActionReject, File: "command-manifest", Message: "im messages list --sort_type must use kebab-case"},
	}
	got := Build(m, nil, nil, nil, nil, nil, diags)
	byPath := map[string]CommandFact{}
	for _, cmd := range got.Commands {
		byPath[cmd.Path] = cmd
	}
	if !byPath["drive +task_result"].LegacyNaming {
		t.Fatalf("legacy command naming fact not set: %#v", byPath["drive +task_result"])
	}
	if !byPath["docs bad_cmd"].NameConflictsExisting {
		t.Fatalf("rejected command naming fact not set: %#v", byPath["docs bad_cmd"])
	}
	if !byPath["im messages list"].FlagAliasConflict {
		t.Fatalf("rejected flag naming fact not set: %#v", byPath["im messages list"])
	}
}

func TestDiagnosticsFromReportCarriesSubjectFields(t *testing.T) {
	got := DiagnosticsFromReport([]report.Diagnostic{{
		Rule:        "flag_naming",
		Action:      report.ActionReject,
		File:        "command-manifest",
		Message:     "flag must use kebab-case",
		CommandPath: "docs +whiteboard-update",
		FlagName:    "input_format",
		SubjectType: "flag",
	}})
	if len(got) != 1 {
		t.Fatalf("diagnostics len = %d, want 1", len(got))
	}
	if got[0].CommandPath != "docs +whiteboard-update" ||
		got[0].FlagName != "input_format" ||
		got[0].SubjectType != "flag" {
		t.Fatalf("diagnostic subject fields lost: %#v", got[0])
	}
}

func TestBuildAddsScopeAttribution(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "wiki nodes move", Domain: "wiki", Source: manifest.SourceShortcut},
		{Path: "im messages list", Domain: "im", Source: manifest.SourceService, Generated: true},
	}}
	got := Build(
		m,
		[]SkillFact{{SourceFile: "skills/lark-wiki/SKILL.md", Line: 30, CommandPath: "wiki nodes move"}},
		[]SkillQualityFact{{SourceFile: "skills/lark-wiki/SKILL.md"}},
		[]ErrorFact{{File: "cmd/wiki.go", Line: 9, Command: "wiki nodes move"}},
		[]CommandExample{{SourceFile: "skills/lark-wiki/SKILL.md", Line: 31, CommandPath: "wiki nodes move"}},
		[]OutputFact{{Command: "im messages list"}},
		nil,
		map[string]bool{"skills/lark-wiki/SKILL.md": true, "cmd/wiki.go": true},
	)

	if got.Commands[0].Domain != "wiki" || !got.Commands[0].Changed {
		t.Fatalf("command scope = %#v", got.Commands[0])
	}
	if got.Skills[0].Domain != "wiki" || !got.Skills[0].Changed || got.Skills[0].Source != "shortcut" {
		t.Fatalf("skill scope = %#v", got.Skills[0])
	}
	if got.SkillQuality[0].Domain != "wiki" || !got.SkillQuality[0].Changed {
		t.Fatalf("skill quality scope = %#v", got.SkillQuality[0])
	}
	if got.Errors[0].Domain != "wiki" || !got.Errors[0].Changed || got.Errors[0].CommandPath != "wiki nodes move" {
		t.Fatalf("error scope = %#v", got.Errors[0])
	}
	if got.Examples[0].Domain != "wiki" || !got.Examples[0].Changed {
		t.Fatalf("example scope = %#v", got.Examples[0])
	}
	if got.Outputs[0].Domain != "im" || !got.Outputs[0].Changed || got.Outputs[0].Source != "service" {
		t.Fatalf("output scope = %#v", got.Outputs[0])
	}
}

func TestBuildWithCommandLookupEnrichesServiceReferencesWithoutCommandFacts(t *testing.T) {
	handAuthored := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "docs +fetch",
		Domain: "docs",
		Source: manifest.SourceShortcut,
	}}}
	commandLookup := manifest.Manifest{Commands: append([]manifest.Command{}, handAuthored.Commands...)}
	commandLookup.Commands = append(commandLookup.Commands, manifest.Command{
		Path:      "drive file.comments create_v2",
		Domain:    "drive",
		Source:    manifest.SourceService,
		Generated: true,
	})

	got := BuildWithCommandLookup(
		handAuthored,
		commandLookup,
		[]SkillFact{{
			SourceFile:  "skills/lark-drive/references/lark-drive-add-comment.md",
			Line:        126,
			Raw:         "lark-cli drive file.comments create_v2 --file-token doccnxxxx",
			CommandPath: "drive file.comments create_v2",
		}},
		nil,
		nil,
		[]CommandExample{{
			SourceFile:  "skills/lark-drive/references/lark-drive-add-comment.md",
			Line:        126,
			Raw:         "lark-cli drive file.comments create_v2 --file-token doccnxxxx",
			CommandPath: "drive file.comments create_v2",
			Executable:  true,
		}},
		nil,
		nil,
	)

	if len(got.Commands) != 1 || got.Commands[0].Path != "docs +fetch" {
		t.Fatalf("service lookup command must not enter command facts: %#v", got.Commands)
	}
	if got.Skills[0].Domain != "drive" || got.Skills[0].Source != "service" {
		t.Fatalf("service skill fact not enriched: %#v", got.Skills[0])
	}
	if got.Examples[0].Domain != "drive" || got.Examples[0].Source != "service" {
		t.Fatalf("service example fact not enriched: %#v", got.Examples[0])
	}
}

func TestBuildMarksChangedCommandsAndOutputs(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "wiki nodes move", Domain: "wiki", Source: manifest.SourceShortcut},
		{Path: "im messages list", Domain: "im", Source: manifest.SourceService, Generated: true},
		{Path: "docs +fetch", Domain: "docs", Source: manifest.SourceShortcut},
	}}
	got := Build(
		m,
		nil,
		nil,
		nil,
		nil,
		[]OutputFact{{Command: "wiki nodes move"}, {Command: "im messages list"}, {Command: "docs +fetch"}},
		nil,
		map[string]bool{"shortcuts/wiki/move.go": true, "shortcuts/doc/docs_fetch.go": true},
	)

	byPath := map[string]CommandFact{}
	for _, command := range got.Commands {
		byPath[command.Path] = command
	}
	if !byPath["wiki nodes move"].Changed {
		t.Fatalf("shortcut command should be marked changed: %#v", byPath["wiki nodes move"])
	}
	if byPath["im messages list"].Changed {
		t.Fatalf("default metadata changes should not mark service commands changed: %#v", byPath["im messages list"])
	}
	if !byPath["docs +fetch"].Changed {
		t.Fatalf("doc shortcut folder should mark docs command changed: %#v", byPath["docs +fetch"])
	}
	if !got.Outputs[0].Changed || got.Outputs[1].Changed || !got.Outputs[2].Changed {
		t.Fatalf("outputs should inherit command changed state: %#v", got.Outputs)
	}
}

func TestBuildMarksAllShortcutCommandsChangedForRegisterFile(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "wiki +move", Domain: "wiki", Source: manifest.SourceShortcut},
		{Path: "mail +send", Domain: "mail", Source: manifest.SourceShortcut},
		{Path: "auth login", Domain: "auth", Source: manifest.SourceBuiltin},
	}}
	got := Build(
		m,
		nil,
		nil,
		nil,
		nil,
		[]OutputFact{{Command: "wiki +move"}, {Command: "mail +send"}, {Command: "auth login"}},
		nil,
		map[string]bool{"shortcuts/register.go": true},
	)

	byPath := map[string]CommandFact{}
	for _, command := range got.Commands {
		byPath[command.Path] = command
	}
	if !byPath["wiki +move"].Changed || !byPath["mail +send"].Changed {
		t.Fatalf("shortcut register should mark shortcut commands changed: %#v", got.Commands)
	}
	if byPath["auth login"].Changed {
		t.Fatalf("shortcut register should not mark builtin commands changed: %#v", byPath["auth login"])
	}
	if !got.Outputs[0].Changed || !got.Outputs[1].Changed || got.Outputs[2].Changed {
		t.Fatalf("outputs should follow command changed state: %#v", got.Outputs)
	}
}

func TestBuildMarksAllShortcutCommandsChangedForCommonFile(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "wiki +move", Domain: "wiki", Source: manifest.SourceShortcut},
		{Path: "mail +send", Domain: "mail", Source: manifest.SourceShortcut},
		{Path: "auth login", Domain: "auth", Source: manifest.SourceBuiltin},
	}}
	got := Build(
		m,
		nil,
		nil,
		nil,
		nil,
		[]OutputFact{{Command: "wiki +move"}, {Command: "mail +send"}, {Command: "auth login"}},
		nil,
		map[string]bool{"shortcuts/common/runner.go": true},
	)

	byPath := map[string]CommandFact{}
	for _, command := range got.Commands {
		byPath[command.Path] = command
	}
	if !byPath["wiki +move"].Changed || !byPath["mail +send"].Changed {
		t.Fatalf("common shortcut helper should mark shortcut commands changed: %#v", got.Commands)
	}
	if byPath["auth login"].Changed {
		t.Fatalf("common shortcut helper should not mark builtin commands changed: %#v", byPath["auth login"])
	}
	if !got.Outputs[0].Changed || !got.Outputs[1].Changed || got.Outputs[2].Changed {
		t.Fatalf("outputs should follow command changed state: %#v", got.Outputs)
	}
}

func TestBuildMarksDomainShortcutCommandsChangedForShortcutFile(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "docs +whiteboard-update", Domain: "docs", Source: manifest.SourceShortcut},
		{Path: "whiteboard +update", Domain: "whiteboard", Source: manifest.SourceShortcut},
		{Path: "auth login", Domain: "auth", Source: manifest.SourceBuiltin},
	}}
	got := Build(
		m,
		nil,
		nil,
		nil,
		nil,
		[]OutputFact{{Command: "docs +whiteboard-update"}, {Command: "whiteboard +update"}, {Command: "auth login"}},
		nil,
		map[string]bool{"shortcuts/whiteboard/whiteboard_update.go": true},
	)

	byPath := map[string]CommandFact{}
	for _, command := range got.Commands {
		byPath[command.Path] = command
	}
	if byPath["docs +whiteboard-update"].Changed || !byPath["whiteboard +update"].Changed {
		t.Fatalf("shortcut file changes should mark only its domain commands changed: %#v", got.Commands)
	}
	if byPath["auth login"].Changed {
		t.Fatalf("shortcut file should not mark builtin commands changed: %#v", byPath["auth login"])
	}
	if got.Outputs[0].Changed || !got.Outputs[1].Changed || got.Outputs[2].Changed {
		t.Fatalf("outputs should follow command changed state: %#v", got.Outputs)
	}
}

func TestBuildMarksAllCommandsChangedForCmdmeta(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "wiki +move", Domain: "wiki", Source: manifest.SourceShortcut},
		{Path: "mail messages list", Domain: "mail", Source: manifest.SourceService},
		{Path: "auth login", Domain: "auth", Source: manifest.SourceBuiltin},
	}}
	got := Build(
		m,
		nil,
		nil,
		nil,
		nil,
		[]OutputFact{{Command: "wiki +move"}, {Command: "mail messages list"}, {Command: "auth login"}},
		nil,
		map[string]bool{"internal/cmdmeta/meta.go": true},
	)

	for _, command := range got.Commands {
		if !command.Changed {
			t.Fatalf("cmdmeta change should mark every command changed: %#v", got.Commands)
		}
	}
	for _, output := range got.Outputs {
		if !output.Changed {
			t.Fatalf("cmdmeta change should mark every output changed: %#v", got.Outputs)
		}
	}
}

func TestDomainFromSkillPathNormalizesAlias(t *testing.T) {
	known := map[string]bool{"docs": true}
	// skills/lark-doc maps to the canonical command domain "docs"; without
	// alias normalization the lookup would miss and drop domain enrichment.
	if got := domainFromSkillPath("skills/lark-doc/SKILL.md", known); got != "docs" {
		t.Fatalf("domainFromSkillPath alias = %q, want %q", got, "docs")
	}
	if got := domainFromSkillPath("skills/lark-im/SKILL.md", map[string]bool{"im": true}); got != "im" {
		t.Fatalf("domainFromSkillPath non-alias = %q, want %q", got, "im")
	}
}
