// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"
)

func TestEmbeddedHTMLSkill(t *testing.T) {
	for _, path := range []string{"skills/lark-html/SKILL.md", "skills/lark-html/references/lark-html-publish.md", "skills/lark-html/references/lark-html-fetch.md", "affordance/html.md"} {
		data, err := embeddedContentFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Fatalf("embedded %s is empty", path)
		}
	}
	data, err := embeddedContentFS.ReadFile("skills/lark-html/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"references/lark-html-publish.md", "references/lark-html-fetch.md"} {
		if !strings.Contains(string(data), name) {
			t.Errorf("skill is missing link %s", name)
		}
	}
}
