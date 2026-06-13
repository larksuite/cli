// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestCheckDefaultOutputWarnsListWithoutLimit(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im messages list",
		Runnable: true,
	}}}
	diags, facts := CheckDefaultOutput(m)
	if len(diags) == 0 || diags[0].Rule != "default_output" || diags[0].Action != report.ActionWarning {
		t.Fatalf("got diagnostics %#v", diags)
	}
	if diags[0].CommandPath != "im messages list" || diags[0].SubjectType != "output" {
		t.Fatalf("default output diagnostic subject = %#v", diags[0])
	}
	if len(facts) != 1 || !facts[0].IsList || facts[0].HasDefaultLimit {
		t.Fatalf("got facts %#v", facts)
	}
}

func TestCheckDefaultOutputDoesNotEmitEstimatedByteFacts(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im messages list",
		Runnable: true,
	}}}
	diags, facts := CheckDefaultOutput(m)
	for _, diag := range diags {
		if diag.Rule == "default_output_budget" {
			t.Fatalf("default_output_budget must not rely on estimated bytes: %#v", diags)
		}
	}
	data, err := json.Marshal(facts[0])
	if err != nil {
		t.Fatalf("marshal output fact: %v", err)
	}
	if strings.Contains(string(data), "default_bytes") || strings.Contains(string(data), "sample_bytes") {
		t.Fatalf("output fact must not carry estimated byte fields: %s", data)
	}
}

func TestCheckDefaultOutputDoesNotSpecialCaseGeneratedServiceListWithoutFields(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:      "mail messages list",
		Runnable:  true,
		Source:    manifest.SourceService,
		Generated: true,
		Flags:     []manifest.Flag{{Name: "page-limit", DefValue: "10"}},
	}}}
	diags, _ := CheckDefaultOutput(m)
	if len(diags) != 0 {
		t.Fatalf("generated service commands are excluded from v1 manifest and should not have a special output reject, got %#v", diags)
	}
}

func TestCheckDefaultOutputAcceptsDefaultLimit(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im messages list",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "page-size", DefValue: "20"}},
	}}}
	diags, facts := CheckDefaultOutput(m)
	if len(diags) != 0 {
		t.Fatalf("got diagnostics %#v", diags)
	}
	if len(facts) != 1 || !facts[0].HasDefaultLimit {
		t.Fatalf("got facts %#v", facts)
	}
}

func TestCheckDefaultOutputDoesNotTreatZeroDefaultAsLimit(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "im messages list",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "page-limit", DefValue: "0"}},
	}}}
	diags, facts := CheckDefaultOutput(m)
	if len(diags) == 0 || diags[0].Rule != "default_output" {
		t.Fatalf("expected missing default limit warning, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].HasDefaultLimit {
		t.Fatalf("page-limit=0 should not count as bounded default limit: %#v", facts)
	}
}

func TestDefaultOutputRejectsListWithoutLimitOrDecisionFields(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:          "im messages list",
		Runnable:      true,
		Flags:         []manifest.Flag{{Name: "fields"}},
		DefaultFields: []string{"raw_payload"},
	}}}
	diags, facts := CheckDefaultOutput(m)
	if len(diags) == 0 || diags[0].Action != report.ActionReject || diags[0].Rule != "default_output_contract" {
		t.Fatalf("expected default output reject, got %#v", diags)
	}
	if diags[0].CommandPath != "im messages list" || diags[0].SubjectType != "output" {
		t.Fatalf("default output contract diagnostic subject = %#v", diags[0])
	}
	if facts[0].HasDecisionField {
		t.Fatalf("raw_payload should not satisfy decision field family")
	}
}

func TestCheckDefaultOutputDoesNotTreatSubstringsAsDecisionFields(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:     "drive files list",
		Runnable: true,
		Flags:    []manifest.Flag{{Name: "page-size", DefValue: "20"}},
		DefaultFields: []string{
			"width",
			"filename",
			"runtime",
			"curl",
		},
	}}}
	diags, facts := CheckDefaultOutput(m)
	if len(diags) != 1 || diags[0].Action != report.ActionReject || diags[0].Rule != "default_output_contract" {
		t.Fatalf("substring-only decision fields should reject, got %#v", diags)
	}
	if len(facts) != 1 || facts[0].HasDecisionField {
		t.Fatalf("substring-only fields should not satisfy decision field family: %#v", facts)
	}
}
