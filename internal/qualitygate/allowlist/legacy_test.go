// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package allowlist

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/report"
)

func TestLegacyFlagAllowlistRequiresOwnerReasonAndAddedAt(t *testing.T) {
	raw := "docs +fetch\tapi_version\tcli-owner\tlegacy public flag\t2026-06-05\n"
	items, diags := ParseLegacyFlags(strings.NewReader(raw))
	if len(diags) != 0 || len(items) != 1 {
		t.Fatalf("parse allowlist = %#v %#v", items, diags)
	}
	if items[0].Command != "docs +fetch" || items[0].Flag != "api_version" {
		t.Fatalf("item = %#v", items[0])
	}
}

func TestLegacyFlagAllowlistRejectsExtraExpiryColumn(t *testing.T) {
	raw := "docs +fetch\tapi_version\tcli-owner\tlegacy public flag\t2026-01-01\t2026-02-01\n"
	_, diags := ParseLegacyFlags(strings.NewReader(raw))
	if len(diags) != 1 || diags[0].Action != report.ActionReject || diags[0].Rule != "allowlist_format" {
		t.Fatalf("expected format reject, got %#v", diags)
	}
}

func TestLegacyCommandAllowlistRequiresOwnerReasonAndAddedAt(t *testing.T) {
	raw := "drive +task_result\tcli-owner\tlegacy public shortcut\t2026-06-05\n"
	items, diags := ParseLegacyCommands(strings.NewReader(raw))
	if len(diags) != 0 || len(items) != 1 {
		t.Fatalf("parse command allowlist = %#v %#v", items, diags)
	}
	if items[0].Command != "drive +task_result" {
		t.Fatalf("command = %q", items[0].Command)
	}
}

func TestMalformedLegacyCommandAllowlistRejects(t *testing.T) {
	raw := "drive +task_result\tcli-owner\n"
	_, diags := ParseLegacyCommands(strings.NewReader(raw))
	if len(diags) != 1 || diags[0].Action != report.ActionReject || diags[0].Rule != "allowlist_format" {
		t.Fatalf("expected format reject, got %#v", diags)
	}
}

func TestLegacyCommandAllowlistTrimsSurroundingWhitespace(t *testing.T) {
	// Surrounding spaces around tab-separated columns must be trimmed so the
	// stored key matches exact lookups and the date still parses. Internal
	// spaces in the command name are preserved.
	raw := "  drive +task_result \t cli-owner \t legacy public shortcut \t 2026-06-05 \n"
	items, diags := ParseLegacyCommands(strings.NewReader(raw))
	if len(diags) != 0 || len(items) != 1 {
		t.Fatalf("expected one clean row, items=%#v diags=%#v", items, diags)
	}
	if items[0].Command != "drive +task_result" || items[0].Owner != "cli-owner" || items[0].Reason != "legacy public shortcut" {
		t.Fatalf("columns not trimmed: %#v", items[0])
	}
}
