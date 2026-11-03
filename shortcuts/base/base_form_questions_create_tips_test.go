// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"
	"testing"
)

func TestBaseFormQuestionsCreateTipsRequireExistingQuestionCheck(t *testing.T) {
	tips := strings.Join(BaseFormQuestionsCreate.Tips, "\n")
	for _, want := range []string{
		"+form-questions-list",
		"verified empty form can create directly",
		"question IDs are field IDs",
		"use_existing_field=true",
		"without creating another field",
		"explicitly requests a separate same-title question",
		"+form-questions-update",
	} {
		if !strings.Contains(tips, want) {
			t.Fatalf("tips missing %q:\n%s", want, tips)
		}
	}
}

func TestBaseFormQuestionsCreateQuestionsHelpStatesSignaturePNG(t *testing.T) {
	for _, flag := range BaseFormQuestionsCreate.Flags {
		if flag.Name != "questions" {
			continue
		}
		if !strings.Contains(flag.Desc, `{"type":"signature"},"title":"Signature"} (submissions accept PNG images only)`) {
			t.Fatalf("--questions help must state that signature submissions accept PNG only:\n%s", flag.Desc)
		}
		return
	}
	t.Fatal("--questions flag not found")
}
