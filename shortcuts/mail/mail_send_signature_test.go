// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

func TestDefaultSendSignatureIDFromUsages(t *testing.T) {
	usages := []signature.SignatureUsage{
		{EmailAddress: "primary@example.com", SendMailSignatureID: "0"},
		{EmailAddress: "alias@example.com", SendMailSignatureID: "sig_alias"},
		{EmailAddress: "shared@example.com", SendMailSignatureID: "sig_shared"},
	}

	if got := defaultSendSignatureIDFromUsages(usages, "ALIAS@example.com", "primary@example.com"); got != "sig_alias" {
		t.Fatalf("default signature ID = %q, want sig_alias", got)
	}
	if got := defaultSendSignatureIDFromUsages(usages, "missing@example.com", "shared@example.com"); got != "sig_shared" {
		t.Fatalf("fallback default signature ID = %q, want sig_shared", got)
	}
	if got := defaultSendSignatureIDFromUsages(usages, "primary@example.com", "shared@example.com"); got != "" {
		t.Fatalf("zero default signature ID = %q, want empty", got)
	}
}

func TestAppendPlainTextSignature(t *testing.T) {
	sig := &signatureResult{
		ID:              "sig_text",
		RenderedContent: `<p>Regards,<br><a href="https://example.com">Alice</a><img src="cid:x"></p>`,
	}

	got := appendPlainTextSignature("hello\n", sig)
	want := "hello\n\nRegards,\nAlice (https://example.com)"
	if got != want {
		t.Fatalf("plain text signature:\n got %q\nwant %q", got, want)
	}
}

func TestMailSendDefaultSignatureHitAppendsHTML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "mb-default-hit"
	registerSignatureListMock(reg, mailboxID, signatureListBody("sig_default", "sender@example.com", "sig_default", `<p>Default Sig</p>`))
	draftStub := registerDraftCreateForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>body</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if !strings.Contains(eml, "Default Sig") {
		t.Fatalf("default signature not appended in EML:\n%s", eml)
	}
}

func TestMailSendDefaultSignatureZeroDoesNotAppend(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "mb-default-zero"
	registerSignatureListMock(reg, mailboxID, signatureListBody("sig_default", "sender@example.com", "0", `<p>Default Sig</p>`))
	draftStub := registerDraftCreateForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>body</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if strings.Contains(eml, "Default Sig") {
		t.Fatalf("zero default signature should not append signature:\n%s", eml)
	}
}

func TestMailSendDefaultSignatureLookupFailureDowngrades(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "mb-default-fail"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + mailboxID + "/settings/signatures",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "temporary signature failure",
		},
	})
	draftStub := registerDraftCreateForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>body</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send should downgrade default-signature failure: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: default signature lookup failed") {
		t.Fatalf("expected default signature warning, stderr=%q", stderr.String())
	}
	_ = mustDecodeRawEMLFromStub(t, draftStub)
}

func TestMailSendNoSignatureSkipsDefaultLookup(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "mb-no-signature"
	draftStub := registerDraftCreateForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>body</p>",
		"--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send with --no-signature failed: %v", err)
	}
	if strings.Contains(stderr.String(), "default signature lookup failed") {
		t.Fatalf("--no-signature should skip lookup warning, stderr=%q", stderr.String())
	}
	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if strings.Contains(eml, "Default Sig") {
		t.Fatalf("--no-signature should not append signature:\n%s", eml)
	}
}

func TestMailSendPlainTextSignatureStaysText(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "mb-plain-text-signature"
	registerSignatureListMock(reg, mailboxID, signatureListBody("sig_text", "sender@example.com", "sig_text", `<p>Regards,<br>Alice</p>`))
	draftStub := registerDraftCreateForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "plain body",
		"--plain-text",
		"--signature-id", "sig_text",
	}, f, stdout)
	if err != nil {
		t.Fatalf("plain-text send with signature failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftStub)
	if !strings.Contains(eml, "text/plain") {
		t.Fatalf("plain-text signature send should stay text/plain:\n%s", eml)
	}
	if strings.Contains(eml, "text/html") {
		t.Fatalf("plain-text signature send should not add html part:\n%s", eml)
	}
	if !strings.Contains(eml, "plain body") || !strings.Contains(eml, "Regards,") || !strings.Contains(eml, "Alice") {
		t.Fatalf("plain-text signature not present in EML:\n%s", eml)
	}
}

func TestMailSendNoSignatureConflictsWithSignatureID(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "body",
		"--signature-id", "sig_123",
		"--no-signature",
	}, f, stdout)
	assertValidationError(t, err, "--no-signature and --signature-id")
}

func TestNonSendComposeShortcutsStillRejectPlainTextSignature(t *testing.T) {
	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "draft-create",
			shortcut: MailDraftCreate,
			args: []string{
				"+draft-create", "--to", "alice@example.com", "--subject", "hello", "--body", "body",
				"--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "reply",
			shortcut: MailReply,
			args: []string{
				"+reply", "--message-id", "msg_123", "--body", "body",
				"--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "reply-all",
			shortcut: MailReplyAll,
			args: []string{
				"+reply-all", "--message-id", "msg_123", "--body", "body",
				"--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "forward",
			shortcut: MailForward,
			args: []string{
				"+forward", "--message-id", "msg_123", "--to", "alice@example.com", "--body", "body",
				"--plain-text", "--signature-id", "sig_123",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			assertValidationError(t, err, "--plain-text and --signature-id")
		})
	}
}

func registerSignatureListMock(reg *httpmock.Registry, mailboxID string, body map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + mailboxID + "/settings/signatures",
		Body:   body,
	})
}

func registerDraftCreateForMailbox(reg *httpmock.Registry, mailboxID string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/" + mailboxID + "/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_" + mailboxID},
		},
	}
	reg.Register(stub)
	return stub
}

func signatureListBody(signatureID, usageEmail, usageSignatureID, content string) map[string]interface{} {
	return map[string]interface{}{
		"code": 0,
		"data": map[string]interface{}{
			"signatures": []map[string]interface{}{
				{
					"id":             signatureID,
					"name":           "Default",
					"signature_type": "USER",
					"content":        content,
				},
			},
			"usages": []map[string]interface{}{
				{
					"email_address":          usageEmail,
					"send_mail_signature_id": usageSignatureID,
					"reply_signature_id":     "0",
				},
			},
		},
	}
}
