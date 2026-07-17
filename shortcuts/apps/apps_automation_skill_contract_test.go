// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"strings"
	"testing"
)

const automationSkillDoc = "../../skills/lark-apps/references/lark-apps-automation.md"
const localDevSkillDoc = "../../skills/lark-apps/references/lark-apps-local-dev.md"

func readAutomationSkillDoc(t *testing.T) string {
	return readAppsSkillDoc(t, automationSkillDoc)
}

func readLocalDevSkillDoc(t *testing.T) string {
	return readAppsSkillDoc(t, localDevSkillDoc)
}

func readAppsSkillDoc(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill doc %s: %v", path, err)
	}
	return string(raw)
}

func skillSection(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("missing skill section %q", heading)
	}
	rest := doc[start+len(heading):]
	if next := strings.Index(rest, "\n## "); next >= 0 {
		return rest[:next]
	}
	return rest
}

func skillSubsection(t *testing.T, doc, heading string) string {
	t.Helper()
	start := strings.Index(doc, heading)
	if start < 0 {
		t.Fatalf("missing skill subsection %q", heading)
	}
	rest := doc[start+len(heading):]
	end := len(rest)
	for _, marker := range []string{"\n### ", "\n## "} {
		if next := strings.Index(rest, marker); next >= 0 && next < end {
			end = next
		}
	}
	return rest[:end]
}

func requireInOrder(t *testing.T, text string, tokens ...string) {
	t.Helper()
	offset := 0
	for _, token := range tokens {
		idx := strings.Index(text[offset:], token)
		if idx < 0 {
			t.Fatalf("missing %q after %q", token, text[:offset])
		}
		offset += idx + len(token)
	}
}

func TestAutomationSkillContract_CompleteStartWaitsForThisRelease(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 完成并启动/启用/测试")

	requireInOrder(t, section,
		"仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。",
		"disabled",
		"--name",
		"项目 guide",
		"按项目 guide 完成同名业务 handler 并本地验证。",
		"在 Git 已确认/预授权时 commit，然后执行",
		"git push origin sprint/default",
		"+release-create --branch sprint/default",
		"data.release_id",
		"+release-get",
		"data.status=finished",
		"+automation-enable",
		"+automation-get",
		"真实 runtime",
	)
	for _, boundary := range []string{
		"仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。",
		"按项目 guide 完成同名业务 handler 并本地验证。",
		"在 Git 已确认/预授权时 commit，然后执行 `git push origin sprint/default`。",
		"只有 `data.status=finished` 才能继续；`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status，并保持 disabled；`failed` 时报告发布失败并保持 disabled。`is_published=true` 不能代替这轮发布完成。",
		"获得启动/测试授权后才执行 `+automation-enable`，并用 `+automation-get` 确认 enabled。",
		"没有通用的 `automation-debug` 或 trigger 日志 shortcut。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("complete-start section must explain %q boundary", boundary)
		}
	}
}

func TestAutomationSkillContract_BindsTheExactNameAsUser(t *testing.T) {
	doc := readAutomationSkillDoc(t)
	for _, boundary := range []string{
		"全部操作需 `--as user`（AuthType: user）。",
		"当用户希望触发器实际执行业务代码时，先确认当前工作区是已初始化的应用项目，并读取其中与触发器任务匹配的 guide。",
		"`--name` 是应用内唯一的 trigger 定位键；代码侧绑定名称必须与它逐字相同。不得用 trigger ID 或方法名代替它。具体 handler 语法和接入方式以项目 guide 为准。",
	} {
		if !strings.Contains(doc, boundary) {
			t.Errorf("automation skill must preserve %q", boundary)
		}
	}
}

func TestAutomationSkillContract_RoutesAndDiagnosesUnfiredTriggers(t *testing.T) {
	doc := readAutomationSkillDoc(t)
	routeSection := skillSection(t, doc, "## 何时用本 skill（路由锚点）")
	errorSection := skillSection(t, doc, "## 常见错误与决策场景")

	if !strings.Contains(routeSection, "「触发器没反应 / enable 了不触发 / 为什么没执行 / 验证一下触发器」→ 先按「未触发时的诊断顺序」诊断；对 UPSERT 和 feishu-approval 仅验证配置边界，不承诺 handler 或 live 验证。") {
		t.Error("routing anchors must direct unfired triggers to the bounded diagnostic flow")
	}
	if !strings.Contains(errorSection, "已证实的 cron、webhook、record-change（INSERT/UPDATE/DELETE）按「未触发时的诊断顺序」排查；UPSERT 和 feishu-approval 仅核对配置边界，不承诺 handler 或 live 验证。") {
		t.Error("error table must preserve the bounded unfired-trigger diagnostic flow")
	}
}

func TestAutomationSkillContract_ConfigurationStopsDisabled(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 仅创建/配置触发器")

	for _, boundary := range []string{
		"用 `+automation-create` 创建，并省略 `--status` 或显式传 `disabled`，然后报告 name 和 disabled 状态。",
		"不要传 `--status enabled`，也不要写 handler、commit/push、release 或 enable；更不能把创建 API 成功称为“可运行”。",
		"默认 disabled 是这个意图的终点，不是稍后自动 enable 的待办。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("configuration-only section must preserve %q", boundary)
		}
	}
}

