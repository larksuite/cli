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
		"disabled",
		"--name",
		"trigger-guide",
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
		"`--name` 是应用内唯一的 trigger 定位键，也是 `@BindTrigger('<exact-name>')` 的代码绑定字符串。不得用 trigger ID 或方法名代替它。",
	} {
		if !strings.Contains(doc, boundary) {
			t.Errorf("automation skill must preserve %q", boundary)
		}
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
		"创建或定位已明确 name 的 disabled trigger，读取同步的 `trigger-guide`，实现并注册同名 handler，完成本地验证。",
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
		"record-change 的 UPSERT 可创建 disabled 配置，但现有 runtime `DataChangeEventInput.type` 只定义 INSERT、UPDATE、DELETE；不得静默按 UPDATE 处理，也不得承诺 handler payload 或 live 验证。",
		"feishu-approval 可创建 disabled 配置，并读取或更新 `event_type`、对应 status 和可选 `approval_code`。",
		"当前没有已证实的 `TaskHandlerArgs.content.input` schema、`@BindTrigger` handler 或实际投递验证；不要把 enable 或审批 API 成功称为业务代码已执行。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("UPSERT/approval boundary section must preserve %q", boundary)
		}
	}
}

func TestLocalDevSkillContract_UsesPinnedTriggerGuideSync(t *testing.T) {
	section := skillSection(t, readLocalDevSkillDoc(t), "## Trigger guide 同步与版本证明")

	requireInOrder(t, section,
		".spark/meta.json",
		"stack=nestjs-react-fullstack",
		".agents/skills/trigger-guide/SKILL.md",
		"npx -y @lark-apaas/miaoda-cli@<cli-version> skills sync --local --version <published-coding-steering-version>",
		"exit 0",
		"data.stack",
		"data.version",
		"data.syncedSkills",
		"skills_common/trigger-guide",
		".agents/skills/trigger-guide/SKILL.md",
	)
	for _, boundary := range []string{
		"`data.syncedSkills` 包含 `nestjs-react-fullstack/skills_common/trigger-guide`，且不包含旧的 `nestjs-react-fullstack/skills/trigger-guide`；",
		"`+init` 只是一条初始化流程，**不等于版本证明**；浮动 `@latest` 也不能证明当前项目安装了目标版本。",
		"普通本地验证应保持 `MIAODA_DEP_CACHE_DIR` 未设置或为空，配合 `--local` 生成 flat `.agents/skills/` 输出。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("trigger-guide sync section must explain %q boundary", boundary)
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
