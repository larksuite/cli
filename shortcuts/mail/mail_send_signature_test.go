// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

func TestSelectMailSendDefaultSignatureID(t *testing.T) {
	usages := []signature.SignatureUsage{
		{EmailAddress: "owner@example.com", SendMailSignatureID: "sig_owner"},
		{EmailAddress: "alias@example.com", SendMailSignatureID: " sig_alias "},
	}

	if got := selectMailSendDefaultSignatureID(usages, "ALIAS@example.com"); got != "sig_alias" {
		t.Fatalf("alias match = %q, want sig_alias", got)
	}
	if got := selectMailSendDefaultSignatureID(usages, "nobody@example.com"); got != "" {
		t.Fatalf("no match = %q, want empty", got)
	}
	if got := selectMailSendDefaultSignatureID([]signature.SignatureUsage{{SendMailSignatureID: "sig_single"}}, ""); got != "sig_single" {
		t.Fatalf("single fallback = %q, want sig_single", got)
	}
	if got := selectMailSendDefaultSignatureID(usages, ""); got != "" {
		t.Fatalf("multiple fallback = %q, want empty", got)
	}
}

func TestAppendMailSendPlainTextSignatureRendersHTML(t *testing.T) {
	got := appendMailSendPlainTextSignature("Hello\n", &signatureResult{
		ID:              "sig_plain",
		RenderedContent: `<div>Kind<br><b>Alice</b><img src="cid:logo"></div>`,
	}, "en_us")

	want := "Hello\n\nKind\nAlice [image]"
	if got != want {
		t.Fatalf("plain-text signature body = %q, want %q", got, want)
	}
}

func TestAppendMailSendPlainTextSignatureRendersEscapedHTML(t *testing.T) {
	got := appendMailSendPlainTextSignature("Hello", &signatureResult{
		ID:              "sig_plain_escaped",
		RenderedContent: `<div>Owner&lt;div style=&#34;white-space: nowrap;&#34;&gt;Kind&lt;br&gt;&lt;img src=&#34;cid:logo&#34;&gt;&lt;/div&gt;</div>`,
	}, "zh_cn")

	want := "Hello\n\nOwner\nKind\n[图片]"
	if got != want {
		t.Fatalf("escaped plain-text signature body = %q, want %q", got, want)
	}
	if strings.Contains(got, "<div") || strings.Contains(got, "<img") {
		t.Fatalf("escaped HTML tags must not leak into plain-text body: %q", got)
	}
}

func TestValidateMailSendSignatureFlagsRejectsNoSignatureConflict(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "world",
		"--signature-id", "sig_123",
		"--no-signature",
	}, f, stdout)

	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error, got %T (%v)", err, err)
	}
	if len(validationErr.Params) != 2 {
		t.Fatalf("params = %#v, want signature/no-signature conflict", validationErr.Params)
	}
}