func TestAutomationSkillContract_HandlerOnlyStopsBeforeRelease(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 仅完成 handler（不发布/不启用）")

	for _, boundary := range []string{
		"创建或定位已明确 name 的 disabled trigger，读取项目 guide，按其要求实现同名业务 handler，完成本地验证。",
		"只在既有 Git 确认或预授权下 commit/push；停止在 `+release-create` 和 `+automation-enable` 之前。",
		"用户没有明确“发布好”时，先问，不能默认把完整应用上线。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("handler-only section must preserve %q", boundary)
		}
	}
}

func TestAutomationSkillContract_PublishedHandlerStaysDisabled(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 把 handler 发布好，但先不要启动")

	for _, boundary := range []string{
		"仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。",
		"按项目 guide 完成同名业务 handler 并本地验证后，commit、`git push origin sprint/default`，再发布完整应用：",
		"保存返回的 `data.release_id`，对**这一轮** ID 调用 `+release-get`：`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status，并保持 disabled；只有 `data.status=finished` 才算完成；`failed` 时报告发布失败、保持 disabled，修复并重新发布后再取得新的 finished 结果。",
		"release 是整个应用上线，可能影响既有线上功能；未获得启动或测试授权时，始终保持 disabled，不执行 `+automation-enable`。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("publish-without-start section must preserve %q", boundary)
		}
	}
}

func TestAutomationSkillContract_UPSERTAndApprovalStayConfigurationOnly(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### UPSERT 与飞书审批边界")

	for _, boundary := range []string{
		"record-change 的 UPSERT 可创建 disabled 配置，但当前没有已证实的运行时代码契约；不得静默按 UPDATE 处理，也不得承诺 handler 或 live 验证。",
		"feishu-approval 可创建 disabled 配置，并读取或更新 `event_type`、对应 status 和可选 `approval_code`。",
		"当前没有已证实的运行时 handler 契约或实际投递验证；不要把 enable 或审批 API 成功称为业务代码已执行。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("UPSERT/approval boundary section must preserve %q", boundary)
		}
	}
}

func TestLocalDevSkillContract_UsesProjectGuideWithoutSyncInternals(t *testing.T) {
	section := skillSection(t, readLocalDevSkillDoc(t), "## Trigger guide 的项目边界")

	for _, boundary := range []string{
		"先查看工作区 `.agents/skills/`，读取与自动化任务匹配的 `trigger-guide`。",
		"文件缺失或不能覆盖当前任务时，报告项目缺少可用的领域 guide；不要在本 lark-cli reference 中猜测安装命令、版本或包内目录。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("trigger-guide boundary section must explain %q", boundary)
		}
	}
	for _, implementationShape := range []string{"npx ", "skills sync", "data.", "skills_", "_CACHE_DIR", "nestjs-"} {
		if strings.Contains(section, implementationShape) {
			t.Errorf("local-dev skill must not expose project-sync implementation shape %q", implementationShape)
		}
	}
}

func TestAppsSkillContract_DoesNotExposeSteeringImplementation(t *testing.T) {
	for name, doc := range map[string]string{
		"automation": readAutomationSkillDoc(t),
		"local-dev":  readLocalDevSkillDoc(t),
	} {
		for _, implementationShape := range []string{"npx ", "skills sync"} {
			if strings.Contains(doc, implementationShape) {
				t.Errorf("%s skill must not expose project-sync implementation shape %q", name, implementationShape)
			}
		}
	}
}

func TestLocalDevSkillContract_UsesEnvironmentAndDefersEnableToAutomationSOP(t *testing.T) {
	doc := readLocalDevSkillDoc(t)
	for _, legacy := range []string{"--env dev", "--env online"} {
		if strings.Contains(doc, legacy) {
			t.Errorf("local-dev skill must not recommend legacy %q", legacy)
		}
	}
	for _, boundary := range []string{
		"`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status。",
		"若本次发布包含自动化 handler，继续读取 [automation SOP](lark-apps-automation.md)。enable 不能替代 commit/push/release，也绝不能发生在本轮 release finished 之前；只有用户明确要求启动、启用或测试时才在该门槛后启用 trigger。",
		"使用 `--environment dev|online`，不要使用旧的 `--env`。只有确认应用已开启多环境时才引导 `--environment dev`；单环境应用省略 `--environment`（服务端选 online）或显式传 `--environment online`。",
	} {
		if !strings.Contains(doc, boundary) {
			t.Errorf("local-dev skill must preserve %q", boundary)
		}
	}
}

func TestLocalDevSkillContract_DoesNotRequireOnlineURL(t *testing.T) {
	section := skillSection(t, readLocalDevSkillDoc(t), "## 改完代码后部署上线")

	if strings.Contains(section, "`finished` 成功时该命令输出已含 `online_url`") {
		t.Error("release guidance must not claim every finished release includes online_url")
	}
	if !strings.Contains(section, "若返回 `online_url`，可直接使用；未返回时不要编造链接。") {
		t.Error("release guidance must explain that online_url is optional")
	}
}
