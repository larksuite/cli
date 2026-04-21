// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBuildDraftSendOutputIncludesOptionalFields(t *testing.T) {
	got := buildDraftSendOutput(map[string]interface{}{
		"message_id":    "msg_001",
		"thread_id":     "thread_001",
		"recall_status": "available",
		"automation_send_disable": map[string]interface{}{
			"reason":    "Automation send is disabled by your mailbox setting",
			"reference": "https://open.larksuite.com/mail/settings/automation",
		},
	}, "me")

	if got["message_id"] != "msg_001" {
		t.Fatalf("message_id = %v", got["message_id"])
	}
	if got["thread_id"] != "thread_001" {
		t.Fatalf("thread_id = %v", got["thread_id"])
	}
	if _, ok := got["recall_status"]; ok {
		t.Fatalf("recall_status should be omitted, got %#v", got["recall_status"])
	}
	if got["recall_available"] != true {
		t.Fatalf("recall_available = %v", got["recall_available"])
	}
	if got["recall_tip"] == "" {
		t.Fatalf("recall_tip should be populated")
	}
	if _, ok := got["automation_send_disable"]; ok {
		t.Fatalf("automation_send_disable should be omitted, got %#v", got["automation_send_disable"])
	}
	if got["automation_send_disable_reason"] != "Automation send is disabled by your mailbox setting" {
		t.Fatalf("automation_send_disable_reason = %v", got["automation_send_disable_reason"])
	}
	if got["automation_send_disable_reference"] != "https://open.larksuite.com/mail/settings/automation" {
		t.Fatalf("automation_send_disable_reference = %v", got["automation_send_disable_reference"])
	}
}

func TestMailSendConfirmSendOutputsAutomationDisable(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "me@example.com",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_001",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts/draft_001/send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "msg_001",
				"thread_id":  "thread_001",
				"automation_send_disable": map[string]interface{}{
					"reason":    "Automation send is disabled by your mailbox setting",
					"reference": "https://open.larksuite.com/mail/settings/automation",
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "world",
		"--confirm-send",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["message_id"] != "msg_001" {
		t.Fatalf("message_id = %v", data["message_id"])
	}
	if data["thread_id"] != "thread_001" {
		t.Fatalf("thread_id = %v", data["thread_id"])
	}
	if _, ok := data["automation_send_disable"]; ok {
		t.Fatalf("automation_send_disable should be omitted, got %#v", data["automation_send_disable"])
	}
	if data["automation_send_disable_reason"] != "Automation send is disabled by your mailbox setting" {
		t.Fatalf("automation_send_disable_reason = %v", data["automation_send_disable_reason"])
	}
	if data["automation_send_disable_reference"] != "https://open.larksuite.com/mail/settings/automation" {
		t.Fatalf("automation_send_disable_reference = %v", data["automation_send_disable_reference"])
	}
}

func TestMailSendSaveDraftOutputsReference(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "me@example.com",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id":  "draft_001",
				"reference": "https://www.feishu.cn/mail?draftId=draft_001",
			},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "world",
	}, f, stdout)
	if err != nil {
		t.Fatalf("save draft failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["draft_id"] != "draft_001" {
		t.Fatalf("draft_id = %v", data["draft_id"])
	}
	if data["reference"] != "https://www.feishu.cn/mail?draftId=draft_001" {
		t.Fatalf("reference = %v", data["reference"])
	}
}
