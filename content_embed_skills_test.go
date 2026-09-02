// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/skillcontent"
)

func TestEmbeddedLarkAppsCreativeDesignHierarchyIsReadable(t *testing.T) {
	skillFS, err := fs.Sub(embeddedContentFS, "skills")
	if err != nil {
		t.Fatalf("open embedded skills tree: %v", err)
	}
	reader := skillcontent.New(skillFS)

	const entry = "creative-design/creative-design.md"
	raw, resolved, err := reader.ReadReference("lark-apps", entry)
	if err != nil {
		t.Fatalf("skills read lark-apps %s: %v", entry, err)
	}
	if resolved != entry {
		t.Fatalf("resolved path = %q, want %q", resolved, entry)
	}

	markdownLink := regexp.MustCompile(`\]\(([^)#]+\.md)(?:#[^)]*)?\)`)
	links := markdownLink.FindAllStringSubmatch(string(raw), -1)
	if len(links) < 10 {
		t.Fatalf("creative-design guide exposes %d Markdown links, want at least 10", len(links))
	}
	for _, match := range links {
		target := path.Clean(path.Join(path.Dir(entry), match[1]))
		if strings.HasPrefix(target, "../") {
			t.Fatalf("linked path escapes lark-apps: %q", match[1])
		}
		if _, _, err := reader.ReadReference("lark-apps", target); err != nil {
			t.Errorf("embedded creative-design link %q (%s) is unreadable: %v", match[1], target, err)
		}
	}
}

func TestEmbeddedLarkAppsCreativeDesignExcludesMachineResources(t *testing.T) {
	for _, sourceOnly := range []string{
		"skills/lark-apps/creative-design/assets/index.html",
		"skills/lark-apps/creative-design/scripts",
		"skills/lark-apps/creative-design/starter-components/deck-stage.js",
		"skills/lark-apps/creative-design/agents/fork-verifier-agent.md",
	} {
		if _, err := fs.Stat(embeddedContentFS, sourceOnly); err == nil {
			t.Errorf("source-only creative-design resource is embedded: %s", sourceOnly)
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("stat source-only resource %s: %v", sourceOnly, err)
		}
	}
}
