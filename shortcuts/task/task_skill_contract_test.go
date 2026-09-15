// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"os"
	"strings"
	"testing"
)

func TestTaskSkillUsesRuntimeDiscoveryInsteadOfStaticMetaInventory(t *testing.T) {
	raw, err := os.ReadFile("../../skills/lark-task/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)

	for _, required := range []string{
		"--print-schema --flag-name <flag>",
		"字段不在 schema 中",
		"重新选择 shortcut",
		"lark-cli schema task.<resource>.<method>",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("lark-task discovery guidance missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"## API Resources",
		"## 权限表",
		"### sections",
		"### custom_fields",
		"### custom_field_options",
	} {
		if strings.Contains(content, forbidden) {
			t.Errorf("lark-task duplicates Meta-owned inventory %q", forbidden)
		}
	}
}
