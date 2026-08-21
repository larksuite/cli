// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"os"
	"strings"
	"testing"
)

func TestIMSkillIdentityContract(t *testing.T) {
	// lark-shared keeps its detail in references; SKILL.md is a slim index, so
	// the identity protocol lives with the rest of the identity guidance.
	shared := readGuidanceFile(t, "../../skills/lark-shared/references/lark-shared-identity-and-permissions.md")
	for _, required := range []string{"### 身份决策协议", "**显式 actor**", "**当前 authority**", "**禁止猜测切换**"} {
		if !strings.Contains(shared, required) {
			t.Errorf("lark-shared identity reference missing %q", required)
		}
	}

	template := readGuidanceFile(t, "../../skill-template/domains/im.md")
	generated := readGuidanceFile(t, "../../skills/lark-im/SKILL.md")
	for _, required := range []string{"**Requested actor:**", "**Command constraint:**", "**Current authority:**", "**Identity continuity:**", "**No guessed switch:**"} {
		if !strings.Contains(template, required) || !strings.Contains(generated, required) {
			t.Errorf("IM identity evidence missing %q from template or generated skill", required)
		}
	}
	for _, forbidden := range []string{
		"When the sending identity is unspecified, pass `--as bot` explicitly",
		"### Identity and Token Mapping",
	} {
		if strings.Contains(template, forbidden) || strings.Contains(generated, forbidden) {
			t.Errorf("IM skill retains forbidden identity guidance %q", forbidden)
		}
	}

	for _, path := range []string{
		"../../skills/lark-im/references/lark-im-chat-identity.md",
		"../../skills/lark-im/references/lark-im-chat-update.md",
	} {
		doc := readGuidanceFile(t, path)
		if strings.Contains(doc, "Try switching identity with `--as bot` or `--as user`") {
			t.Errorf("%s retains guessed identity switch guidance", path)
		}
	}
}

func readGuidanceFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
