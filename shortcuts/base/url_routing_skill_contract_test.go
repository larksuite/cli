// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"
)

func TestRecordShareURLSkillRoutingContract(t *testing.T) {
	baseSkill := readSkillContractFile(t, larkBaseSkillDoc)
	frontmatter := strings.SplitN(baseSkill, "---", 3)
	if len(frontmatter) != 3 || !strings.Contains(frontmatter[1], "/record/") {
		t.Fatalf("Base skill frontmatter description must route /record/ URLs to lark-base")
	}
	for _, contract := range []string{
		"`/record/<token>` 是 Base 记录分享链接",
		"`record_share_token` 不是 `record_id` 或 `minute_token`",
		"`lark-cli base +url-resolve --url '<url>' --as user`",
	} {
		if !strings.Contains(baseSkill, contract) {
			t.Fatalf("Base skill must contain record-share routing contract %q", contract)
		}
	}

	driveSkill := readSkillContractFile(t, "../../skills/lark-drive/SKILL.md")
	for _, contract := range []string{
		"URL 路径为 `/record/<token>`",
		"[`lark-base`](../lark-base/SKILL.md)",
		"不要使用 `drive +inspect`",
	} {
		if !strings.Contains(driveSkill, contract) {
			t.Fatalf("Drive skill must contain record-share routing contract %q", contract)
		}
	}

	minutesSkill := readSkillContractFile(t, "../../skills/lark-minutes/SKILL.md")
	for _, contract := range []string{
		"`/record/<token>` 是 Base 记录分享链接",
		"不是 `minute_token`",
		"[`lark-base`](../lark-base/SKILL.md)",
	} {
		if !strings.Contains(minutesSkill, contract) {
			t.Fatalf("Minutes skill must contain record-share routing contract %q", contract)
		}
	}
}
