// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func registerSignatureListStub(reg *httpmock.Registry, mailboxID string, signatures []interface{}, usages []interface{}) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + url.PathEscape(mailboxID) + "/settings/signatures",
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

func registerSendAsStub(reg *httpmock.Registry, mailboxID, email string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + url.PathEscape(mailboxID) + "/settings/send_as",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"sendable_addresses": []interface{}{
					map[string]interface{}{"email_address": email, "name": "Sender Name"},
				},
			},
		},
	})
}

func signatureForTest(id, content string, images []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"id":             id,
		"name":           id,
		"signature_type": "USER",
		"content":        content,
		"images":         images,
	}
}

func sendDefaultUsageForTest(email, signatureID string) map[string]interface{} {
	return map[string]interface{}{
		"email_address":          email,
		"send_mail_signature_id": signatureID,
		"reply_signature_id":     "reply-unused",
	}
}

func registerDraftCreateStub(reg *httpmock.Registry, mailboxID string) *httpmock.Stub {
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/" + url.PathEscape(mailboxID) + "/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(stub)
	return stub
}

func TestMailSendDefaultSignatureHTML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-html@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{
			signatureForTest("sig_owner", `<p>Owner Sig<img src="cid:sigcid"></p>`, []interface{}{
				map[string]interface{}{
					"image_name":   "logo.png",
					"cid":          "sigcid",
					"download_url": "https://storage.example.com/sigcid",
				},
			}),
		},
		[]interface{}{sendDefaultUsageForTest("owner-html@example.com", "sig_owner")},
	)
	registerSendAsStub(reg, mailboxID, "owner-html@example.com")
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "https://storage.example.com/sigcid",
		RawBody: []byte{0x89, 'P', 'N', 'G'},
	})
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
	}, f, stdout); err != nil {
		t.Fatalf("+send default signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	for _, want := range []string{`class="lark-mail-signature"`, "Owner Sig", "Content-Id: <sigcid>"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("EML missing %q:\n%s", want, raw)
		}
	}
}

func TestMailSendNoSignatureSkipsSignatureLookup(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-skip@example.com"
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "owner-skip@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
		"--no-signature",
	}, f, stdout); err != nil {
		t.Fatalf("+send --no-signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	if strings.Contains(raw, "lark-mail-signature") {
		t.Fatalf("EML should not contain signature:\n%s", raw)
	}
	reg.Verify(t)
}

func TestMailSendExplicitSignatureIgnoresDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-explicit@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{
			signatureForTest("sig_default", "<p>Default Sig</p>", nil),
			signatureForTest("sig_explicit", "<p>Explicit Sig</p>", nil),
		},
		[]interface{}{sendDefaultUsageForTest("owner-explicit@example.com", "sig_default")},
	)
	registerSendAsStub(reg, mailboxID, "owner-explicit@example.com")
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
		"--signature-id", "sig_explicit",
	}, f, stdout); err != nil {
		t.Fatalf("+send explicit signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	if !strings.Contains(raw, "Explicit Sig") {
		t.Fatalf("EML missing explicit signature:\n%s", raw)
	}
	if strings.Contains(raw, "Default Sig") {
		t.Fatalf("EML should not contain default signature:\n%s", raw)
	}
}

func TestMailSendDefaultSignatureMatchesAliasSender(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-alias@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{
			signatureForTest("sig_owner", "<p>Owner Sig</p>", nil),
			signatureForTest("sig_alias", "<p>Alias Sig</p>", nil),
		},
		[]interface{}{
			sendDefaultUsageForTest("owner-alias@example.com", "sig_owner"),
			sendDefaultUsageForTest("alias@example.com", "sig_alias"),
		},
	)
	registerSendAsStub(reg, mailboxID, "alias@example.com")
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "alias@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
	}, f, stdout); err != nil {
		t.Fatalf("+send alias signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	if !strings.Contains(raw, "Alias Sig") || strings.Contains(raw, "Owner Sig") {
		t.Fatalf("EML should contain alias signature only:\n%s", raw)
	}
}

func TestMailSendNoDefaultSignatureCreatesDraft(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-nodefault@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{signatureForTest("sig_other", "<p>Other Sig</p>", nil)},
		[]interface{}{sendDefaultUsageForTest("owner-nodefault@example.com", "0")},
	)
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "owner-nodefault@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
	}, f, stdout); err != nil {
		t.Fatalf("+send no default signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	if strings.Contains(raw, "Other Sig") || strings.Contains(raw, "lark-mail-signature") {
		t.Fatalf("EML should not contain a signature:\n%s", raw)
	}
}

