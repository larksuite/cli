// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"os"
	"strings"
	"testing"
)

const (
	baseSkillDoc      = "../../skills/lark-base/SKILL.md"
	agentsSkillDoc    = "../../skills/lark-agents/SKILL.md"
	baseProviderDoc   = "../../skills/lark-agents/references/providers/lark-agents-base.md"
	dataAnalysisDoc   = "../../skills/lark-base/references/lark-base-data-analysis-sop.md"
	dataQueryGuide    = "../../skills/lark-base/references/lark-base-data-query-guide.md"
	dataQueryContract = "../../skills/lark-base/references/lark-base-data-query.md"
)

func readContractDoc(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract doc %s: %v", path, err)
	}
	return string(raw)
}

func requireContractText(t *testing.T, doc, path string, values ...string) {
	t.Helper()
	for _, value := range values {
		if !strings.Contains(doc, value) {
			t.Errorf("%s must preserve routing contract %q", path, value)
		}
	}
}

func TestBaseSkillRoutesBeforeChoosingACommand(t *testing.T) {
	doc := readContractDoc(t, baseSkillDoc)
	routeAt := strings.Index(doc, "## 先路由，再执行")
	tokenAt := strings.Index(doc, "## 获取 Base Token 和所需 ID")
	commandAt := strings.Index(doc, "## CLI 快速路由（仅在判定 CLI 后）")
	if routeAt < 0 || tokenAt < 0 || commandAt < 0 || !(routeAt < tokenAt && tokenAt < commandAt) {
		t.Fatalf("routing must precede resource resolution and CLI command selection: route=%d token=%d command=%d", routeAt, tokenAt, commandAt)
	}

	requireContractText(t, doc, baseSkillDoc,
		"version: 1.3.0",
		"用户明确要求使用 Agent 或明确指定某条 Base CLI 命令时尊重其选择",
		"数据检索与分析",
		"一次新增 ≥2 个字段",
		"字段改类型、仪表盘组件等组件改类型",
		"记录新增、修改、删除；目标 ID/筛选条件和值明确的批量写入",
		"查一条记录也属于此类",
		"混合意图只要包含数据检索分析、复杂建设或类型变更，整体走 `base:assistant`",
		"建设类 Agent 请求没有现成 Base",
		"数据查询/分析没有目标 Base",
	)
	if strings.Contains(doc, "| 一次性聚合统计 | `+data-query`") {
		t.Fatal("natural-language aggregation must not route to +data-query by default")
	}
}

func TestBaseAgentHandoffUsesOnePublicAssistant(t *testing.T) {
	agentsDoc := readContractDoc(t, agentsSkillDoc)
	providerDoc := readContractDoc(t, baseProviderDoc)

	requireContractText(t, agentsDoc, agentsSkillDoc,
		"version: 1.3.1",
		"用户明确指定 Agent / `base:assistant`",
		"由 `lark-base` 先按产品规则分流",
		"首次读 Card → 校验身份/scope/参数 → `send` → `task get --watch` → 必要时 `--answer`",
	)
	requireContractText(t, providerDoc, baseProviderDoc,
		"对外只暴露统一 Base Assistant",
		"Card 只做能力与参数校验，不再次判断建设/分析类型",
		"lark-cli agents card base:assistant --operation all --as user --format json",
		"lark-cli base +url-resolve --url \"<url>\" --as user",
		"输入已经是真实 `base_token` 时",
		"结构化 `input_required` 回答",
		"Card、身份、scope 或 Assistant 服务失败时不静默改走 Base CLI",
		"`has_more` / `next_cursor` 分页 envelope",
	)

	publicDocs := agentsDoc + "\n" + providerDoc + "\n" + readContractDoc(t, baseSkillDoc)
	for _, forbidden := range []string{"Building Agent", "Analysis Agent"} {
		if strings.Contains(publicDocs, forbidden) {
			t.Errorf("public routing docs expose internal child name %q", forbidden)
		}
	}
}

func TestBaseQueryReferencesDoNotBypassAssistantRouting(t *testing.T) {
	for _, path := range []string{dataAnalysisDoc, dataQueryGuide, dataQueryContract} {
		doc := readContractDoc(t, path)
		requireContractText(t, doc, path,
			"base:assistant",
			"默认路由",
		)
	}
}
