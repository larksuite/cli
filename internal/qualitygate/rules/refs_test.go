// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/qualitygate/skillscan"
)

func TestCheckReferencesRejectsUnknownFlag(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "docs +fetch",
		Flags: []manifest.Flag{{Name: "api-version"}, {Name: "doc"}},
	}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch --api-version v2 --minute-token abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       12,
	}
	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("unknown flag should reject, got %#v", diags)
	}
	if !facts[0].ReferencesInvalidCommand {
		t.Fatalf("fact should mark invalid command reference")
	}
}

func TestCheckReferencesDowngradesUnchangedLegacySkillReferencesInIncrementalMode(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "docs +fetch"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch --legacy-flag abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       12,
	}
	diags, facts := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:  true,
		ChangedFiles: map[string]bool{},
	})
	if len(diags) != 1 || diags[0].Action != report.ActionWarning {
		t.Fatalf("unchanged legacy skill reference should warn, got %#v", diags)
	}
	if !facts[0].ReferencesInvalidCommand {
		t.Fatalf("fact should still mark invalid command reference")
	}
}

func TestCheckReferencesRejectsUnchangedSkillReferenceForChangedCommandSurface(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "docs +fetch"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch --removed-flag abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       12,
	}
	base := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "docs +fetch",
		Flags: []manifest.Flag{{Name: "removed-flag", TakesValue: true}},
	}}}
	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:            true,
		ChangedFiles:           map[string]bool{},
		CommandSurfaceAffected: true,
		BaseManifest:           &base,
	})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("changed command surface should reject unchanged same-domain reference, got %#v", diags)
	}
}

func TestCheckReferencesDowngradesUnchangedSkillReferenceWhenCommandSurfaceChangedWithoutBase(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "drive file.comments create_v2"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli drive file.comments create_v2 --removed-flag abc",
		SourceFile: "skills/lark-drive/SKILL.md",
		Line:       42,
	}

	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:            true,
		ChangedFiles:           map[string]bool{},
		CommandSurfaceAffected: true,
	})
	if len(diags) != 1 || diags[0].Action != report.ActionWarning {
		t.Fatalf("missing base index during command-surface change should warn, got %#v", diags)
	}
}

func TestCheckReferencesDowngradesServiceReferenceWhenIncompleteBaseManifestCannotProveRegression(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "drive file.comments create_v2",
		Domain: "drive",
		Source: manifest.SourceService,
	}}}
	ex := skillscan.Example{
		Raw:        "lark-cli drive file.comments create_v2 --removed-flag abc",
		SourceFile: "skills/lark-drive/SKILL.md",
		Line:       42,
	}
	base := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "docs +fetch",
		Domain: "docs",
		Source: manifest.SourceShortcut,
	}}}

	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:            true,
		ChangedFiles:           map[string]bool{},
		CommandSurfaceAffected: true,
		BaseManifest:           &base,
		BaseManifestComplete:   false,
	})
	if len(diags) != 1 || diags[0].Action != report.ActionWarning {
		t.Fatalf("incomplete base manifest should not block unchanged legacy references without proof, got %#v", diags)
	}
}

func TestCheckReferencesUsesBaseCommandDomainForCrossSkillReferences(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "auth login"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli auth login --domain mail",
		SourceFile: "skills/lark-mail/SKILL.md",
		Line:       42,
	}
	base := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "auth login",
		Flags: []manifest.Flag{{Name: "domain", TakesValue: true}},
	}}}
	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:            true,
		ChangedFiles:           map[string]bool{},
		CommandSurfaceAffected: true,
		BaseManifest:           &base,
	})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("base command domain should reject cross-skill broken reference, got %#v", diags)
	}
}

func TestCheckReferencesDoesNotTrustChangedPathDomainForBaseRegression(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "docs +whiteboard-update"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +whiteboard-update --removed-flag abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       42,
	}
	base := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "docs +whiteboard-update",
		Flags: []manifest.Flag{{Name: "removed-flag", TakesValue: true}},
	}}}
	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:            true,
		ChangedFiles:           map[string]bool{},
		CommandSurfaceAffected: true,
		BaseManifest:           &base,
	})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("base regression should reject even when changed path domain differs, got %#v", diags)
	}
}

