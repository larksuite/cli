// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"strings"
	"testing"

	qallowlist "github.com/larksuite/cli/internal/qualitygate/allowlist"
	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestFlagNamingRejectsNewUnderscore(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "im messages list",
		Source: manifest.SourceShortcut,
		Flags:  []manifest.Flag{{Name: "sort_type"}},
	}}}
	diags := CheckNaming(m, NamingAllowlist{})
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics", len(diags))
	}
	if diags[0].Action != report.ActionReject {
		t.Fatalf("new underscore flag should reject, got %s", diags[0].Action)
	}
	if diags[0].CommandPath != "im messages list" || diags[0].FlagName != "sort_type" || diags[0].SubjectType != "flag" {
		t.Fatalf("flag diagnostic subject = %#v", diags[0])
	}
}

func TestFlagNamingLabelsAllowlistedLegacy(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "im messages list",
		Source: manifest.SourceShortcut,
		Flags:  []manifest.Flag{{Name: "sort_type"}},
	}}}
	allow := NamingAllowlist{Flags: Allowlist{"im messages list sort_type": "legacy public flag"}}
	diags := CheckNaming(m, allow)
	if len(diags) != 1 || diags[0].Action != report.ActionLabel {
		t.Fatalf("allowlisted legacy flag should label, got %#v", diags)
	}
}

func TestCommandNamingRejectsNewHandAuthoredUnderscore(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "docs bad_name",
		Source: manifest.SourceShortcut,
	}}}
	diags := CheckNaming(m, NamingAllowlist{})
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics", len(diags))
	}
	if diags[0].Rule != "command_naming" || diags[0].Action != report.ActionReject {
		t.Fatalf("new hand-authored command should reject, got %#v", diags)
	}
	if diags[0].CommandPath != "docs bad_name" || diags[0].SubjectType != "command" {
		t.Fatalf("command diagnostic subject = %#v", diags[0])
	}
}

func TestCommandNamingLabelsAllowlistedLegacyShortcut(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "drive +task_result",
		Source: manifest.SourceShortcut,
	}}}
	allow := NamingAllowlist{Commands: Allowlist{"drive +task_result": "legacy public shortcut"}}
	diags := CheckNaming(m, allow)
	if len(diags) != 1 || diags[0].Action != report.ActionLabel {
		t.Fatalf("allowlisted legacy command should label, got %#v", diags)
	}
}

func TestCommandNamingRejectsGeneratedAnnotationOnHandAuthoredCommand(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:      "docs +fetch",
		Source:    manifest.SourceShortcut,
		Generated: true,
	}}}
	diags := CheckNaming(m, NamingAllowlist{})
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics", len(diags))
	}
	if diags[0].Rule != "source_annotation_misuse" || diags[0].Action != report.ActionReject {
		t.Fatalf("invalid generated annotation should reject, got %#v", diags)
	}
}

func TestLegacyNamingCandidatesMatchAllowlistParsers(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:   "drive +task_result",
		Source: manifest.SourceShortcut,
		Flags:  []manifest.Flag{{Name: "input_format"}},
	}}}

	commandItems, commandDiags := qallowlist.ParseLegacyCommands(strings.NewReader(strings.Join(LegacyCommandCandidates(m), "\n")))
	if len(commandDiags) != 0 || len(commandItems) != 1 {
		t.Fatalf("legacy command candidates must parse as allowlist rows, items=%#v diags=%#v", commandItems, commandDiags)
	}
	flagItems, flagDiags := qallowlist.ParseLegacyFlags(strings.NewReader(strings.Join(LegacyFlagCandidates(m), "\n")))
	if len(flagDiags) != 0 || len(flagItems) != 1 {
		t.Fatalf("legacy flag candidates must parse as allowlist rows, items=%#v diags=%#v", flagItems, flagDiags)
	}
}
