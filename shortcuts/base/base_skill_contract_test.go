// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestBaseSkillKeepsCreateSemanticsLiteralAndGeneric(t *testing.T) {
	const skillPath = "../../skills/lark-base/SKILL.md"
	content, err := vfs.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("read lark-base skill: %v", err)
	}

	skill := string(content)
	normalizedSkill := strings.Join(strings.Fields(skill), " ")
	for _, want := range []string{
		"用户要求“新增/创建”时",
		"本轮 create 返回的对象、ID 或数量",
		"不能把已有资源算作本轮新增",
		"按具体命令或 guide 的同名契约处理",
		"不得自行改写用户语义",
		"对每类资源只做一次必要盘点",
		"只有命令明确返回逐项结果时才优先使用批量创建",
		"继续配置本轮返回的 ID",
	} {
		if !strings.Contains(normalizedSkill, want) {
			t.Fatalf("lark-base skill missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"base_table_",
		"larkoffice.com/base/",
		"grading_pass_rate",
	} {
		if strings.Contains(skill, forbidden) {
			t.Fatalf("lark-base skill must remain generic, found %q", forbidden)
		}
	}
}

func TestFieldCreateBatchContractStaysConsistentAcrossSkillAndReferences(t *testing.T) {
	readNormalized := func(path string) string {
		t.Helper()
		content, err := vfs.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return strings.Join(strings.Fields(string(content)), " ")
	}

	skill := readNormalized("../../skills/lark-base/SKILL.md")
	fieldJSON := readNormalized("../../skills/lark-base/references/lark-base-field-json.md")
	fieldCreate := readNormalized("../../skills/lark-base/references/lark-base-field-create.md")

	for _, want := range []string{
		"同一表创建多个字段时，默认一次向 `+field-create --json` 传字段对象数组",
		"预计串行运行时间超过 caller/tool timeout 时按时间预算拆分",
		"不按固定条数切块",
		"仅创建一个或多个只含 `name` + `type:text` 的简单字段时按 `+field-create --help` 即可",
		"其他类型或属性必读",
		"除上述简单 text fast path 外，写字段前先读",
		"只有命令明确返回逐项结果时才优先使用批量创建",
		"`+field-create` 数组是顺序单项请求，按 caller timeout 而非固定条数拆分",
	} {
		if !strings.Contains(skill, want) {
			t.Fatalf("lark-base skill missing %q", want)
		}
	}
	for _, want := range []string{
		"单个字段定义始终是 JSON 对象",
		"`+field-create --json` 接受一个字段对象或非空字段对象数组",
		"`+field-update --json` 只接受一个字段对象",
	} {
		if !strings.Contains(fieldJSON, want) {
			t.Fatalf("field JSON SSOT missing %q", want)
		}
	}
	if strings.Contains(fieldJSON, "`--json` 必须是 JSON 对象") {
		t.Fatal("field JSON SSOT must not apply the update-only top-level object rule to field-create")
	}
	for _, want := range []string{
		"预计串行运行时间超过 caller/tool timeout 时按时间预算拆分，不按固定条数切块",
		"遇到首个失败即停止且不自动回滚已创建字段",
		"部分失败返回 `ok:false`",
		"`summary`",
		"`items`",
		"`next_step:\"inspect_items\"`",
		"`missing_scopes`、`identity`、`console_url`",
		"`challenge_url`",
		"`retryable:true` 只表示该 `failed` 项可原样自动重试",
		"否则先按该项 `hint` 完成授权或修正输入，再重新提交该项",
		"`not_attempted` 项应单独继续",
		"调用方超时且未收到命令终态输出时，不要重投整个数组",
		"先按本次提交的字段名定向读回，再只提交缺失项",
		"没有写前快照时，读回命中的同名项只能标记为 `ambiguous`",
		"不得计作本轮 `created`",
	} {
		if !strings.Contains(fieldCreate, want) {
			t.Fatalf("field-create reference missing %q", want)
		}
	}
}
