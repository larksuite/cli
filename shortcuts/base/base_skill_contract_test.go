// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"os"
	"strings"
	"testing"
)

const larkBaseSkillDoc = "../../skills/lark-base/SKILL.md"

func TestBaseSkillContract_ReusedBlockConfigPreservesExplicitIntent(t *testing.T) {
	raw, err := os.ReadFile(larkBaseSkillDoc)
	if err != nil {
		t.Fatalf("read skill doc %s: %v", larkBaseSkillDoc, err)
	}
	doc := string(raw)
	start := strings.Index(doc, "- BaseApp（应用模式）")
	end := strings.Index(doc, "- Workspace 创建结果")
	if start < 0 || end <= start {
		t.Fatalf("missing BaseApp routing section in %s", larkBaseSkillDoc)
	}
	section := doc[start:end]
	for _, contract := range []string{
		"只能作为结构模板",
		"首次 Create/Update 前仍要逐项对齐用户显式要求",
		"`group_by[].sort.order`",
		"顶层 `sort.order`",
		"不能用旧配置省略的方向",
		"`get-data` 结果顺序代替",
	} {
		if !strings.Contains(section, contract) {
			t.Fatalf("BaseApp routing must contain %q:\n%s", contract, section)
		}
	}
}
