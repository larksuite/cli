// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedBaseLinkageGuideReachable(t *testing.T) {
	const guide = "lark-base-dashboard-linkage.md"
	for _, route := range []struct{ path, link string }{
		{"skills/lark-base/SKILL.md", "](references/lark-base-dashboard.md)"},
		{"skills/lark-base/references/lark-base-dashboard.md", "](" + guide + ")"},
		{"affordance/base.md", "`lark-base/references/" + guide + "`"},
	} {
		content, err := embeddedContentFS.ReadFile(route.path)
		if err != nil || !strings.Contains(string(content), route.link) {
			t.Fatalf("%s does not contain route %s: %v", route.path, route.link, err)
		}
	}
	content, err := embeddedContentFS.ReadFile("skills/lark-base/references/" + guide)
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range regexp.MustCompile(`\]\(([^)]+\.md)\)`).FindAllStringSubmatch(string(content), -1) {
		if _, err := fs.Stat(embeddedContentFS, "skills/lark-base/references/"+link[1]); err != nil {
			t.Errorf("guide has an unshipped reference %s: %v", link[1], err)
		}
	}
}
