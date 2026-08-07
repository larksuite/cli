// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestDataQueryQuickGuideCoversConditionValueShapesWithoutCaseArtifacts(t *testing.T) {
	const guidePath = "../../skills/lark-base/references/lark-base-data-query-guide.md"
	content, err := vfs.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("read data-query quick guide: %v", err)
	}
	guide := string(content)
	normalizedGuide := strings.Join(strings.Fields(guide), " ")

	for _, want := range []string{
		"Common `Condition.value` shapes",
		"`is` / `isNot`",
		"exactly one option name",
		"`isGreater`",
		"`isLess`",
		"`isEmpty`",
		"`isNotEmpty`",
		"uses `[]`",
		`["Today"]`,
		`["ExactDate","<epoch_ms>"]`,
		"Use relative date keywords only for relative requests",
		"[lark-base-data-query.md](lark-base-data-query.md)",
	} {
		if !strings.Contains(normalizedGuide, want) {
			t.Fatalf("quick guide missing %q", want)
		}
	}

	if len(content) > 6*1024 {
		t.Fatalf("quick guide grew to %d bytes; keep full DSL details in lark-base-data-query.md", len(content))
	}

	for _, forbidden := range []string{
		"base_table_",
		"bytedance.larkoffice.com/base/",
	} {
		if strings.Contains(normalizedGuide, forbidden) {
			t.Fatalf("quick guide must remain generic, found %q", forbidden)
		}
	}
}
