// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestAppsSkillRoutesRuntimeDomain(t *testing.T) {
	doc := readAppsSkillDoc(t, larkAppsSkillDoc)
	frontmatterEnd := strings.Index(strings.TrimPrefix(doc, "---\n"), "\n---\n")
	if !strings.HasPrefix(doc, "---\n") || frontmatterEnd < 0 {
		t.Fatal("lark-apps skill must have YAML frontmatter")
	}
	frontmatter := strings.TrimPrefix(doc, "---\n")[:frontmatterEnd]
	var metadata struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		t.Fatalf("parse lark-apps skill frontmatter: %v", err)
	}

	const runtimeDomain = "*.feishuapp.com"
	if !strings.Contains(metadata.Description, runtimeDomain) {
		t.Errorf("lark-apps description must route runtime URL %q", runtimeDomain)
	}
}
