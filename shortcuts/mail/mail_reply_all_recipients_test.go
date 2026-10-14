// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
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

func TestRecipientSetContainsDifferentOwnedSenderAddress(t *testing.T) {
	selfEmails := map[string]bool{
		"primary@example.com": true,
		"alias@example.com":   true,
	}
	if !recipientSetContains(selfEmails, "Primary <PRIMARY@example.com>") {
		t.Fatal("primary address should be recognized as self when sending from an alias")
	}

	isSelfSent := recipientSetContains(selfEmails, "primary@example.com") ||
		sameRecipientAddress("primary@example.com", "alias@example.com")
	to, cc := buildReplyAllRecipients(
		"",
		[]string{"primary@example.com"},
		nil,
		"alias@example.com",
		selfEmails,
		isSelfSent,
	)
	if to != "primary@example.com" || cc != "" {
		t.Fatalf("got to=%q cc=%q", to, cc)
	}
}

func TestFilterAndDeduplicateReplyAllRecipients(t *testing.T) {
	remove, err := buildRemoveSet([]string{"remove@example.com", "Me <ME@example.com>"})
	if err != nil {
		t.Fatal(err)
	}
	to, cc, bcc := filterAndDeduplicateReplyAllRecipients(
		`First Person <First@example.com>, FIRST@example.com, remove@example.com`,
		`duplicate <first@EXAMPLE.com>, Second <second@example.com>, SECOND@example.com, me@example.com`,
		`SECOND@example.com, bcc@example.com, BCC@example.com`,
		remove,
	)
	if to != "First Person <First@example.com>" {
		t.Fatalf("to = %q", to)
	}
	if cc != "duplicate <first@EXAMPLE.com>, Second <second@example.com>" {
		t.Fatalf("cc = %q", cc)
	}
	if bcc != "SECOND@example.com, bcc@example.com" {
		t.Fatalf("bcc = %q", bcc)
	}
}