func TestMailSendPlainTextSignatureDoesNotDownloadImages(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-plain@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{
			signatureForTest("sig_plain", `<div>Plain Sig</div><img src="cid:sigcid"><script>bad()</script>`, []interface{}{
				map[string]interface{}{
					"image_name":   "logo.png",
					"cid":          "sigcid",
					"download_url": "https://storage.example.com/should-not-download",
				},
			}),
		},
		[]interface{}{sendDefaultUsageForTest("owner-plain@example.com", "sig_plain")},
	)
	registerSendAsStub(reg, mailboxID, "owner-plain@example.com")
	draftStub := registerDraftCreateStub(reg, mailboxID)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "Body",
		"--plain-text",
	}, f, stdout); err != nil {
		t.Fatalf("+send plain-text signature failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, draftStub.CapturedBody)
	for _, want := range []string{"Content-Type: text/plain", "Body", "Plain Sig"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("plain EML missing %q:\n%s", want, raw)
		}
	}
	for _, forbidden := range []string{"lark-mail-signature", "Content-ID:", "cid:sigcid", "bad()"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("plain EML leaked %q:\n%s", forbidden, raw)
		}
	}
}

func TestMailSendSignatureAPIFailureDoesNotCreateDraft(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-api-fail@example.com"
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/" + url.PathEscape(mailboxID) + "/settings/signatures",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "boom",
		},
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "owner-api-fail@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected signature API failure")
	}
	if strings.Contains(err.Error(), "draft") {
		t.Fatalf("error should come from signature lookup before draft creation, got: %v", err)
	}
}

func TestMailSendExplicitSignatureMissingDoesNotCreateDraft(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-missing@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{signatureForTest("sig_other", "<p>Other Sig</p>", nil)},
		nil,
	)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "owner-missing@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
		"--signature-id", "sig_missing",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "signature not found") {
		t.Fatalf("expected signature not found error, got: %v", err)
	}
}

func TestMailSendSignatureImageFailureDoesNotCreateDraft(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	mailboxID := "owner-image-fail@example.com"
	registerSignatureListStub(reg, mailboxID,
		[]interface{}{
			signatureForTest("sig_img", `<p>Sig <img src="cid:sigcid"></p>`, []interface{}{
				map[string]interface{}{
					"image_name":   "logo.png",
					"cid":          "sigcid",
					"download_url": "https://storage.example.com/missing-logo",
				},
			}),
		},
		[]interface{}{sendDefaultUsageForTest("owner-image-fail@example.com", "sig_img")},
	)
	registerSendAsStub(reg, mailboxID, "owner-image-fail@example.com")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "https://storage.example.com/missing-logo",
		Status: 404,
		Body:   "missing",
	})

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "failed to download signature image") {
		t.Fatalf("expected image download error, got: %v", err)
	}
}

func TestMailSendSignatureFlagConflictValidation(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "Body",
		"--signature-id", "sig_1",
		"--no-signature",
	}, f, stdout)
	assertValidationError(t, err, "--signature-id and --no-signature")
}

func TestMailSendEmptySignatureIDFailsBeforeAPI(t *testing.T) {
	f, stdout, _, _ := mailShortcutTestFactory(t)
	mailboxID := "owner-empty@example.com"

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--mailbox", mailboxID,
		"--from", "owner-empty@example.com",
		"--to", "alice@example.com",
		"--subject", "hello",
		"--body", "<p>Body</p>",
		"--signature-id", "",
	}, f, stdout)
	assertValidationError(t, err, "--signature-id must not be empty")
}

func TestMailSendDryRunSignaturePlan(t *testing.T) {
	runtimeFor := func(args []string) string {
		f, stdout, _, _ := mailShortcutTestFactory(t)
		err := runMountedMailShortcut(t, MailSend, append([]string{
			"+send", "--to", "alice@example.com", "--subject", "hello", "--body", "Body",
		}, args...), f, stdout)
		if err != nil {
			t.Fatalf("dry-run failed: %v", err)
		}
		var out map[string]interface{}
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal dry-run: %v; stdout=%s", err, stdout.String())
		}
		b, _ := json.Marshal(out)
		return string(b)
	}

	defaultPlan := runtimeFor([]string{"--dry-run"})
	if !strings.Contains(defaultPlan, "/settings/signatures") {
		t.Fatalf("default dry-run missing signatures call: %s", defaultPlan)
	}
	explicitPlan := runtimeFor([]string{"--signature-id", "sig_1", "--dry-run"})
	if !strings.Contains(explicitPlan, "/settings/signatures") {
		t.Fatalf("explicit dry-run missing signatures call: %s", explicitPlan)
	}
	skipPlan := runtimeFor([]string{"--no-signature", "--dry-run"})
	if strings.Contains(skipPlan, "/settings/signatures") {
		t.Fatalf("--no-signature dry-run should omit signatures call: %s", skipPlan)
	}
}