func TestCheckReferencesRejectsChangedSkillReferencesInIncrementalMode(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "docs +fetch"}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch --bad-flag abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       12,
	}
	diags, _ := CheckReferencesWithPolicy(m, []skillscan.Example{ex}, ReferencePolicy{
		Incremental:  true,
		ChangedFiles: map[string]bool{"skills/lark-doc/SKILL.md": true},
	})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("changed skill reference should reject, got %#v", diags)
	}
}

func TestCheckReferencesAcceptsEmbeddedServiceCommand(t *testing.T) {
	m := embeddedServiceCommandIndex()
	ex := skillscan.Example{
		Raw:        `lark-cli drive file.comments create_v2 --file-token doccnxxxx --params '{"file_type":"docx"}' --data '{"reply_list":[{"content":"looks good"}]}'`,
		SourceFile: "skills/lark-drive/references/lark-drive-add-comment.md",
		Line:       126,
	}

	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("service command reference should pass, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].ReferencesInvalidCommand {
		t.Fatalf("service command fact should be valid, got %#v", facts)
	}
	if facts[0].CommandPath != "drive file.comments create_v2" {
		t.Fatalf("command path = %q", facts[0].CommandPath)
	}
}

func TestCheckReferencesRejectsUnknownFlagOnEmbeddedServiceCommand(t *testing.T) {
	m := embeddedServiceCommandIndex()
	ex := skillscan.Example{
		Raw:        `lark-cli drive file.comments create_v2 --file-token doccnxxxx --bad-flag value`,
		SourceFile: "skills/lark-drive/references/lark-drive-add-comment.md",
		Line:       126,
	}

	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("unknown service flag should reject, got %#v", diags)
	}
	if !facts[0].ReferencesInvalidCommand {
		t.Fatalf("fact should mark invalid command reference")
	}
}

func embeddedServiceCommandIndex() manifest.Manifest {
	return manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:      "drive file.comments create_v2",
		Domain:    "drive",
		Source:    manifest.SourceService,
		Generated: true,
		Runnable:  true,
		Flags: []manifest.Flag{
			{Name: "file-token", TakesValue: true},
			{Name: "params", TakesValue: true},
			{Name: "data", TakesValue: true},
			{Name: "dry-run"},
		},
	}}}
}

func TestCheckReferencesRejectsUnknownFlagAfterPlaceholderArg(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "docs +fetch",
		Flags: []manifest.Flag{{Name: "api-version", TakesValue: true}},
	}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch <doc_token> --bad-flag",
		SourceFile: "skills/lark-doc/references/fetch.md",
		Line:       20,
	}
	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 1 || diags[0].Action != report.ActionReject {
		t.Fatalf("unknown flag after placeholder should reject, got %#v", diags)
	}
	if !facts[0].ReferencesInvalidCommand {
		t.Fatalf("fact should mark invalid command reference")
	}
}

func TestParseExampleUsesCommandTreeBeforeFlagValues(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:  "docs +fetch",
		Flags: []manifest.Flag{{Name: "api-version", TakesValue: true}, {Name: "doc", TakesValue: true}},
	}}}
	ex := skillscan.Example{
		Raw:        "lark-cli docs +fetch --api-version v2 --doc abc",
		SourceFile: "skills/lark-doc/SKILL.md",
		Line:       8,
	}

	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("valid command produced diagnostics: %#v", diags)
	}
	if facts[0].CommandPath != "docs +fetch" {
		t.Fatalf("command path = %q", facts[0].CommandPath)
	}
}

