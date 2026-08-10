// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

func TestTaskDownloadAttachmentAffordanceTracesToSkill(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	if got, ok := DomainSkill("task"); !ok || got != "lark-task" {
		t.Fatalf("DomainSkill(task) = (%q, %v), want (lark-task, true)", got, ok)
	}
	raw, ok := For("task", "+download-attachment")
	if !ok {
		t.Fatal("For(task, +download-attachment) ok=false")
	}
	affordance, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatal("task +download-attachment affordance did not parse")
	}
	const command = "lark-cli task +download-attachment --attachment-guid <attachment_guid> --output ./downloads/"
	if len(affordance.Examples) != 1 || affordance.Examples[0].Command != command {
		t.Fatalf("examples = %#v, want one command %q", affordance.Examples, command)
	}
	for _, skill := range []string{"lark-task", "lark-task/references/lark-task-download-attachment.md"} {
		if !containsExact(affordance.Skills, skill) {
			t.Fatalf("skills = %v, want %s", affordance.Skills, skill)
		}
	}
	source, err := os.ReadFile("../../skills/lark-task/references/lark-task-download-attachment.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compactSkillText(string(source)), compactSkillText(command)) {
		t.Fatalf("skill reference does not contain affordance command %q", command)
	}
}
