// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	qdiff "github.com/larksuite/cli/internal/qualitygate/diff"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
	"github.com/larksuite/cli/internal/testutil/gitcmd"
	"github.com/larksuite/cli/internal/vfs"
)

func TestShouldRunNamingForCommandChanges(t *testing.T) {
	skillOnly := qdiff.FromChangedFiles([]string{"skills/lark-doc/SKILL.md"})
	if shouldRunNaming("origin/main", skillOnly) {
		t.Fatal("skill-only change should not run naming")
	}
	commandChange := qdiff.FromChangedFiles([]string{"shortcuts/docs/docs_fetch.go"})
	if !shouldRunNaming("origin/main", commandChange) {
		t.Fatal("shortcut change should run naming")
	}
}

func TestShouldRunNamingIgnoresDefaultMetadataChanges(t *testing.T) {
	scope := qdiff.FromChangedFiles([]string{"internal/registry/meta_data_default.json"})
	if shouldRunNaming("origin/main", scope) {
		t.Fatal("default metadata changes should not run ordinary naming gate")
	}
}

func TestReferenceCommandSurfaceTreatsShortcutRegisterAsGlobal(t *testing.T) {
	affected, domains := referenceCommandSurface(map[string]bool{"shortcuts/register.go": true})
	if !affected || len(domains) != 0 {
		t.Fatalf("shortcut registration must be global command surface, affected=%v domains=%#v", affected, domains)
	}
}

func TestReferenceCommandSurfaceTreatsTopLevelCmdFilesAsGlobal(t *testing.T) {
	for _, file := range []string{"cmd/build.go", "cmd/global_flags.go"} {
		affected, domains := referenceCommandSurface(map[string]bool{file: true})
		if !affected || len(domains) != 0 {
			t.Fatalf("%s must be global command surface, affected=%v domains=%#v", file, affected, domains)
		}
	}
}

func TestReferenceCommandSurfaceTreatsServiceMetadataAsGlobal(t *testing.T) {
	for _, file := range []string{"internal/registry/meta_data.json", "internal/registry/meta_data_default.json"} {
		affected, domains := referenceCommandSurface(map[string]bool{file: true})
		if !affected || len(domains) != 0 {
			t.Fatalf("%s must affect reference command surface, affected=%v domains=%#v", file, affected, domains)
		}
	}
}

func TestReferenceCommandSurfaceNormalizesShortcutDomain(t *testing.T) {
	affected, domains := referenceCommandSurface(map[string]bool{"shortcuts/doc/docs_fetch.go": true})
	if !affected || !domains["docs"] {
		t.Fatalf("shortcut doc folder should map to docs command domain, affected=%v domains=%#v", affected, domains)
	}
}

func TestRunRequiresCommandIndexToCoverManifest(t *testing.T) {
	repo := t.TempDir()
	manifestPath := filepath.Join(repo, "command-manifest.json")
	indexPath := filepath.Join(repo, "command-index.json")
	m := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:          "docs +fetch",
		CanonicalPath: "docs +fetch",
		Domain:        "docs",
		Source:        manifest.SourceShortcut,
	}}}
	idx := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:          "drive file.comments create_v2",
		CanonicalPath: "drive file-comments create-v2",
		Domain:        "drive",
		Source:        manifest.SourceService,
		Generated:     true,
	}}}
	if err := manifest.WriteFile(manifestPath, manifest.KindCommandManifest, m); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteFile(indexPath, manifest.KindCommandIndex, idx); err != nil {
		t.Fatal(err)
	}

	_, _, err := Run(context.Background(), Options{
		Repo:             repo,
		CLIBin:           "./lark-cli",
		ManifestPath:     manifestPath,
		CommandIndexPath: indexPath,
	})
	if err == nil || !strings.Contains(err.Error(), `missing "docs +fetch"`) {
		t.Fatalf("Run() error = %v, want incomplete command-index error", err)
	}
}