func TestParseExampleAllowsFlagShorthandAndIgnoresPipelineTail(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path: "mail +message",
		Flags: []manifest.Flag{
			{Name: "message-id", TakesValue: true},
			{Name: "format", TakesValue: true},
			{Name: "jq", Shorthand: "q", TakesValue: true},
		},
	}}}
	ex := skillscan.Example{
		Raw:        `lark-cli mail +message --message-id abc --format json -q '.data.body_html' | jq -r '.'`,
		SourceFile: "skills/lark-mail/SKILL.md",
		Line:       8,
	}

	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("valid command produced diagnostics: %#v", diags)
	}
	if facts[0].CommandPath != "mail +message" {
		t.Fatalf("command path = %q", facts[0].CommandPath)
	}
}

func TestParseAgainstManifestConsumesShortFlagValue(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path: "mail +message",
		Flags: []manifest.Flag{
			{Name: "jq", Shorthand: "q", TakesValue: true},
		},
	}}}

	got, err := parseAgainstManifest(m, `lark-cli mail +message -q '.data.body_html' target`)
	if err != nil {
		t.Fatalf("parseAgainstManifest() error = %v", err)
	}
	if strings.Join(got.Flags, ",") != "jq" {
		t.Fatalf("flags = %#v, want jq", got.Flags)
	}
	if strings.Join(got.Positional, ",") != "target" {
		t.Fatalf("positional = %#v, want target", got.Positional)
	}
}

func TestParseExampleIgnoresTrailingShellComment(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path: "schema",
	}}}
	ex := skillscan.Example{
		Raw:        `lark-cli schema wiki.<resource>.<method> # read --data and --params shape first`,
		SourceFile: "skills/lark-wiki/SKILL.md",
		Line:       82,
	}

	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("trailing shell comment should not produce flag diagnostics: %#v", diags)
	}
	if facts[0].CommandPath != "schema" {
		t.Fatalf("command path = %q, want schema", facts[0].CommandPath)
	}
}

func TestCheckReferencesUsesLongestManifestPrefix(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "api",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "params"}, {Name: "dry-run"}},
	}}}
	ex := skillscan.Example{
		Raw:        `lark-cli api GET /open-apis/test --params '{"a":"1"}' --dry-run`,
		SourceFile: "command-manifest",
		Line:       1,
	}
	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("api command with positional args should pass, got %#v", diags)
	}
	if facts[0].ReferencesInvalidCommand {
		t.Fatalf("fact should not mark invalid command reference")
	}
}

func TestCheckReferencesDoesNotLetGroupCommandSwallowUnknownMethod(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "mail user_mailboxes", Runnable: true, Flags: []manifest.Flag{{Name: "params"}}},
		{Path: "mail user_mailboxes search", Runnable: true, Flags: []manifest.Flag{{Name: "params"}}},
	}}
	ex := skillscan.Example{
		Raw:        `lark-cli mail user_mailboxes missing_method --params '{"id":"me"}'`,
		SourceFile: "skills/lark-mail/SKILL.md",
		Line:       1,
	}
	diags, _ := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 1 || !strings.Contains(diags[0].Message, "unknown command") {
		t.Fatalf("unknown method should reject as command, got %#v", diags)
	}
}

func TestCheckReferencesAllowsHelpFlag(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "im"}}}
	ex := skillscan.Example{Raw: "lark-cli im --help", SourceFile: "skills/lark-im/SKILL.md", Line: 1}
	diags, _ := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("--help should be allowed, got %#v", diags)
	}
}

func TestCheckReferencesSkipsTemplateServicePlaceholder(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "im"}}}
	ex := skillscan.Example{Raw: "lark-cli im <resource> <method> [flags]", SourceFile: "skills/lark-demo/SKILL.md", Line: 1}
	diags, facts := CheckReferences(m, []skillscan.Example{ex})
	if len(diags) != 0 {
		t.Fatalf("template placeholder should not reject, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].ReferencesInvalidCommand {
		t.Fatalf("template fact should be non-invalid, got %#v", facts)
	}
}

func TestParseAgainstManifestWarnsOnUnclosedQuote(t *testing.T) {
	_, err := parseAgainstManifest(manifest.Manifest{}, `lark-cli docs +fetch --doc "abc`)
	if err == nil {
		t.Fatal("expected parse error")
	}
}