func TestMailSendDefaultSignatureUsesAliasSender(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	createStub := registerMailSendSignatureScenario(t, reg, mailSendSignatureScenario{
		MailboxID:      "sig_default_alias_box",
		DefaultEmail:   "owner@example.com",
		DefaultSigID:   "sig_owner",
		SignatureHTML:  `<p>Owner Signature</p>`,
		AliasEmail:     "alias@example.com",
		AliasSigID:     "sig_alias",
		AliasSignature: `<p>Alias Signature</p>`,
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_default_alias_box",
		"--from", "alias@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Alias Signature") {
		t.Fatalf("expected alias signature in EML:\n%s", raw)
	}
	if strings.Contains(raw, "Owner Signature") {
		t.Fatalf("default owner signature should not be used for alias sender:\n%s", raw)
	}
}

func TestMailSendExplicitSignatureOverridesDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	createStub := registerMailSendSignatureScenario(t, reg, mailSendSignatureScenario{
		MailboxID:     "sig_explicit_box",
		DefaultEmail:  "owner@example.com",
		DefaultSigID:  "sig_default",
		SignatureHTML: `<p>Default Signature</p>`,
		ExtraSignatures: []signature.Signature{
			{ID: "sig_explicit", Name: "Explicit", SignatureType: signature.SignatureTypeUser, Content: `<p>Explicit Signature</p>`},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_explicit_box",
		"--from", "owner@example.com",
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
		t.Fatalf("expected explicit signature in EML:\n%s", raw)
	}
	if strings.Contains(raw, "Default Signature") {
		t.Fatalf("explicit signature should override default signature:\n%s", raw)
	}
}

func TestMailSendNoSignatureSkipsSignatureAPIs(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	createStub := registerMailSendDraftCreate(reg, "sig_no_signature_box")

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_no_signature_box",
		"--from", "owner@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if strings.Contains(raw, "lark-mail-signature") {
		t.Fatalf("signature block should be absent when --no-signature is set:\n%s", raw)
	}
}

func TestMailSendPlainTextExplicitSignatureAppendsText(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	createStub := registerMailSendSignatureScenario(t, reg, mailSendSignatureScenario{
		MailboxID:     "sig_plain_text_box",
		DefaultEmail:  "owner@example.com",
		DefaultSigID:  "sig_default",
		SignatureHTML: `<div>Default</div>`,
		ExtraSignatures: []signature.Signature{
			{ID: "sig_text", Name: "Text", SignatureType: signature.SignatureTypeUser, Content: `<div>Kind<br><b>Alice</b></div>`},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_plain_text_box",
		"--from", "owner@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
		"--plain-text",
		"--signature-id", "sig_text",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Content-Type: text/plain") {
		t.Fatalf("expected text/plain EML:\n%s", raw)
	}
	if strings.Contains(raw, "Content-Type: text/html") {
		t.Fatalf("plain-text signature must not upgrade to HTML:\n%s", raw)
	}
	if !strings.Contains(raw, "Hello\n\nKind\nAlice") {
		t.Fatalf("expected plain-text signature appended after body:\n%s", raw)
	}
}

func TestMailSendDefaultSignatureLookupFailureHintsNoSignature(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/sig_lookup_failure_box/settings/signatures",
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "signature service unavailable",
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_lookup_failure_box",
		"--from", "owner@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected lookup failure")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if !strings.Contains(p.Message, "failed to look up default send signature") {
		t.Fatalf("message = %q, want default lookup prefix", p.Message)
	}
	if !strings.Contains(p.Hint, "--no-signature") {
		t.Fatalf("hint = %q, want --no-signature", p.Hint)
	}
}

func TestMailSendDefaultSignatureMissingTargetHintsUser(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/sig_missing_target_box/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []interface{}{},
				"usages": []interface{}{
					map[string]interface{}{
						"email_address":          "owner@example.com",
						"send_mail_signature_id": "sig_missing",
						"reply_signature_id":     "0",
					},
				},
			},
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_missing_target_box",
		"--from", "owner@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected missing signature validation error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T (%v)", err, err)
	}
	if !strings.Contains(p.Message, "default send signature") {
		t.Fatalf("message = %q, want default send signature", p.Message)
	}
	if !strings.Contains(p.Hint, "mail +signature") || !strings.Contains(p.Hint, "--no-signature") {
		t.Fatalf("hint = %q, want mail +signature and --no-signature", p.Hint)
	}
}

