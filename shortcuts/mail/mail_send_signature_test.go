// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailSendDefaultSignatureHTML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "sig-html@example.com"
	draftStub := registerMailSendDraftStub(reg, mailboxID)
	registerSignaturesStub(reg, mailboxID, []map[string]interface{}{
		{"id": "sig_default", "name": "Default", "content": "<p>Default Signature</p>"},
	}, []map[string]interface{}{
		{"email_address": mailboxID, "send_mail_signature_id": "sig_default"},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeDraftRawEML(t, draftStub)
	if !strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("expected signature wrapper in EML:\n%s", eml)
	}
	if !strings.Contains(eml, "Default Signature") {
		t.Fatalf("expected default signature content in EML:\n%s", eml)
	}
}

func TestMailSendDefaultSignaturePlainText(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "sig-plain@example.com"
	draftStub := registerMailSendDraftStub(reg, mailboxID)
	registerSignaturesStub(reg, mailboxID, []map[string]interface{}{
		{"id": "sig_plain", "name": "Plain", "content": "<div>Best regards<br><strong>Alice</strong><img src=\"cid:logo\"></div>"},
	}, []map[string]interface{}{
		{"email_address": mailboxID, "send_mail_signature_id": "sig_plain"},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "Hi",
		"--plain-text",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeDraftRawEML(t, draftStub)
	textBody := decodeSinglePartBase64Body(t, eml)
	if !strings.Contains(textBody, "Hi\n\nBest regards") {
		t.Fatalf("expected plain-text signature after blank line, body=%q EML:\n%s", textBody, eml)
	}
	if strings.Contains(textBody, "<div>") || strings.Contains(textBody, "<strong>") || strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("plain-text signature should not include HTML tags or wrapper, body=%q EML:\n%s", textBody, eml)
	}
}

func TestMailSendNoSignatureSkipsSignatureQuery(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "no-sig@example.com"
	draftStub := registerMailSendDraftStub(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeDraftRawEML(t, draftStub)
	if strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("did not expect signature wrapper in EML:\n%s", eml)
	}
}

func TestMailSendExplicitSignatureOverridesDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "sig-explicit@example.com"
	draftStub := registerMailSendDraftStub(reg, mailboxID)
	registerSignaturesStub(reg, mailboxID, []map[string]interface{}{
		{"id": "sig_default", "name": "Default", "content": "<p>Default Signature</p>"},
		{"id": "sig_explicit", "name": "Explicit", "content": "<p>Explicit Signature</p>"},
	}, []map[string]interface{}{
		{"email_address": mailboxID, "send_mail_signature_id": "sig_default"},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--signature-id", "sig_explicit",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeDraftRawEML(t, draftStub)
	if !strings.Contains(eml, "Explicit Signature") {
		t.Fatalf("expected explicit signature in EML:\n%s", eml)
	}
	if strings.Contains(eml, "Default Signature") {
		t.Fatalf("explicit signature should override default signature:\n%s", eml)
	}
}

func TestMailSendNoSignatureAndSignatureIDMutuallyExclusive(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "Hello",
		"--no-signature",
		"--signature-id", "sig_123",
	}, f, stdout)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	if !strings.Contains(err.Error(), "--no-signature and --signature-id are mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMailSendDefaultSignatureMatchesFromUsage(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner@example.com"
	draftStub := registerMailSendDraftStub(reg, mailboxID)
	registerSignaturesStub(reg, mailboxID, []map[string]interface{}{
		{"id": "sig_owner", "name": "Owner", "content": "<p>Owner Signature</p>"},
		{"id": "sig_alias", "name": "Alias", "content": "<p>Alias Signature</p>"},
	}, []map[string]interface{}{
		{"email_address": mailboxID, "send_mail_signature_id": "sig_owner"},
		{"email_address": "alias@example.com", "send_mail_signature_id": "sig_alias"},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "alias@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := decodeDraftRawEML(t, draftStub)
	if !strings.Contains(eml, "Alias Signature") {
		t.Fatalf("expected alias signature in EML:\n%s", eml)
	}
	if strings.Contains(eml, "Owner Signature") {
		t.Fatalf("expected --from usage to win over fallback default:\n%s", eml)
	}
}

func TestAppendPlainTextSignatureDoesNotTruncate(t *testing.T) {
	longText := strings.Repeat("x", 240)
	got := appendPlainTextSignature("Hi", &signatureResult{RenderedContent: "<p>" + longText + "</p>"}, "en_us")
	if !strings.Contains(got, longText) {
		t.Fatalf("plain-text signature was truncated: %q", got)
	}
}

func registerSignaturesStub(reg *httpmock.Registry, mailboxID string, signatures []map[string]interface{}, usages []map[string]interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + mailboxID + "/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": signatures,
				"usages":     usages,
			},
		},
	}
	reg.Register(stub)
	return stub
}

func registerMailSendDraftStub(reg *httpmock.Registry, mailboxID string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/" + mailboxID + "/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_001",
			},
		},
	}
	reg.Register(stub)
	return stub
}

func decodeDraftRawEML(t *testing.T, stub *httpmock.Stub) string {
	t.Helper()
	var reqBody map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &reqBody); err != nil {
		t.Fatalf("unmarshal captured draft body: %v", err)
	}
	raw, _ := reqBody["raw"].(string)
	decoded, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("base64url decode raw EML: %v", err)
	}
	return string(decoded)
}

func decodeSinglePartBase64Body(t *testing.T, eml string) string {
	t.Helper()
	parts := strings.SplitN(eml, "\r\n\r\n", 2)
	if len(parts) != 2 {
		parts = strings.SplitN(eml, "\n\n", 2)
	}
	if len(parts) != 2 {
		t.Fatalf("EML missing body separator:\n%s", eml)
	}
	encoded := strings.TrimSpace(parts[1])
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode base64 body: %v; body=%q", err, encoded)
	}
	return strings.ReplaceAll(string(decoded), "\r\n", "\n")
}
