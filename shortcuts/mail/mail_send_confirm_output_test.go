// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/httpmock"
)

func grantMailSendScope(t *testing.T) {
	t.Helper()

	cfg := mailTestConfig()
	token := &auth.StoredUAToken{
		UserOpenId:       cfg.UserOpenId,
		AppId:            cfg.AppID,
		AccessToken:      "test-user-access-token",
		RefreshToken:     "test-refresh-token",
		ExpiresAt:        time.Now().Add(1 * time.Hour).UnixMilli(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).UnixMilli(),
		Scope: strings.Join([]string{
			"mail:user_mailbox.messages:write",
			"mail:user_mailbox.messages:read",
			"mail:user_mailbox.message:modify",
			"mail:user_mailbox.message:readonly",
			"mail:user_mailbox.message.address:read",
			"mail:user_mailbox.message.subject:read",
			"mail:user_mailbox.message.body:read",
			"mail:user_mailbox.message:send",
			"mail:user_mailbox:readonly",
		}, " "),
		GrantedAt: time.Now().Add(-1 * time.Hour).UnixMilli(),
	}
	if err := auth.SetStoredToken(token); err != nil {
		t.Fatalf("SetStoredToken() error = %v", err)
	}
}

func TestBuildDraftSendOutputIncludesOptionalFields(t *testing.T) {
	got := buildDraftSendOutput(map[string]interface{}{
		"message_id": "msg_001",
		"thread_id":  "thread_001",
		"recall_status": map[string]interface{}{
			"status": "available",
		},
		"automation_send_disable": map[string]interface{}{
			"reason":    "Automation send is disabled by your mailbox setting",
			"reference": "https://open.larksuite.com/mail/settings/automation",
		},
	})

	if got["message_id"] != "msg_001" {
		t.Fatalf("message_id = %v", got["message_id"])
	}
	if got["thread_id"] != "thread_001" {
		t.Fatalf("thread_id = %v", got["thread_id"])
	}
	if _, ok := got["recall_status"].(map[string]interface{}); !ok {
		t.Fatalf("recall_status missing or wrong type: %#v", got["recall_status"])
	}
	if automation, ok := got["automation_send_disable"].(map[string]interface{}); !ok {
		t.Fatalf("automation_send_disable missing or wrong type: %#v", got["automation_send_disable"])
	} else if automation["reason"] != "Automation send is disabled by your mailbox setting" {
		t.Fatalf("automation_send_disable.reason = %v", automation["reason"])
	}
}

func TestMailSendConfirmSendOutputsAutomationDisable(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	grantMailSendScope(t)

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
	automation, ok := data["automation_send_disable"].(map[string]interface{})
	if !ok {
		t.Fatalf("automation_send_disable missing or wrong type: %#v", data["automation_send_disable"])
	}
	if automation["reason"] != "Automation send is disabled by your mailbox setting" {
		t.Fatalf("automation_send_disable.reason = %v", automation["reason"])
	}
}
