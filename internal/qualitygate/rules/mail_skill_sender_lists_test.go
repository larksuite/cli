// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package rules

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs"
)

func TestMailSkillRoutesBlockedSenderPromptToAtomicCommand(t *testing.T) {
	data, err := vfs.ReadFile("../../../skills/lark-mail/SKILL.md")
	if err != nil {
		t.Fatalf("read lark-mail skill: %v", err)
	}
	body := string(data)

	required := []string{
		"lark-cli mail user_mailbox.blocked_senders batch_create --as user",
		"cli-ai-block@example.test",
		"不要用 `user_mailbox.rules create`",
		"只说\"加到我的名单里\"但没说白名单或黑名单",
		"先澄清要信任还是屏蔽",
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Fatalf("lark-mail skill is missing blocked sender routing hint %q", want)
		}
	}
}

func TestMailSkillTemplateKeepsSenderListRoutingSource(t *testing.T) {
	data, err := vfs.ReadFile("../../../skill-template/domains/mail.md")
	if err != nil {
		t.Fatalf("read mail skill template: %v", err)
	}
	body := string(data)

	required := []string{
		"lark-cli mail user_mailbox.allow_senders batch_create --as user",
		"lark-cli mail user_mailbox.blocked_senders batch_create --as user",
		"`user_mailbox.rules` 仅用于",
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Fatalf("mail skill template is missing sender-list routing hint %q", want)
		}
	}
}