func TestMailSendSignatureSendAsFailureDegrades(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	createStub := registerMailSendSignatureScenario(t, reg, mailSendSignatureScenario{
		MailboxID:      "sig_send_as_degrade_box",
		DefaultEmail:   "owner@example.com",
		DefaultSigID:   "sig_default",
		SignatureHTML:  `<p>Default Signature</p>`,
		SkipSendAsStub: true,
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "sig_send_as_degrade_box",
		"--from", "owner@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send should degrade when send_as lookup fails: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Default Signature") {
		t.Fatalf("signature should still be appended when send_as fails:\n%s", raw)
	}
}

func TestNonSendComposeStillRejectsPlainTextSignatureID(t *testing.T) {
	cases := []struct {
		name     string
		shortcut common.Shortcut
		args     []string
	}{
		{
			name:     "draft-create",
			shortcut: MailDraftCreate,
			args: []string{
				"+draft-create", "--to", "alice@example.com", "--subject", "s", "--body", "body", "--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "reply",
			shortcut: MailReply,
			args: []string{
				"+reply", "--message-id", "msg_001", "--body", "body", "--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "reply-all",
			shortcut: MailReplyAll,
			args: []string{
				"+reply-all", "--message-id", "msg_001", "--body", "body", "--plain-text", "--signature-id", "sig_123",
			},
		},
		{
			name:     "forward",
			shortcut: MailForward,
			args: []string{
				"+forward", "--message-id", "msg_001", "--to", "alice@example.com", "--body", "body", "--plain-text", "--signature-id", "sig_123",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := mailShortcutTestFactory(t)
			err := runMountedMailShortcut(t, tc.shortcut, tc.args, f, stdout)
			assertValidationError(t, err, "--plain-text and --signature-id are mutually exclusive")
		})
	}
}

type mailSendSignatureScenario struct {
	MailboxID       string
	DefaultEmail    string
	DefaultSigID    string
	SignatureHTML   string
	AliasEmail      string
	AliasSigID      string
	AliasSignature  string
	ExtraSignatures []signature.Signature
	SkipSendAsStub  bool
}

func registerMailSendSignatureScenario(t *testing.T, reg *httpmock.Registry, scenario mailSendSignatureScenario) *httpmock.Stub {
	t.Helper()
	signatures := []interface{}{
		map[string]interface{}{
			"id":               scenario.DefaultSigID,
			"name":             "Default",
			"signature_type":   string(signature.SignatureTypeUser),
			"signature_device": string(signature.DevicePC),
			"content":          scenario.SignatureHTML,
		},
	}
	usages := []interface{}{
		map[string]interface{}{
			"email_address":          scenario.DefaultEmail,
			"send_mail_signature_id": scenario.DefaultSigID,
			"reply_signature_id":     "0",
		},
	}
	if scenario.AliasEmail != "" {
		signatures = append(signatures, map[string]interface{}{
			"id":               scenario.AliasSigID,
			"name":             "Alias",
			"signature_type":   string(signature.SignatureTypeUser),
			"signature_device": string(signature.DevicePC),
			"content":          scenario.AliasSignature,
		})
		usages = append(usages, map[string]interface{}{
			"email_address":          scenario.AliasEmail,
			"send_mail_signature_id": scenario.AliasSigID,
			"reply_signature_id":     "0",
		})
	}
	for _, sig := range scenario.ExtraSignatures {
		signatures = append(signatures, map[string]interface{}{
			"id":               sig.ID,
			"name":             sig.Name,
			"signature_type":   string(sig.SignatureType),
			"signature_device": string(signature.DevicePC),
			"content":          sig.Content,
		})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + scenario.MailboxID + "/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": signatures,
				"usages":     usages,
			},
		},
	})
	if !scenario.SkipSendAsStub {
		registerMailSendSignatureSendAs(reg, scenario.MailboxID, scenario.DefaultEmail, scenario.AliasEmail)
	}
	return registerMailSendDraftCreate(reg, scenario.MailboxID)
}

func registerMailSendSignatureSendAs(reg *httpmock.Registry, mailboxID, defaultEmail, aliasEmail string) {
	addresses := []interface{}{
		map[string]interface{}{"name": "Owner", "email_address": defaultEmail},
	}
	if aliasEmail != "" {
		addresses = append(addresses, map[string]interface{}{"name": "Alias", "email_address": aliasEmail})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + mailboxID + "/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": addresses,
			},
		},
	})
}

func registerMailSendDraftCreate(reg *httpmock.Registry, mailboxID string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/" + mailboxID + "/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_" + mailboxID,
			},
		},
	}
	reg.Register(stub)
	return stub
}
