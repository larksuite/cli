// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

const larkBaseSkillDoc = "../../skills/lark-base/SKILL.md"

func readSkillContractFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := vfs.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill doc %s: %v", path, err)
	}
	return string(raw)
}

func TestBaseSkillContract_ReusedBlockConfigPreservesExplicitIntent(t *testing.T) {
	doc := readSkillContractFile(t, larkBaseSkillDoc)
	start := strings.Index(doc, "- BaseApp（应用模式）")
	end := strings.Index(doc, "- 应用页面的 block")
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

func TestBaseSkillContract_AppModeConceptsAndDataConfigRelationship(t *testing.T) {
	skill := readSkillContractFile(t, larkBaseSkillDoc)
	for _, contract := range []string{
		"## 应用模式与 Workspace 心智模型",
		"Workspace 是组织 Base 和 BaseApp 的空间容器",
		"Workspace 负责资源归属，App 负责页面和组件，Base 负责数据",
	} {
		if !strings.Contains(skill, contract) {
			t.Fatalf("Base skill must contain %q", contract)
		}
	}

	appConfig := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-app-block-data-config.md")
	for _, contract := range []string{
		"复用 [Dashboard Block 配置](lark-base-dashboard-block-config.md)",
		"列表组件是 App 独有协议",
		"所有列表 subtype 均可使用",
		"不能把 `filter` 提到顶层",
	} {
		if !strings.Contains(appConfig, contract) {
			t.Fatalf("App block config reference must contain %q", contract)
		}
	}

	dashboardConfig := readSkillContractFile(t, "../../skills/lark-base/references/lark-base-dashboard-block-config.md")
	for _, contract := range []string{
		"复用本文的字段取值、筛选、分组、排序及规范化规则",
		"`isGreaterEqual` / `isLessEqual` 不是全局不支持",
		"可用于 `number`，但不能用于 `datetime`",
	} {
		if !strings.Contains(dashboardConfig, contract) {
			t.Fatalf("Dashboard block config reference must contain %q", contract)
		}
	}
}
