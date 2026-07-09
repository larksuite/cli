// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillscan

import (
	"path/filepath"
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
