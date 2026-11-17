// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
)

func TestAppsOwnerTransferSkillMemberVerification(t *testing.T) {
	doc := readAppsSkillDoc(t, "../../skills/lark-apps/references/lark-apps-owner-transfer.md")
	for _, want := range []string{
		"--type apps --as user",
		"`+member-list` 不接受 `miaoda_apps`",
		"无法核验时将权限核验结论标为 partial",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("owner transfer guidance missing %q", want)
		}
	}
	if strings.Contains(doc, "--type '<meta_doc_type>'") {
		t.Error("member-list example must not forward unsupported resource types")
	}
}
