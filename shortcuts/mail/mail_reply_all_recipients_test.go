// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBuildReplyAllRecipientsNormalExcludesSelf(t *testing.T) {
	to, cc := buildReplyAllRecipients(
		"Reply <reply@example.com>",
		[]string{"me@example.com", "first@example.com"},
		[]string{"ME@example.com", "second@example.com"},
		"me@example.com",
		map[string]bool{"me@example.com": true},
		false,
	)
	if to != "Reply <reply@example.com>" {
		t.Fatalf("to = %q", to)
	}
	if cc != "first@example.com, second@example.com" {
		t.Fatalf("cc = %q", cc)
	}
}

func TestBuildReplyAllRecipientsSelfSentKeepsOriginalLists(t *testing.T) {
	to, cc := buildReplyAllRecipients(
		"",
		[]string{"me@example.com", "First@example.com"},
		[]string{"ME@example.com", "second@example.com"},
		"me@example.com",
		map[string]bool{"me@example.com": true},
		true,
	)
	if to != "me@example.com, First@example.com" {
		t.Fatalf("to = %q", to)
	}
	if cc != "second@example.com" {
		t.Fatalf("cc = %q", cc)
	}
}

func TestBuildReplyAllRecipientsSelfSentAddsOnlyRealReplyTo(t *testing.T) {
	to, cc := buildReplyAllRecipients(
		"Replies <reply@example.com>",
		[]string{"me@example.com"},
		nil,
		"ME@example.com",
		map[string]bool{"me@example.com": true},
		true,
	)
	if to != "me@example.com, Replies <reply@example.com>" || cc != "" {
		t.Fatalf("got to=%q cc=%q", to, cc)
	}
}

func TestFilterAndDeduplicateReplyAllRecipients(t *testing.T) {
	remove, err := buildRemoveSet([]string{"remove@example.com", "Me <ME@example.com>"})
	if err != nil {
		t.Fatal(err)
	}
	to, cc, bcc := filterAndDeduplicateReplyAllRecipients(
		`First Person <First@example.com>, remove@example.com`,
		`duplicate <first@EXAMPLE.com>, Second <second@example.com>, me@example.com`,
		`SECOND@example.com, bcc@example.com`,
		remove,
	)
	if to != "First Person <First@example.com>" {
		t.Fatalf("to = %q", to)
	}
	if cc != "Second <second@example.com>" {
		t.Fatalf("cc = %q", cc)
	}
	if bcc != "bcc@example.com" {
		t.Fatalf("bcc = %q", bcc)
	}
}

func TestBuildRemoveSetRejectsInvalidAddress(t *testing.T) {
	_, err := buildRemoveSet([]string{"not-an-email"})
	if err == nil || !strings.Contains(err.Error(), "invalid email address") {
		t.Fatalf("err = %v", err)
	}
}

func TestFilterAndDeduplicateReplyAllRecipientsAllowsBccOnly(t *testing.T) {
	to, cc, bcc := filterAndDeduplicateReplyAllRecipients(
		"remove@example.com", "", "kept@example.com",
		map[string]bool{"remove@example.com": true},
	)
	if to != "" || cc != "" || bcc != "kept@example.com" {
		t.Fatalf("got to=%q cc=%q bcc=%q", to, cc, bcc)
	}
	if err := validateReplyAllRecipients(to, cc, bcc); err != nil {
		t.Fatalf("Bcc-only recipients must be accepted: %v", err)
	}
}

func TestValidateReplyAllRecipientsRejectsEmptyResult(t *testing.T) {
	err := validateReplyAllRecipients("", "", "")
	if err == nil || !strings.Contains(err.Error(), "no valid recipients") {
		t.Fatalf("err = %v", err)
	}
}

func TestMailReplyAllSelfSentCreatesThreadDraftByDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailboxProfile(reg, "me@example.com")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_self",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_self",
					"thread_id":       "thread_self",
					"smtp_message_id": "<self@smtp.example.com>",
					"subject":         "self sent",
					"head_from": map[string]interface{}{
						"mail_address": "ME@example.com",
						"name":         "Me",
					},
					"to": []interface{}{
						map[string]interface{}{"mail_address": "me@example.com", "name": "Myself"},
					},
					"body_plain_text": base64.RawURLEncoding.EncodeToString([]byte("original body")),
				},
			},
		},
	})
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_self"},
		},
	}
	reg.Register(createStub)

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all", "--message-id", "msg_self", "--body", "reply body",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all self-sent draft failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["draft_id"] != "draft_self" {
		t.Fatalf("draft_id = %v", data["draft_id"])
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "To: <me@example.com>") {
		t.Fatalf("self recipient missing from draft EML:\n%s", raw)
	}
	if !strings.Contains(raw, "X-LMS-Reply-To-Message-Id: msg_self") {
		t.Fatalf("original thread linkage header missing from draft EML:\n%s", raw)
	}
}
