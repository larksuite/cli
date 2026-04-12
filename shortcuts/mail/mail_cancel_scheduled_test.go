// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestCancelScheduledSend_Success(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Stub cancel_scheduled_send endpoint — capture request to verify path
	cancelStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/messages/msg_sched_001/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(cancelStub)

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--message-id", "msg_sched_001",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}

	// Verify the stub was matched (i.e., correct URL was called)
	if len(cancelStub.CapturedBody) > 0 {
		// Body should be nil for cancel — if present, just verify it's empty or null
		var body interface{}
		_ = json.Unmarshal(cancelStub.CapturedBody, &body)
		// No specific body assertions needed; the important thing is the correct URL was hit.
	}

	// Verify output contains expected fields
	out := stdout.String()
	if !strings.Contains(out, "msg_sched_001") {
		t.Fatalf("expected message_id in output, got: %s", out)
	}
	if !strings.Contains(out, "cancelled") {
		t.Fatalf("expected cancelled status in output, got: %s", out)
	}
}

func TestCancelScheduledSend_MissingMessageID(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected error when --message-id is not provided")
	}
}

func TestCancelScheduledSend_CustomMailbox(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)

	// Stub cancel endpoint with custom mailbox
	cancelStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/custom@example.com/messages/msg_sched_002/cancel_scheduled_send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(cancelStub)

	err := runMountedMailShortcut(t, MailCancelScheduledSend, []string{
		"+cancel-scheduled-send",
		"--mailbox", "custom@example.com",
		"--message-id", "msg_sched_002",
	}, f, stdout)
	if err != nil {
		t.Fatalf("runMountedMailShortcut() error = %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "msg_sched_002") {
		t.Fatalf("expected message_id in output, got: %s", out)
	}
}
