// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func registerSendSignatureList(reg *httpmock.Registry, mailboxID string, signatures []map[string]interface{}, usages []map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailboxID, "settings", "signatures"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": signatures,
				"usages":     usages,
			},
		},
	})
}

func registerSendAsForSignature(reg *httpmock.Registry, mailboxID, email string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailboxID, "settings", "send_as"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []map[string]interface{}{
					{"email_address": email, "name": "Sender"},
				},
			},
		},
	})
}

func registerDraftCaptureStubsForMailbox(reg *httpmock.Registry, mailboxID string) *httpmock.Stub {
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    mailboxPath(mailboxID, "drafts"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(createStub)
	return createStub
}

func TestMailSendAppendsDefaultSignatureToHTMLBody(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "sig-html-box"
	sender := "sender@example.com"
	registerSendSignatureList(reg, mailboxID,
		[]map[string]interface{}{
			{
				"id":               "sig_default",
				"name":             "Default",
				"signature_type":   "USER",
				"signature_device": "PC",
				"content":          `<div>Best,<br>Alice</div>`,
			},
		},
		[]map[string]interface{}{
			{"email_address": sender, "send_mail_signature_id": "sig_default"},
		},
	)
	registerSendAsForSignature(reg, mailboxID, sender)
	createStub := registerDraftCaptureStubsForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", sender,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "lark-mail-signature") {
		t.Fatalf("HTML EML missing signature wrapper:\n%s", raw)
	}
	if !strings.Contains(raw, "Best") || !strings.Contains(raw, "Alice") {
		t.Fatalf("HTML EML missing signature content:\n%s", raw)
	}
}

func TestMailSendAppendsDefaultSignatureToPlainTextBody(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "sig-text-box"
	sender := "sender@example.com"
	registerSendSignatureList(reg, mailboxID,
		[]map[string]interface{}{
			{
				"id":               "sig_text",
				"name":             "Text",
				"signature_type":   "USER",
				"signature_device": "PC",
				"content":          `<div>Best,<br>Alice</div>`,
			},
		},
		[]map[string]interface{}{
			{"email_address": sender, "send_mail_signature_id": "sig_text"},
		},
	)
	registerSendAsForSignature(reg, mailboxID, sender)
	createStub := registerDraftCaptureStubsForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", sender,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Content-Type: text/plain") {
		t.Fatalf("plain body should remain text/plain:\n%s", raw)
	}
	if strings.Contains(raw, "lark-mail-signature") || strings.Contains(raw, "Content-Type: text/html") {
		t.Fatalf("plain body should not contain HTML signature wrapper:\n%s", raw)
	}
	if !strings.Contains(raw, "Hello") || !strings.Contains(raw, "Best") || !strings.Contains(raw, "Alice") {
		t.Fatalf("plain EML missing body or signature text:\n%s", raw)
	}
}

func TestMailSendNoSignatureSkipsSignatureLookup(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "no-sig-box"
	createStub := registerDraftCaptureStubsForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "sender@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if strings.Contains(raw, "lark-mail-signature") || strings.Contains(raw, "Best") {
		t.Fatalf("--no-signature should keep EML signature-free:\n%s", raw)
	}
}

func TestMailSendExplicitSignatureOverridesDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailboxID := "sig-explicit-box"
	sender := "sender@example.com"
	registerSendSignatureList(reg, mailboxID,
		[]map[string]interface{}{
			{
				"id":               "sig_default",
				"name":             "Default",
				"signature_type":   "USER",
				"signature_device": "PC",
				"content":          `<div>Default Signature</div>`,
			},
			{
				"id":               "sig_explicit",
				"name":             "Explicit",
				"signature_type":   "USER",
				"signature_device": "PC",
				"content":          `<div>Explicit Signature</div>`,
			},
		},
		[]map[string]interface{}{
			{"email_address": sender, "send_mail_signature_id": "sig_default"},
		},
	)
	registerSendAsForSignature(reg, mailboxID, sender)
	createStub := registerDraftCaptureStubsForMailbox(reg, mailboxID)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", sender,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--signature-id", "sig_explicit",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Explicit Signature") {
		t.Fatalf("explicit signature missing from EML:\n%s", raw)
	}
	if strings.Contains(raw, "Default Signature") {
		t.Fatalf("default signature should not be used when --signature-id is set:\n%s", raw)
	}
}

func TestMailSendDryRunShowsSignatureLookupUnlessDisabled(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--dry-run",
	}, f, stdout); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "/settings/signatures") {
		t.Fatalf("default dry-run should include signatures lookup:\n%s", stdout.String())
	}

	stdout.Reset()
	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--no-signature",
		"--dry-run",
	}, f, stdout); err != nil {
		t.Fatalf("dry-run with --no-signature failed: %v", err)
	}
	if strings.Contains(stdout.String(), "/settings/signatures") {
		t.Fatalf("--no-signature dry-run should omit signatures lookup:\n%s", stdout.String())
	}
}
