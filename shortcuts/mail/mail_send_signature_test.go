// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestMailSendDefaultHTMLSignatureAppendsSignature(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailbox := "sig-html@example.com"
	registerSignatureListStub(reg, mailbox, []map[string]interface{}{
		{
			"id":             "sig_default",
			"signature_type": "USER",
			"content":        "<div>Best<br>Alice</div>",
		},
	}, []map[string]interface{}{
		{"email_address": mailbox, "send_mail_signature_id": "sig_default"},
	})
	registerSendAsStub(reg, mailbox, "Alice", mailbox)
	draftsStub := registerDraftCreateStub(reg, mailbox)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailbox,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftsStub)
	if !strings.Contains(eml, `class="lark-mail-signature"`) {
		t.Fatalf("expected signature wrapper in HTML EML:\n%s", eml)
	}
	if !strings.Contains(eml, "Best") || !strings.Contains(eml, "Alice") {
		t.Fatalf("expected default signature content in EML:\n%s", eml)
	}
}

func TestMailSendPlainTextDefaultSignatureKeepsTextPlain(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailbox := "sig-plain@example.com"
	registerSignatureListStub(reg, mailbox, []map[string]interface{}{
		{
			"id":             "sig_plain",
			"signature_type": "USER",
			"content":        "<div>Regards<br>Alice</div>",
			"images": []map[string]interface{}{
				{"cid": "logo", "image_name": "logo.png", "download_url": "https://example.com/logo.png"},
			},
		},
	}, []map[string]interface{}{
		{"email_address": mailbox, "send_mail_signature_id": "sig_plain"},
	})
	registerSendAsStub(reg, mailbox, "Alice", mailbox)
	draftsStub := registerDraftCreateStub(reg, mailbox)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailbox,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
		"--plain-text",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftsStub)
	if !strings.Contains(eml, "Content-Type: text/plain") {
		t.Fatalf("expected text/plain EML:\n%s", eml)
	}
	if strings.Contains(eml, "text/html") || strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("plain-text signature should not create HTML body:\n%s", eml)
	}
	if !strings.Contains(eml, "Hello") || !strings.Contains(eml, "Regards") || !strings.Contains(eml, "Alice") {
		t.Fatalf("expected body and text signature in EML:\n%s", eml)
	}
}

func TestMailSendNoSignatureSkipsSignatureAPI(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailbox := "sig-skip@example.com"
	draftsStub := registerDraftCreateStub(reg, mailbox)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailbox,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--no-signature",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftsStub)
	if strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("--no-signature should not append signature:\n%s", eml)
	}
}

func TestMailSendExplicitSignatureOverridesDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	mailbox := "sig-explicit@example.com"
	registerSignatureListStub(reg, mailbox, []map[string]interface{}{
		{
			"id":             "sig_default",
			"signature_type": "USER",
			"content":        "<div>Default Signature</div>",
		},
		{
			"id":             "sig_explicit",
			"signature_type": "TENANT",
			"content":        "<div>Explicit Signature</div>",
		},
	}, []map[string]interface{}{
		{"email_address": mailbox, "send_mail_signature_id": "sig_default"},
	})
	registerSendAsStub(reg, mailbox, "Alice", mailbox)
	draftsStub := registerDraftCreateStub(reg, mailbox)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailbox,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "<p>Hello</p>",
		"--signature-id", "sig_explicit",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	eml := mustDecodeRawEMLFromStub(t, draftsStub)
	if !strings.Contains(eml, "Explicit Signature") {
		t.Fatalf("expected explicit signature in EML:\n%s", eml)
	}
	if strings.Contains(eml, "Default Signature") {
		t.Fatalf("explicit signature should override default:\n%s", eml)
	}
}

func TestMailSendDefaultSignatureFailureWarnsAndContinues(t *testing.T) {
	f, stdout, stderr, reg := mailShortcutTestFactoryWithSendScope(t)
	mailbox := "sig-fail-open@example.com"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailbox, "settings", "signatures"),
		Status: 500,
		Body: map[string]interface{}{
			"code": 500,
			"msg":  "signature service unavailable",
		},
	})
	draftsStub := registerDraftCreateStub(reg, mailbox)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailbox,
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send should fail open when default signature API fails: %v", err)
	}
	if !strings.Contains(stderr.String(), "warning: failed to resolve default signature") {
		t.Fatalf("expected default signature warning, got stderr=%q", stderr.String())
	}
	eml := mustDecodeRawEMLFromStub(t, draftsStub)
	if strings.Contains(eml, "lark-mail-signature") {
		t.Fatalf("failed default signature lookup should not append signature:\n%s", eml)
	}
}

func TestMailSendDryRunSignatureAPIs(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactoryWithSendScope(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "dry-default@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	got := dryRunURLs(t, stdout.String())
	assertContainsURL(t, got, "/settings/signatures")
	assertContainsURL(t, got, "/settings/send_as")

	f, stdout, _, _ = mailShortcutTestFactoryWithSendScope(t)
	err = runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", "dry-skip@example.com",
		"--to", "bob@example.com",
		"--subject", "hello",
		"--body", "Hello",
		"--no-signature",
		"--dry-run",
	}, f, stdout)
	if err != nil {
		t.Fatalf("dry-run with --no-signature failed: %v", err)
	}
	got = dryRunURLs(t, stdout.String())
	assertNotContainsURL(t, got, "/settings/signatures")
	assertNotContainsURL(t, got, "/settings/send_as")
}

func registerSignatureListStub(reg *httpmock.Registry, mailbox string, signatures, usages []map[string]interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailbox, "settings", "signatures"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": signatures,
				"usages":     usages,
			},
		},
	})
}

func registerSendAsStub(reg *httpmock.Registry, mailbox, name, email string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    mailboxPath(mailbox, "settings", "send_as"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []map[string]interface{}{
					{"name": name, "email_address": email},
				},
			},
		},
	})
}

func registerDraftCreateStub(reg *httpmock.Registry, mailbox string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    mailboxPath(mailbox, "drafts"),
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "draft_sig_001",
			},
		},
	}
	reg.Register(stub)
	return stub
}

func dryRunURLs(t *testing.T, stdout string) []string {
	t.Helper()
	var payload struct {
		API []struct {
			URL string `json:"url"`
		} `json:"api"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal dry-run stdout: %v\nstdout=%s", err, stdout)
	}
	urls := make([]string, 0, len(payload.API))
	for _, call := range payload.API {
		urls = append(urls, call.URL)
	}
	return urls
}

func assertContainsURL(t *testing.T, urls []string, needle string) {
	t.Helper()
	for _, url := range urls {
		if strings.Contains(url, needle) {
			return
		}
	}
	t.Fatalf("expected dry-run URL containing %q, got %#v", needle, urls)
}

func assertNotContainsURL(t *testing.T, urls []string, needle string) {
	t.Helper()
	for _, url := range urls {
		if strings.Contains(url, needle) {
			t.Fatalf("did not expect dry-run URL containing %q, got %#v", needle, urls)
		}
	}
}