func TestRunReadsManifestFilesAndAcceptsServiceReferences(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := vfs.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")

	skillPath := filepath.Join(repo, "skills", "lark-drive", "SKILL.md")
	if err := vfs.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatal(err)
	}
	skill := `---
name: lark-drive
description: Manage Drive comments with service command references.
---

` + "```bash\n" + `lark-cli drive file.comments create_v2 --file-token doccnxxxx --params '{"file_type":"docx"}' --data '{"reply_list":[]}'` + "\n```\n"
	if err := vfs.WriteFile(skillPath, []byte(skill), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "skills/lark-drive/SKILL.md")
	runGit(t, repo, "commit", "-m", "add skill reference")

	manifestPath := filepath.Join(repo, "command-manifest.json")
	indexPath := filepath.Join(repo, "command-index.json")
	m := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:          "docs +fetch",
		CanonicalPath: "docs +fetch",
		Domain:        "docs",
		Source:        manifest.SourceShortcut,
	}}}
	idx := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{
		{
			Path:          "docs +fetch",
			CanonicalPath: "docs +fetch",
			Domain:        "docs",
			Source:        manifest.SourceShortcut,
		},
		{
			Path:          "drive file.comments create_v2",
			CanonicalPath: "drive file-comments create-v2",
			Domain:        "drive",
			Source:        manifest.SourceService,
			Generated:     true,
			Runnable:      true,
			Flags: []manifest.Flag{
				{Name: "file-token", TakesValue: true},
				{Name: "params", TakesValue: true},
				{Name: "data", TakesValue: true},
				{Name: "dry-run"},
			},
		},
	}}
	if err := manifest.WriteFile(manifestPath, manifest.KindCommandManifest, m); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteFile(indexPath, manifest.KindCommandIndex, idx); err != nil {
		t.Fatal(err)
	}
	cliBin, _ := fakeDryRunCLI(t, `{"api":[{"method":"POST","url":"/open-apis/drive/v1/files/comments"}]}`)

	diags, gotFacts, err := Run(context.Background(), Options{
		Repo:             repo,
		CLIBin:           cliBin,
		ChangedFrom:      "HEAD~1",
		ManifestPath:     manifestPath,
		CommandIndexPath: indexPath,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(diags) != 0 {
		t.Fatalf("Run() diagnostics = %#v", diags)
	}
	if len(gotFacts.Skills) != 1 {
		t.Fatalf("skill facts = %#v", gotFacts.Skills)
	}
	if got := gotFacts.Skills[0]; got.ReferencesInvalidCommand || got.CommandPath != "drive file.comments create_v2" || got.Source != string(manifest.SourceService) {
		t.Fatalf("service reference fact = %#v", got)
	}
}

func TestRunCollectsPublicContentFindingsIntoDiagnosticsAndFacts(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := vfs.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "base")

	if err := vfs.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	providerValue := "ghp_" + "1234567890abcdef1234567890abcdef1234"
	publicDoc := "api_" + "key = \"" + providerValue + "\"\n" +
		"Public docs describe a pri" + "vate request header and trust classification detail.\n"
	if err := vfs.WriteFile(filepath.Join(repo, "docs", "public.md"), []byte(publicDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", "docs/public.md")
	runGit(t, repo, "commit", "-m", "add public doc")

	metadataPath := filepath.Join(repo, "pr-metadata.json")
	if err := vfs.WriteFile(metadataPath, []byte(`{"title":"public docs","body":"Change`+`-Id: I0123456789abcdef0123456789abcdef01234567"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(repo, "command-manifest.json")
	indexPath := filepath.Join(repo, "command-index.json")
	m := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:          "docs +fetch",
		CanonicalPath: "docs +fetch",
		Domain:        "docs",
		Source:        manifest.SourceShortcut,
	}}}
	if err := manifest.WriteFile(manifestPath, manifest.KindCommandManifest, m); err != nil {
		t.Fatal(err)
	}
	idx := manifest.Manifest{SchemaVersion: 1, Commands: append([]manifest.Command{}, m.Commands...)}
	idx.Commands = append(idx.Commands, manifest.Command{
		Path:          "drive files get",
		CanonicalPath: "drive files get",
		Domain:        "drive",
		Source:        manifest.SourceService,
		Generated:     true,
		Runnable:      true,
	})
	if err := manifest.WriteFile(indexPath, manifest.KindCommandIndex, idx); err != nil {
		t.Fatal(err)
	}

	diags, gotFacts, err := Run(context.Background(), Options{
		Repo:                      repo,
		CLIBin:                    "./lark-cli",
		ChangedFrom:               "HEAD~1",
		ManifestPath:              manifestPath,
		CommandIndexPath:          indexPath,
		PublicContentMetadataPath: metadataPath,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	actions := map[string]report.Action{}
	for _, diag := range diags {
		actions[diag.Rule] = diag.Action
	}
	if actions["public_content_generic_credential"] != report.ActionReject {
		t.Fatalf("generic credential diagnostic action = %q, diagnostics=%#v", actions["public_content_generic_credential"], diags)
	}
	if actions["public_content_change_id_trailer"] != report.ActionReject {
		t.Fatalf("change-id diagnostic action = %q, diagnostics=%#v", actions["public_content_change_id_trailer"], diags)
	}
	if actions["public_content_semantic_candidate"] != "" {
		t.Fatalf("semantic candidates should not become deterministic diagnostics: %#v", diags)
	}
	factRules := map[string]bool{}
	for _, item := range gotFacts.PublicContent {
		factRules[item.Rule] = true
	}
	for _, want := range []string{
		"public_content_generic_credential",
		"public_content_change_id_trailer",
		"public_content_semantic_candidate",
	} {
		if !factRules[want] {
			t.Fatalf("missing public content fact %s: %#v", want, gotFacts.PublicContent)
		}
	}
	if len(gotFacts.PublicContent) < 3 {
		t.Fatalf("public content facts = %#v", gotFacts.PublicContent)
	}
}

func TestLoadBaseReferenceManifestReadsCommandGolden(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	golden := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:   "docs +fetch",
		Domain: "docs",
		Source: manifest.SourceShortcut,
		Flags:  []manifest.Flag{{Name: "doc", TakesValue: true}},
	}}}
	data, err := json.Marshal(golden)
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	path := filepath.Join(repo, "internal", "qualitygate", "config", "contracts", "command_manifest.golden.json")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}
	if err := vfs.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	runGit(t, repo, "add", "internal/qualitygate/config/contracts/command_manifest.golden.json")
	runGit(t, repo, "commit", "-m", "add command golden")

	base, complete, err := loadBaseReferenceManifest(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatalf("loadBaseReferenceManifest() error = %v", err)
	}
	if complete {
		t.Fatal("legacy command_manifest golden must be marked incomplete")
	}
	if base == nil || len(base.Commands) != 1 {
		t.Fatalf("base manifest = %#v", base)
	}
	if got := base.Commands[0].Flags[0].Name; got != "doc" {
		t.Fatalf("base flag = %q, want doc", got)
	}
}

func TestLoadBaseReferenceManifestReadsCommandIndexGolden(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	golden := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:      "drive file.comments create_v2",
		Domain:    "drive",
		Source:    manifest.SourceService,
		Generated: true,
		Flags:     []manifest.Flag{{Name: "file-token", TakesValue: true}},
	}}}
	data, err := json.Marshal(golden)
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	path := filepath.Join(repo, "internal", "qualitygate", "config", "contracts", "command_index.golden.json")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}
	if err := vfs.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	runGit(t, repo, "add", "internal/qualitygate/config/contracts/command_index.golden.json")
	runGit(t, repo, "commit", "-m", "add command index golden")

	base, complete, err := loadBaseReferenceManifest(context.Background(), repo, "HEAD")
	if err != nil {
		t.Fatalf("loadBaseReferenceManifest() error = %v", err)
	}
	if !complete {
		t.Fatal("command_index golden must be marked complete")
	}
	if base == nil || len(base.Commands) != 1 {
		t.Fatalf("base manifest = %#v", base)
	}
	if got := base.Commands[0].Source; got != manifest.SourceService {
		t.Fatalf("base command source = %q, want service", got)
	}
}

func TestLoadBaseReferenceManifestRejectsEmptyGolden(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	path := filepath.Join(repo, "internal", "qualitygate", "config", "contracts", "command_manifest.golden.json")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}
	if err := vfs.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty golden: %v", err)
	}
	runGit(t, repo, "add", "internal/qualitygate/config/contracts/command_manifest.golden.json")
	runGit(t, repo, "commit", "-m", "add empty golden")

	if _, _, err := loadBaseReferenceManifest(context.Background(), repo, "HEAD"); err == nil {
		t.Fatal("empty base command manifest should be an error, not bootstrap fail-open")
	}
}

func TestLoadBaseReferenceManifestRejectsInvalidGoldenKind(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	golden := manifest.Manifest{SchemaVersion: 1, Commands: []manifest.Command{{
		Path:          "docs +fetch",
		CanonicalPath: "docs +fetch",
		Domain:        "docs",
		Source:        manifest.SourceShortcut,
	}}}
	data, err := json.Marshal(golden)
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	path := filepath.Join(repo, "internal", "qualitygate", "config", "contracts", "command_index.golden.json")
	if err := vfs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir golden dir: %v", err)
	}
	if err := vfs.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	runGit(t, repo, "add", "internal/qualitygate/config/contracts/command_index.golden.json")
	runGit(t, repo, "commit", "-m", "add invalid command index golden")

	if _, _, err := loadBaseReferenceManifest(context.Background(), repo, "HEAD"); err == nil {
		t.Fatal("command_index golden without service commands should be rejected")
	}
}

func TestFilterPRDiagnosticsDropsUnchangedHealthWarnings(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "auth list", Domain: "auth", Source: manifest.SourceBuiltin},
		{Path: "docs +fetch", Domain: "docs", Source: manifest.SourceShortcut},
	}}
	scope := qdiff.FromChangedFiles([]string{"cmd/build.go"})
	diags := []report.Diagnostic{
		{
			Rule:    "default_output",
			Action:  report.ActionWarning,
			File:    "command-manifest",
			Message: "auth list looks like a list command without an explicit default limit flag",
		},
		{
			Rule:    "skill_size_budget",
			Action:  report.ActionWarning,
			File:    "skills/lark-mail/SKILL.md",
			Message: "skill body has 2888 words",
		},
		{
			Rule:    "allowlist_format",
			Action:  report.ActionReject,
			File:    "internal/qualitygate/config/allowlists/legacy-commands.txt",
			Message: "legacy allowlist row must include owner, reason, and added_at",
		},
	}

	got := filterPRDiagnostics(".", "origin/main", scope, m, diags)
	if len(got) != 0 {
		t.Fatalf("unchanged health warnings should be hidden in PR mode, got %#v", got)
	}
}

func TestFilterPRDiagnosticsKeepsChangedSkillAndCommandDomain(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "docs +fetch", Domain: "docs", Source: manifest.SourceShortcut},
		{Path: "sheets +filter-list", Domain: "sheets", Source: manifest.SourceShortcut},
	}}
	scope := qdiff.FromChangedFiles([]string{
		"skills/lark-mail/SKILL.md",
		"shortcuts/doc/docs_fetch.go",
		"internal/qualitygate/config/allowlists/legacy-flags.txt",
	})
	diags := []report.Diagnostic{
		{
			Rule:    "skill_size_budget",
			Action:  report.ActionWarning,
			File:    "skills/lark-mail/SKILL.md",
			Message: "skill body has 2888 words",
		},
		{
			Rule:    "skill_size_budget",
			Action:  report.ActionWarning,
			File:    "skills/lark-drive/SKILL.md",
			Message: "skill body has 3000 words",
		},
		{
			Rule:    "default_output",
			Action:  report.ActionWarning,
			File:    "command-manifest",
			Message: "docs +fetch looks like a list command without an explicit default limit flag",
		},
		{
			Rule:    "default_output",
			Action:  report.ActionWarning,
			File:    "command-manifest",
			Message: "sheets +filter-list looks like a list command without an explicit default limit flag",
		},
		{
			Rule:    "allowlist_format",
			Action:  report.ActionReject,
			File:    "internal/qualitygate/config/allowlists/legacy-flags.txt",
			Message: "legacy allowlist row must include owner, reason, and added_at",
		},
	}

	got := filterPRDiagnostics(".", "origin/main", scope, m, diags)
	if len(got) != 3 {
		t.Fatalf("expected changed skill, changed command domain, and changed allowlist diagnostics, got %#v", got)
	}
	for _, diag := range got {
		switch {
		case diag.File == "skills/lark-mail/SKILL.md":
		case diag.File == "command-manifest" && diag.Message == "docs +fetch looks like a list command without an explicit default limit flag":
		case diag.File == "internal/qualitygate/config/allowlists/legacy-flags.txt":
		default:
			t.Fatalf("unexpected diagnostic kept: %#v", diag)
		}
	}
}

func TestFilterPRDiagnosticsUsesStructuredCommandPath(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "docs +fetch",
		Domain: "docs",
		Source: manifest.SourceShortcut,
	}}}
	scope := qdiff.FromChangedFiles([]string{"shortcuts/doc/docs_fetch.go"})
	diags := []report.Diagnostic{{
		Rule:        "default_output_contract",
		Action:      report.ActionReject,
		File:        "command-manifest",
		Message:     "default output must include a default limit and agent decision fields",
		CommandPath: "docs +fetch",
		SubjectType: "output",
	}}

	got := filterPRDiagnostics(".", "origin/main", scope, m, diags)
	if len(got) != 1 {
		t.Fatalf("structured command_path diagnostic should be kept without parsing message, got %#v", got)
	}
}

func TestFilterPRDiagnosticsKeepsShortcutAliasCommand(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{
		{Path: "docs +whiteboard-update", Domain: "docs", Source: manifest.SourceShortcut},
		{Path: "whiteboard +update", Domain: "whiteboard", Source: manifest.SourceShortcut},
		{Path: "mail +send", Domain: "mail", Source: manifest.SourceShortcut},
		{Path: "auth login", Domain: "auth", Source: manifest.SourceBuiltin},
	}}
	scope := qdiff.FromChangedFiles([]string{"shortcuts/whiteboard/whiteboard_update.go"})
	diags := []report.Diagnostic{
		{
			Rule:        "flag_naming",
			Action:      report.ActionReject,
			File:        "command-manifest",
			Message:     "flag must use kebab-case",
			CommandPath: "docs +whiteboard-update",
			FlagName:    "input_format",
			SubjectType: "flag",
		},
		{
			Rule:        "flag_naming",
			Action:      report.ActionReject,
			File:        "command-manifest",
			Message:     "flag must use kebab-case",
			CommandPath: "mail +send",
			FlagName:    "bad_flag",
			SubjectType: "flag",
		},
		{
			Rule:        "flag_naming",
			Action:      report.ActionReject,
			File:        "command-manifest",
			Message:     "flag must use kebab-case",
			CommandPath: "auth login",
			FlagName:    "bad_flag",
			SubjectType: "flag",
		},
	}

	got := filterPRDiagnostics(".", "origin/main", scope, m, diags)
	if len(got) != 1 {
		t.Fatalf("expected only shortcut alias command diagnostic, got %#v", got)
	}
	if got[0].CommandPath != "docs +whiteboard-update" {
		t.Fatalf("kept diagnostic command_path = %q", got[0].CommandPath)
	}
}

func TestFilterPRDiagnosticsKeepsFullModeDiagnostics(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{Path: "auth list", Domain: "auth", Source: manifest.SourceBuiltin}}}
	diags := []report.Diagnostic{{
		Rule:    "default_output",
		Action:  report.ActionWarning,
		File:    "command-manifest",
		Message: "auth list looks like a list command without an explicit default limit flag",
	}}
	got := filterPRDiagnostics(".", "", qdiff.Scope{}, m, diags)
	if len(got) != len(diags) {
		t.Fatalf("full mode should keep diagnostics, got %#v", got)
	}
}

func TestNormalizeDiagnosticFileHandlesAbsoluteRepo(t *testing.T) {
	parent := t.TempDir()
	repo := filepath.Join(parent, "repo")
	absoluteFile := filepath.Join(repo, "skills", "lark-doc", "SKILL.md")
	got := normalizeDiagnosticFile(repo, absoluteFile)
	if got != "skills/lark-doc/SKILL.md" {
		t.Fatalf("normalizeDiagnosticFile() = %q, want skills/lark-doc/SKILL.md", got)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	commandArgs := append([]string{"-c", "core.hooksPath=/dev/null"}, args...)
	cmd := gitcmd.Command(repo, commandArgs...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-06-17T00:00:00Z", "GIT_COMMITTER_DATE=2026-06-17T00:00:00Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
