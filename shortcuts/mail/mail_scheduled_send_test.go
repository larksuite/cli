// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailSendConfirmSendWiresScheduledSendTime(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	scheduled := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_123",
			},
		},
	})
	sendStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts/draft_123/send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "msg_123",
				"thread_id":  "thread_123",
			},
		},
	}
	reg.Register(sendStub)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "me",
		"--from", "alias@example.com",
		"--to", "alice@example.com",
		"--subject", "Scheduled",
		"--body", "hello",
		"--confirm-send",
		"--send-time", scheduled,
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailSend execution failed: %v", err)
	}

	var sendBody map[string]interface{}
	if err := json.Unmarshal(sendStub.CapturedBody, &sendBody); err != nil {
		t.Fatalf("unmarshal send body: %v", err)
	}
	if got, ok := sendBody["send_time"].(string); !ok || got != scheduled {
		t.Fatalf("send_time = %v, want %q", sendBody["send_time"], scheduled)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if got := data["message_id"]; got != "msg_123" {
		t.Fatalf("message_id = %v, want msg_123", got)
	}
	if got := data["scheduled_send_time"]; got != scheduled {
		t.Fatalf("scheduled_send_time = %v, want %q", got, scheduled)
	}
	if got := data["scheduled_send_time_human"]; got == "" {
		t.Fatal("expected scheduled_send_time_human in output")
	}
}

func TestMailCancelScheduledSendExecute(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/msg_123/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "msg_123",
				"draft_id":   "draft_123",
			},
		},
	})

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--mailbox", "me",
		"--message-id", "msg_123",
	}, f, stdout)
	if err != nil {
		t.Fatalf("MailCancelScheduledSend execution failed: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if got := data["message_id"]; got != "msg_123" {
		t.Fatalf("message_id = %v, want msg_123", got)
	}
	if got := data["draft_id"]; got != "draft_123" {
		t.Fatalf("draft_id = %v, want draft_123", got)
	}
	if got := data["mailbox_id"]; got != "me" {
		t.Fatalf("mailbox_id = %v, want me", got)
	}
	if got := data["status"]; got != "canceled" {
		t.Fatalf("status = %v, want canceled", got)
	}
}

func TestMailShortcutsIncludesCancelScheduledSend(t *testing.T) {
	for _, s := range Shortcuts() {
		if s.Command == "+cancel-scheduled-send" {
			return
		}
	}
	t.Fatal("Shortcuts() missing +cancel-scheduled-send")
}
