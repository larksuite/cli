// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// calendarEventArgs are CLI flags that embed a calendar event in a compose command.
var calendarEventArgs = []string{
	"--event-summary", "Team Sync",
	"--event-start", "2026-05-10T10:00+08:00",
	"--event-end", "2026-05-10T11:00+08:00",
}

// extractEMLFromDraftsStub decodes the base64url EML from the captured request body.
func extractEMLFromDraftsStub(t *testing.T, stub *httpmock.Stub) string {
	t.Helper()
	var reqBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	raw, _ := reqBody["raw"].(string)
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("base64url decode raw: %v", err)
	}
	return string(decoded)
}

// assertCalendarInEML checks that the decoded EML contains a text/calendar part.
func assertCalendarInEML(t *testing.T, eml string) {
	t.Helper()
	if !strings.Contains(eml, "text/calendar") {
		t.Errorf("expected text/calendar part in EML:\n%s", eml)
	}
	if !strings.Contains(eml, "method=REQUEST") {
		t.Errorf("expected method=REQUEST in Content-Type:\n%s", eml)
	}
}

// stubSourceMessage registers the minimum stubs to fetch a simple source message
// (used by reply/forward/reply-all).
func stubSourceMessage(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/messages/msg_001",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_001",
					"thread_id":       "thread_001",
					"smtp_message_id": "<msg_001@example.com>",
					"subject":         "Re: Original",
					"head_from":       map[string]interface{}{"mail_address": "sender@example.com", "name": "Sender"},
					"to":              []map[string]interface{}{{"mail_address": "me@example.com", "name": "Me"}},
					"cc":              []interface{}{},
					"bcc":             []interface{}{},
					"body_html":       base64.URLEncoding.EncodeToString([]byte("<p>Original</p>")),
					"body_plain_text": base64.URLEncoding.EncodeToString([]byte("Original")),
					"internal_date":   "1704067200000",
					"attachments":     []interface{}{},
				},
			},
		},
	})
}

// ---------------------------------------------------------------------------
// +reply with calendar event
// ---------------------------------------------------------------------------

func TestReply_WithCalendarEvent(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubSourceMessage(reg)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})
	draftsStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(draftsStub)

	args := append([]string{
		"+reply",
		"--message-id", "msg_001",
		"--body", "<p>Let us meet</p>",
	}, calendarEventArgs...)
	if err := runMountedMailShortcut(t, MailReply, args, f, stdout); err != nil {
		t.Fatalf("+reply with calendar failed: %v", err)
	}
	assertCalendarInEML(t, extractEMLFromDraftsStub(t, draftsStub))
}

// ---------------------------------------------------------------------------
// +reply-all with calendar event
// ---------------------------------------------------------------------------

func TestReplyAll_WithCalendarEvent(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubSourceMessage(reg)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})
	draftsStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(draftsStub)

	args := append([]string{
		"+reply-all",
		"--message-id", "msg_001",
		"--body", "<p>Let us meet</p>",
	}, calendarEventArgs...)
	if err := runMountedMailShortcut(t, MailReplyAll, args, f, stdout); err != nil {
		t.Fatalf("+reply-all with calendar failed: %v", err)
	}
	assertCalendarInEML(t, extractEMLFromDraftsStub(t, draftsStub))
}

// ---------------------------------------------------------------------------
// +forward with calendar event
// ---------------------------------------------------------------------------

func TestForward_WithCalendarEvent(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubSourceMessage(reg)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"primary_email_address": "me@example.com"},
		},
	})
	draftsStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(draftsStub)

	args := append([]string{
		"+forward",
		"--message-id", "msg_001",
		"--to", "carol@example.com",
		"--body", "<p>FYI</p>",
	}, calendarEventArgs...)
	if err := runMountedMailShortcut(t, MailForward, args, f, stdout); err != nil {
		t.Fatalf("+forward with calendar failed: %v", err)
	}
	assertCalendarInEML(t, extractEMLFromDraftsStub(t, draftsStub))
}
