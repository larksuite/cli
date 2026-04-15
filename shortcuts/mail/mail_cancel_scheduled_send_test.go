// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailCancelScheduledSend_Success(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/msg_sched_123/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--message-id", "msg_sched_123",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["message_id"] != "msg_sched_123" {
		t.Fatalf("expected message_id=msg_sched_123, got %v", data["message_id"])
	}
	if data["status"] != "cancelled" {
		t.Fatalf("expected status=cancelled, got %v", data["status"])
	}
	if data["restored_as_draft"] != true {
		t.Fatalf("expected restored_as_draft=true, got %v", data["restored_as_draft"])
	}
}

func TestMailCancelScheduledSend_CustomMailbox(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/mailbox_abc/messages/msg_456/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--message-id", "msg_456",
		"--user-mailbox-id", "mailbox_abc",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["message_id"] != "msg_456" {
		t.Fatalf("expected message_id=msg_456, got %v", data["message_id"])
	}
}

func TestMailCancelScheduledSend_MissingMessageID(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
	}, f, stdout)

	if err == nil {
		t.Fatal("expected error for missing --message-id")
	}
}

func TestMailCancelScheduledSend_APIError(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/msg_err/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 99999,
			"msg":  "message already sent",
		},
	})

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--message-id", "msg_err",
	}, f, stdout)

	if err == nil {
		t.Fatal("expected error for API failure")
	}
}

func TestMailCancelScheduledSend_OutputFormat(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/msg_fmt/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--message-id", "msg_fmt",
	}, f, stdout)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}
	if !envelope.OK {
		t.Fatal("expected ok=true in output")
	}
	if envelope.Data["status"] != "cancelled" {
		t.Fatalf("expected status=cancelled in output, got %v", envelope.Data["status"])
	}
}