func TestBuildRemoveSetAcceptsQuotedDisplayNameAndCommaList(t *testing.T) {
	remove, err := buildRemoveSet([]string{
		`"Doe, Jane" <Jane@example.com>, second@example.com`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !remove["jane@example.com"] || !remove["second@example.com"] {
		t.Fatalf("remove = %v", remove)
	}
}

func TestBuildRemoveSetRejectsInvalidAddress(t *testing.T) {
	_, err := buildRemoveSet([]string{"not-an-email"})
	if err == nil || !strings.Contains(err.Error(), "invalid email address") {
		t.Fatalf("err = %v", err)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if validationErr.Param != "--remove" || errors.Unwrap(err) == nil {
		t.Fatalf("validation error = %#v, cause = %v", validationErr, errors.Unwrap(err))
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
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T, want *errs.ValidationError", err)
	}
	if len(validationErr.Params) != 1 || validationErr.Params[0].Name != "--remove" ||
		validationErr.Params[0].Reason != "all recipients were removed" {
		t.Fatalf("params = %#v", validationErr.Params)
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

func TestMailReplyAllSelfSentFromOwnedAddressWhileSendingAsAlias(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailboxProfile(reg, "primary@example.com")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_self_alias",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_self_alias",
					"thread_id":       "thread_self_alias",
					"smtp_message_id": "<self-alias@smtp.example.com>",
					"subject":         "self sent with alternate sender",
					"head_from": map[string]interface{}{
						"mail_address": "primary@example.com",
						"name":         "Primary",
					},
					"to": []interface{}{
						map[string]interface{}{"mail_address": "primary@example.com", "name": "Primary"},
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
			"data": map[string]interface{}{"draft_id": "draft_self_alias"},
		},
	}
	reg.Register(createStub)

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all",
		"--mailbox", "me",
		"--from", "alias@example.com",
		"--message-id", "msg_self_alias",
		"--body", "reply body",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all self-sent draft with alias failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "From: <alias@example.com>") {
		t.Fatalf("alias sender missing from draft EML:\n%s", raw)
	}
	if !strings.Contains(raw, "To: <primary@example.com>") {
		t.Fatalf("original self recipient missing from draft EML:\n%s", raw)
	}
}

func TestMailReplyAllSelfSentAliasWithoutExplicitFromPreservesOriginalRecipients(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailboxProfile(reg, "primary@example.com")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []interface{}{
					map[string]interface{}{"email_address": "primary@example.com", "email_type": "USER_PRIMARY"},
					map[string]interface{}{"email_address": "alias@example.com", "email_type": "USER_ALIAS"},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_self_alias_default",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_self_alias_default",
					"thread_id":       "thread_self_alias_default",
					"smtp_message_id": "<self-alias-default@smtp.example.com>",
					"subject":         "self sent from alias to primary",
					"head_from": map[string]interface{}{
						"mail_address": "alias@example.com",
						"name":         "Alias",
					},
					"to": []interface{}{
						map[string]interface{}{"mail_address": "primary@example.com", "name": "Primary"},
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
			"data": map[string]interface{}{"draft_id": "draft_self_alias_default"},
		},
	}
	reg.Register(createStub)

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all",
		"--message-id", "msg_self_alias_default",
		"--body", "reply body",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all self-sent alias draft failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "From: <primary@example.com>") {
		t.Fatalf("default primary sender missing from draft EML:\n%s", raw)
	}
	if !strings.Contains(raw, "To: <primary@example.com>") {
		t.Fatalf("original primary recipient was not preserved:\n%s", raw)
	}
	if strings.Contains(raw, "To: <alias@example.com>") {
		t.Fatalf("alias sender incorrectly replaced original To recipient:\n%s", raw)
	}
	if !strings.Contains(raw, "X-LMS-Reply-To-Message-Id: msg_self_alias_default") {
		t.Fatalf("original thread linkage header missing from draft EML:\n%s", raw)
	}
}

func TestMailReplyAllRemoveAppliesToTemplateRecipients(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetTemplate(
		reg,
		"123",
		[]interface{}{map[string]interface{}{"mail_address": "remove@example.com", "name": "Remove"}},
		nil,
		nil,
		nil,
	)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/msg_template_remove",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_template_remove",
					"thread_id":       "thread_template_remove",
					"smtp_message_id": "<template-remove@smtp.example.com>",
					"subject":         "template recipient removal",
					"head_from": map[string]interface{}{
						"mail_address": "sender@example.com",
						"name":         "Sender",
					},
					"to": []interface{}{
						map[string]interface{}{"mail_address": "me@example.com", "name": "Me"},
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
			"data": map[string]interface{}{"draft_id": "draft_template_remove"},
		},
	}
	reg.Register(createStub)

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all",
		"--message-id", "msg_template_remove",
		"--body", "reply body",
		"--template-id", "123",
		"--remove", "Remove <remove@example.com>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all with template removal failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if strings.Contains(strings.ToLower(raw), "remove@example.com") {
		t.Fatalf("explicitly removed template recipient remains in draft EML:\n%s", raw)
	}
	if !strings.Contains(raw, "To: <sender@example.com>") {
		t.Fatalf("reply target missing from draft EML:\n%s", raw)
	}
}

func TestFilterReplyAllRecipientsRejectsInvalidMailboxes(t *testing.T) {
	to, cc, bcc := filterAndDeduplicateReplyAllRecipients(
		"undisclosed-recipients:;, removed@example.com",
		"not-an-email",
		"invalid@",
		map[string]bool{"removed@example.com": true},
	)
	if to != "" || cc != "" || bcc != "" {
		t.Fatalf("got to=%q cc=%q bcc=%q", to, cc, bcc)
	}
	err := validateReplyAllRecipients(to, cc, bcc)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error = %v, want *errs.ValidationError", err)
	}
}

func TestFilterReplyAllRecipientsKeepsValidMailboxesInOrder(t *testing.T) {
	to, cc, bcc := filterAndDeduplicateReplyAllRecipients(
		"undisclosed-recipients:;, First <First@example.com>, not-an-email, second@example.com, FIRST@example.com",
		"invalid@, Copy <copy@example.com>",
		"undisclosed-recipients:;, hidden@example.com",
		nil,
	)
	if to != "First <First@example.com>, second@example.com" ||
		cc != "Copy <copy@example.com>" || bcc != "hidden@example.com" {
		t.Fatalf("got to=%q cc=%q bcc=%q", to, cc, bcc)
	}
}
