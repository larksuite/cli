// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHarvestSkillCommands(t *testing.T) {
	got, err := Harvest(filepath.Join("testdata", "skills"))
	if err != nil {
		t.Fatalf("Harvest() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d commands, want 2: %#v", len(got), got)
	}
	if got[0].Raw != "lark-cli docs +fetch --api-version v2 --doc A3Ijdemo" {
		t.Fatalf("first raw = %q", got[0].Raw)
	}
	if !got[1].HasPlaceholder {
		t.Fatalf("oc_xxx should be classified as placeholder")
	}
}

func TestFilterExamplesBySkill(t *testing.T) {
	examples := []Example{
		{SourceFile: "skills/lark-doc/SKILL.md", Raw: "lark-cli docs +fetch"},
		{SourceFile: "skills/lark-im/SKILL.md", Raw: "lark-cli im chats list"},
	}
	got := FilterExamples(examples, map[string]bool{"lark-doc": true})
	if len(got) != 1 || got[0].SourceFile != "skills/lark-doc/SKILL.md" {
		t.Fatalf("FilterExamples() = %#v", got)
	}
}

func TestHasPlaceholderDistinguishesHTMLFromPlaceholders(t *testing.T) {
	if HasPlaceholder(`lark-cli mail +send --body '<p>Hello <strong>team</strong></p>'`) {
		t.Fatal("HTML tags should not make an example a placeholder")
	}
	for _, raw := range []string{
		`lark-cli slides +replace-slide --parts '[{"replacement":"<shape type=\"rect\" width=\"100\" height=\"100\"/>"}]'`,
		`lark-cli slides +replace-slide --parts '[{"replacement":"<shape type=\"text\"><content textType=\"title\"><p>Title</p></content></shape>"}]'`,
	} {
		if HasPlaceholder(raw) {
			t.Fatalf("XML tags should not make an example a placeholder: %q", raw)
		}
	}
	for _, raw := range []string{
		`lark-cli docs +fetch <doc_token>`,
		`lark-cli wiki +node-get --node-token <node_token | obj_token | Lark URL>`,
		`lark-cli whiteboard +update --whiteboard-token <画板Token>`,
		`lark-cli wiki +delete-space --space-id <SPACE_ID>`,
		`lark-cli approval <resource> <method> [flags]`,
		`lark-cli sheets <shortcut> <workbook 定位> <sheet 定位> <其它 flag>`,
		`lark-cli mail +draft-edit --draft-id <draft-id>`,
		`lark-cli vc-agent +meeting-events --meeting-id <meeting.id>`,
		`lark-cli schema <service.resource.method>`,
	} {
		if !HasPlaceholder(raw) {
			t.Fatalf("expected placeholder for %q", raw)
		}
	}
}

func TestHarvestFollowsQuotedArgumentsAcrossLines(t *testing.T) {
	// A quoted flag value continues a shell word without a trailing backslash,
	// which is how the shipped skills spell multi-line JSON. Stopping at the
	// first unescaped newline hands the caller a fragment with an unclosed
	// quote, and the example goes unvalidated.
	got, err := Harvest(filepath.Join("testdata", "multiline"))
	if err != nil {
		t.Fatalf("Harvest() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d examples, want 2: %#v", len(got), got)
	}

	for _, ex := range got {
		if hasOpenQuote(ex.Raw) {
			t.Errorf("harvested example ends inside a quote: %q", ex.Raw)
		}
	}

	if !strings.Contains(got[0].Raw, `"shaper": {"format": "flat"}`) {
		t.Errorf("single-quoted JSON was truncated: %q", got[0].Raw)
	}
	if !strings.HasSuffix(got[0].Raw, "}'") {
		t.Errorf("single-quoted value did not close: %q", got[0].Raw)
	}
	if !strings.Contains(got[1].Raw, "line one line two") {
		t.Errorf("double-quoted value was truncated: %q", got[1].Raw)
	}
}

func TestHasOpenQuote(t *testing.T) {
	tests := map[string]bool{
		`lark-cli docs +fetch --doc A3Ij`:      false,
		`lark-cli base +data-query --dsl '{`:   true,
		`lark-cli base +data-query --dsl '{}'`: false,
		`lark-cli docs +fetch --note "open`:    true,
		`lark-cli docs +fetch --note "shut"`:   false,
		// inside single quotes a backslash is literal, so the quote still closes
		`lark-cli docs +fetch --note 'a\'`: false,
		// outside quotes a backslash escapes the quote that follows
		`lark-cli docs +fetch --note \'`: false,
	}
	for raw, want := range tests {
		if got := hasOpenQuote(raw); got != want {
			t.Errorf("hasOpenQuote(%q) = %v, want %v", raw, got, want)
		}
	}
}
