// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const automationSkillDoc = "../../skills/lark-apps/references/lark-apps-automation.md"
const localDevSkillDoc = "../../skills/lark-apps/references/lark-apps-local-dev.md"
const larkAppsSkillDoc = "../../skills/lark-apps/SKILL.md"
const releaseGetSkillDoc = "../../skills/lark-apps/references/lark-apps-release-get.md"

func readAutomationSkillDoc(t *testing.T) string {
	return readAppsSkillDoc(t, automationSkillDoc)
}

func readLocalDevSkillDoc(t *testing.T) string {
	return readAppsSkillDoc(t, localDevSkillDoc)
}

func readReleaseGetSkillDoc(t *testing.T) string {
	return readAppsSkillDoc(t, releaseGetSkillDoc)
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

func requireFirstOccurrencesInOrder(t *testing.T, text string, tokens ...string) {
	t.Helper()
	previous := -1
	for _, token := range tokens {
		idx := strings.Index(text, token)
		if idx < 0 {
			t.Fatalf("missing %q", token)
		}
		if idx <= previous {
			t.Fatalf("first %q at %d must follow the previous contract token at %d", token, idx, previous)
		}
		previous = idx
	}
}

func TestAutomationSkillContract_ChangedHandlerStartWaitsForThisRelease(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 实现或更新 handler 后发布并启动/测试")

	requireInOrder(t, section,
		"仅当本轮确实需要新增或修改 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` handler",
		"+automation-get",
		"记录发布前状态",
		"--name",
		"项目 guide",
		"按项目 guide 完成同名业务 handler 并本地验证。",
		"在 Git 已确认/预授权时 commit，然后执行",
		"git push origin sprint/default",
		"临时停用授权",
		"+automation-disable",
		"确认 disabled",
		"+release-create --branch sprint/default",
		"data.release_id",
		"+release-get",
		"data.status=finished",
		"仅启动",
		"+automation-enable",
		"+automation-get",
		"不制造 runtime probe",
		"测试",
		"运行时验证的操作级授权",
		"完成全部 preflight",
		"才执行 `+automation-enable`",
		"真实 runtime",
		"仅要求测试",
		"恢复到发布前状态",
	)
	requireFirstOccurrencesInOrder(t, section,
		"+automation-get",
		"git push origin sprint/default",
		"临时停用授权",
		"+automation-disable",
		"+release-create --branch sprint/default",
		"data.status=finished",
		"仅启动",
	)
	for _, boundary := range []string{
		"仅当本轮确实需要新增或修改 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` handler，且用户要求把这次代码发布后启动或测试时，才使用此路径。",
		"按项目 guide 完成同名业务 handler 并本地验证。",
		"在 Git 已确认/预授权时 commit，然后执行 `git push origin sprint/default`。",
		"若该命令本身返回错误或未返回 `data.release_id`：视为确认未创建本轮 release（新代码未上线），原本 enabled 的 trigger 恢复 enabled 并回读、原本 disabled 的保持 disabled 后停止；若因超时等导致结果未知，保持 disabled，先用 `+release-list --status finished --page-size 1` 核对是否已产生新 release 再决定。",
		"只有 `data.status=finished` 才能继续；`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟。",
		"确认 `failed` 时报告发布失败，原本 enabled 的 trigger 仅在确认新代码未上线后恢复 enabled，原本 disabled 的保持 disabled。",
		"发布状态仍不确定时不得进入 enable、probe 或状态恢复分支。",
		"**仅启动**：取得持续启动授权后执行 `+automation-enable`，并用 `+automation-get` 确认 enabled；到此结束，不制造 runtime probe。",
		"**测试（含“启动并测试”）**：先按下节“运行时验证的操作级授权”完成全部 preflight",
		"若用户仅要求测试而不是持续启动，只在本轮 release 已 `finished` 且 probe 成功后恢复到发布前状态",
		"无论用户是仅测试还是启动并测试，probe 失败、结果不确定或 enable 后提前结束时，一律 `+automation-disable` 并回读 disabled",
		"不得把“发布前 enabled”当作失败后的恢复依据",
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

func TestAutomationSkillContract_EnableExistingTriggerDoesNotPublish(t *testing.T) {
	doc := readAutomationSkillDoc(t)
	section := skillSubsection(t, doc, "### 仅启用已有 disabled trigger")
	routeSection := skillSection(t, doc, "## 何时用本 skill（路由锚点）")

	requireInOrder(t, section,
		"用户只要求启用已存在且 disabled 的 trigger",
		"+automation-get",
		"+release-list --status finished --page-size 1",
		"已完成线上 release",
		"当前线上应用",
		"不能证明该 trigger name 已绑定 handler",
		"+automation-enable",
		"+automation-get",
		"不得修改 handler、commit/push 或 release",
		"对 UPSERT 或 feishu-approval 只改变配置状态",
	)
	if !strings.Contains(section, "未发布时不得自动创建 release，也不得声称 trigger 已开始实际运行") {
		t.Error("enable-only flow must distinguish configuration enablement from a published runtime")
	}
	if !strings.Contains(section, "即使存在 finished release，也只能把 enable 报告为配置激活") {
		t.Error("enable-only flow must not infer handler provenance from app release history")
	}
	if strings.Contains(section, "apps +get") || strings.Contains(section, "`is_published`") {
		t.Error("enable-only flow must use finished release history instead of an optional app detail field")
	}
	for _, forbidden := range []string{"git push", "+release-create"} {
		if strings.Contains(section, forbidden) {
			t.Errorf("enable-only flow must not contain %q", forbidden)
		}
	}
	if !strings.Contains(routeSection, "「启用 / 启动已有 trigger」→ 先核对现有状态；只启用时不要修改源码或发布应用。") {
		t.Error("routing anchors must keep existing-trigger enablement separate from code release")
	}
}

func TestAutomationSkillContract_TestExistingTriggerDoesNotPublish(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 测试已有线上 trigger（不改代码）")

	requireInOrder(t, section,
		"用户要求测试已经发布的 trigger",
		"+automation-get",
		"+release-list --status finished --page-size 1",
		"当前线上代码",
		"不得为测试自动修改源码、commit/push 或 release",
		"在任何临时 enable 之前完成",
		"测试请求已明确包含临时 enable，或另行取得 enable 授权",
		"运行时验证的操作级授权",
		"无论 probe 成功、失败、结果不确定，还是临时 enable 后提前结束或中断，最终都必须 `+automation-disable` 并回读 disabled",
	)
	for _, forbidden := range []string{"git push", "+release-create"} {
		if strings.Contains(section, forbidden) {
			t.Errorf("existing-trigger test flow must not contain %q", forbidden)
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

func TestAutomationSkillContract_HandlerOnlyExcludesUnverifiedRuntimeTypes(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 仅完成 handler（不发布/不启用）")

	if !strings.Contains(section, "仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。") {
		t.Error("handler-only flow must exclude UPSERT and feishu-approval without a verified runtime contract")
	}
}

func TestAutomationSkillContract_PublishedHandlerStaysDisabled(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 把 handler 发布好，但先不要启动")

	for _, boundary := range []string{
		"仅对 cron、webhook、record-change 的 `INSERT`、`UPDATE`、`DELETE` 使用此路径。",
		"先用 `+automation-get` 定位；不存在时用 `+automation-create` 创建同名 disabled trigger，再次回读确认。",
		"已存在时记录它是否 enabled。",
		"若 trigger 已 enabled，先说明发布前必须临时停用以及可能造成的运行中断，并取得这次临时停用授权；未获授权时停止在发布前。",
		"取得授权后，在发布前执行 `+automation-disable`，并再次用 `+automation-get` 确认 disabled。",
		"按项目 guide 完成同名业务 handler 并本地验证后，commit、`git push origin sprint/default`。",
		"随后发布完整应用：",
		"若 `+release-create` 本身返回错误或未返回 `data.release_id`：视为确认未创建本轮 release（新代码未上线），原本 enabled 的 trigger 恢复 enabled 并回读、原本 disabled 的保持 disabled，然后停止；若因超时等导致创建结果未知，保持 disabled，先用 `+release-list --status finished --page-size 1` 核对是否已产生新 release 再决定。",
		"取得 `data.release_id` 后，对**这一轮** ID 调用 `+release-get`：`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时且状态仍不确定时报告 `release_id` 和当前 status，并保持 disabled；只有 `data.status=finished` 才算完成。",
		"确认 `failed` 且新代码未上线时，原本 enabled 的 trigger 恢复 enabled 并回读，原本 disabled 的保持 disabled。",
		"release 是整个应用上线，可能影响既有线上功能；未获得启动或测试授权时，finished 后始终保持 disabled，不执行 `+automation-enable`。",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("publish-without-start section must preserve %q", boundary)
		}
	}
	requireFirstOccurrencesInOrder(t, section,
		"+automation-get",
		"git push origin sprint/default",
		"临时停用授权",
		"+automation-disable",
		"+release-create",
	)
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

func TestAutomationSkillContract_RuntimeProbeRequiresOperationScope(t *testing.T) {
	section := skillSubsection(t, readAutomationSkillDoc(t), "### 运行时验证的操作级授权")

	for _, boundary := range []string{
		"启用 trigger 的授权不等于制造 runtime 事件的授权，测试授权也不等于任意数据库写入授权。",
		"record-change 在执行任何 DML 前，必须明确并取得覆盖以下作用域的授权",
		"环境、表、操作、精确测试记录或筛选条件、payload、预期结果和清理方式",
		"优先使用专用测试记录",
		"`DELETE`",
		"[lark-apps-db-execute.md](lark-apps-db-execute.md)",
		"先 `SELECT count(*)`、执行 `--dry-run`",
		"取得针对该删除目标的明确授权",
		"+automation-list --trigger-type record-change --all",
		"同一环境、表和操作可能命中的其他 enabled trigger",
		"聚合业务影响",
		"恢复 UPDATE 或清理 INSERT 也可能再次触发自动化",
		"缺少安全、已授权且可清理的事件入口时，记录 blocked",
	} {
		if !strings.Contains(section, boundary) {
			t.Errorf("runtime probe section must preserve %q", boundary)
		}
	}
}

func TestAutomationSkillContract_UsesResolvableSharedSkillLink(t *testing.T) {
	doc := readAutomationSkillDoc(t)

	if strings.Contains(doc, "](../lark-shared/SKILL.md)") {
		t.Error("automation reference must not resolve lark-shared inside the lark-apps directory")
	}
	if !strings.Contains(doc, "](../../lark-shared/SKILL.md)") {
		t.Error("automation reference must link to the sibling lark-shared skill")
	}
	sharedSkillDoc := filepath.Clean(filepath.Join(filepath.Dir(automationSkillDoc), "../../lark-shared/SKILL.md"))
	if _, err := os.Stat(sharedSkillDoc); err != nil {
		t.Fatalf("automation reference target %s must exist: %v", sharedSkillDoc, err)
	}
}

func TestAppsSkillContract_AllSharedSkillLinksResolve(t *testing.T) {
	docs := []string{larkAppsSkillDoc}
	references, err := filepath.Glob("../../skills/lark-apps/references/*.md")
	if err != nil {
		t.Fatalf("glob lark-apps references: %v", err)
	}
	docs = append(docs, references...)
	sharedLink := regexp.MustCompile(`\]\(([^)]+lark-shared/SKILL\.md)\)`)

	for _, docPath := range docs {
		doc := readAppsSkillDoc(t, docPath)
		for _, match := range sharedLink.FindAllStringSubmatch(doc, -1) {
			target := filepath.Clean(filepath.Join(filepath.Dir(docPath), match[1]))
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s shared-skill link %q resolves to missing target %s: %v", docPath, match[1], target, err)
			}
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
	for _, implementationShape := range []string{
		"npx ", "skills sync", "data.", "skills_", "_CACHE_DIR", "nestjs-",
		"@lark-apaas/miaoda-cli", "@lark-apaas/coding-steering", "miaoda-coding", "skills_common/",
	} {
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
		for _, implementationShape := range []string{
			"npx ", "skills sync", "@lark-apaas/miaoda-cli", "@lark-apaas/coding-steering", "miaoda-coding", "skills_common/",
		} {
			if strings.Contains(doc, implementationShape) {
				t.Errorf("%s skill must not expose project-sync implementation shape %q", name, implementationShape)
			}
		}
	}
}

func TestLocalDevSkillContract_UsesEnvironmentAndDefersEnableToAutomationSOP(t *testing.T) {
	doc := readLocalDevSkillDoc(t)
	releaseSection := skillSection(t, doc, "## 改完代码后部署上线")
	for _, legacy := range []string{"--env dev", "--env online"} {
		if strings.Contains(doc, legacy) {
			t.Errorf("local-dev skill must not recommend legacy %q", legacy)
		}
	}
	for _, boundary := range []string{
		"`publishing` 时每 20 秒继续轮询，整体最多约 5 分钟；超时仍未完成时停止本轮轮询、报告 `release_id` 和当前 status。",
		"若本次改动包含自动化 handler，在执行本节通用 commit/push/release 序列前就转到 [automation SOP](lark-apps-automation.md) 的匹配路径，由该 SOP 负责完整的状态门禁、commit/push、release 和可选 enable/test；不要先按本节发布再补 trigger 状态检查。",
		"用户只要求启用已有 trigger 时，转到 [automation SOP 的「仅启用已有 disabled trigger」路径](lark-apps-automation.md#仅启用已有-disabled-trigger)；不得因 enable 反向修改 handler、commit/push 或 release。",
		"使用 `--environment dev|online`，不要使用旧的 `--env`。只有确认应用已开启多环境时才引导 `--environment dev`；单环境应用省略 `--environment`（服务端选 online）或显式传 `--environment online`。",
	} {
		if !strings.Contains(doc, boundary) {
			t.Errorf("local-dev skill must preserve %q", boundary)
		}
	}
	routeIndex := strings.Index(releaseSection, "若本次改动包含自动化 handler")
	releaseIndex := strings.Index(releaseSection, "+release-create")
	if routeIndex < 0 || releaseIndex < 0 || routeIndex >= releaseIndex {
		t.Error("automation routing must appear before the generic release sequence")
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

func TestLocalDevSkillContract_TreatsErrorLogsAsOptional(t *testing.T) {
	section := skillSection(t, readLocalDevSkillDoc(t), "## 改完代码后部署上线")

	if !strings.Contains(section, "`failed` 时若返回非空 `error_logs`，据此给出失败原因；否则只报告 `release_id` 和当前 status，不要编造原因") {
		t.Error("release guidance must not promise error_logs on every failed release")
	}
}

func TestReleaseSkillContract_TreatsOptionalOutputAsOptional(t *testing.T) {
	releaseGet := readReleaseGetSkillDoc(t)
	for _, boundary := range []string{
		"`finished` 后才可能有 `online_url`。",
		"若输出含 `online_url`，直接读取它作为本轮发布的线上访问链接；未返回时只报告发布完成，不要编造链接。",
		"若输出含 `error_logs`（`step`/`error_log`），据此向用户转述关键失败步骤和可行动修复；未返回时不要编造失败原因。",
	} {
		if !strings.Contains(releaseGet, boundary) {
			t.Errorf("release-get skill must preserve optional-output boundary %q", boundary)
		}
	}
}
