// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// stubMailboxProfile registers a profile API stub that returns the given
// primary email address (or an empty response when primary is empty).
func stubMailboxProfile(reg *httpmock.Registry, primary string) {
	data := map[string]interface{}{}
	if primary != "" {
		data["primary_email_address"] = primary
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": data,
		},
	})
}

// stubGetMessageWithFormat registers a messages.get stub returning a minimal
// message suitable for reply / reply-all / forward. Subject / body / headers
// are fixed to deterministic values.
func stubGetMessageWithFormat(reg *httpmock.Registry, messageID string) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/" + messageID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      messageID,
					"thread_id":       "thread_abc",
					"smtp_message_id": "<orig@smtp.example.com>",
					"subject":         "original subject",
					"head_from": map[string]interface{}{
						"mail_address": "bob@example.com",
						"name":         "Bob",
					},
					"to": []interface{}{
						map[string]interface{}{"mail_address": "alice@example.com", "name": "Alice"},
					},
					"internal_date":   "1700000000000",
					"body_plain_text": base64.RawURLEncoding.EncodeToString([]byte("original body")),
				},
			},
		},
	})
}

// registerDraftCaptureStubs wires the registry so drafts.create captures the
// posted raw EML (via Stub.CapturedBody) and drafts.send returns a
// successful send response. The returned Stub's CapturedBody contains the
// JSON body of the drafts.create request; decodeCapturedRawEML extracts the
// base64url-decoded EML from it.
func registerDraftCaptureStubs(reg *httpmock.Registry) *httpmock.Stub {
	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "draft_001"},
		},
	}
	reg.Register(createStub)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts/draft_001/send",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message_id": "msg_001",
				"thread_id":  "thread_abc",
			},
		},
	})
	return createStub
}

// decodeCapturedRawEML extracts and base64url-decodes the "raw" field from
// the captured drafts.create request body. Returns "" when the body is
// unavailable or malformed.
func decodeCapturedRawEML(t *testing.T, capturedBody []byte) string {
	t.Helper()
	s := string(capturedBody)
	const key = `"raw":"`
	idx := strings.Index(s, key)
	if idx < 0 {
		t.Fatalf(`missing "raw" field in captured body: %s`, s)
	}
	rest := s[idx+len(key):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		t.Fatalf(`malformed "raw" field in captured body: %s`, s)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rest[:end])
	if err != nil {
		// Try standard URL encoding as fallback.
		decoded, err = base64.URLEncoding.DecodeString(rest[:end])
		if err != nil {
			t.Fatalf("failed to decode captured raw EML: %v", err)
		}
	}
	return string(decoded)
}

// TestMailSend_RequestReceiptAddsHeader_Integration verifies that running
// `+send --request-receipt` end-to-end writes a Disposition-Notification-To
// header addressed to the sender into the outgoing draft's EML.
func TestMailSend_RequestReceiptAddsHeader_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "bob@example.com",
		"--subject", "hi",
		"--body", "please confirm",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	// Pin the full header value so the From: header's me@example.com doesn't
	// satisfy a substring check even when DNT is broken.
	if !strings.Contains(raw, "Disposition-Notification-To: <me@example.com>") {
		t.Errorf("expected DNT header addressed to sender; got EML:\n%s", raw)
	}
}

// TestMailSend_RequestReceiptNoSender_FailsValidation covers the
// requireSenderForRequestReceipt error path on +send: --request-receipt set,
// no --from, profile returns no primary email → should fail fast with a
// clear error, no HTTP call to drafts.create.
func TestMailSend_RequestReceiptNoSender_FailsValidation(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "") // profile returns no primary address

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "bob@example.com",
		"--subject", "hi",
		"--body", "body",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error for --request-receipt without resolvable sender")
	}
	if !strings.Contains(err.Error(), "--request-receipt") {
		t.Errorf("error should mention --request-receipt, got: %v", err)
	}
}

// TestMailReply_RequestReceiptAddsHeader_Integration mirrors the +send test
// for +reply: verifies DNT ends up in the reply draft's EML.
func TestMailReply_RequestReceiptAddsHeader_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetMessageWithFormat(reg, "msg_orig")
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailReply, []string{
		"+reply",
		"--message-id", "msg_orig",
		"--body", "got it",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("reply failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Disposition-Notification-To: <me@example.com>") {
		t.Errorf("expected DNT header addressed to sender in reply EML; got:\n%s", raw)
	}
}

// TestMailReplyAll_RequestReceiptAddsHeader_Integration covers the +reply-all
// branch — reply-all had an extra concern because senderEmail falls back to
// orig.headTo when resolveComposeSenderEmail returns "". The gating added in
// this PR moves requireSenderForRequestReceipt before that fallback, so the
// receipt only resolves against an explicitly configured sender.
func TestMailReplyAll_RequestReceiptAddsHeader_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetMessageWithFormat(reg, "msg_orig")
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all",
		"--message-id", "msg_orig",
		"--body", "ack",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("reply-all failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Disposition-Notification-To: <me@example.com>") {
		t.Errorf("expected DNT header addressed to sender in reply-all EML; got:\n%s", raw)
	}
}

// stubGetMessageWithLabels registers a messages.get stub for the decline-
// receipt flow: the minimum fields the Execute path reads are message_id and
// label_ids. Callers supply the label list so tests can exercise both the
// "label present" and "already cleared" branches.
func stubGetMessageWithLabels(reg *httpmock.Registry, messageID string, labels []interface{}) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/" + messageID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id": messageID,
					"label_ids":  labels,
				},
			},
		},
	})
}

// TestMailDeclineReceipt_RemovesLabel_Integration exercises the happy path:
// a message carries the READ_RECEIPT_REQUEST label → +decline-receipt issues
// a PUT user_mailbox.message.modify (the public OpenAPI endpoint for the
// MessageModify RPC that the Lark client's "不发送" button also triggers
// internally) whose body removes exactly "READ_RECEIPT_REQUEST". Endpoint
// (single-message modify, not batch) and label-id form (symbolic name —
// the public OpenAPI accepts the symbolic form and translates to the
// internal numeric id server-side; the internal RPC uses -607 directly)
// are both pinned so regressions get caught here.
func TestMailDeclineReceipt_RemovesLabel_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubGetMessageWithLabels(reg, "msg_orig", []interface{}{"UNREAD", "READ_RECEIPT_REQUEST"})

	modifyStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/messages/msg_orig/modify",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	}
	reg.Register(modifyStub)

	if err := runMountedMailShortcut(t, MailDeclineReceipt, []string{
		"+decline-receipt",
		"--message-id", "msg_orig",
	}, f, stdout); err != nil {
		t.Fatalf("decline-receipt failed: %v", err)
	}

	body := string(modifyStub.CapturedBody)
	for _, want := range []string{
		`"remove_label_ids"`,
		`"READ_RECEIPT_REQUEST"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected modify body to contain %q; got:\n%s", want, body)
		}
	}
	// Guard: the public OpenAPI expects the symbolic label name; "-607"
	// (the internal numeric id used by the MessageModify RPC directly) is
	// not in the OpenAPI contract and must not leak into the request.
	if strings.Contains(body, `"-607"`) {
		t.Errorf("modify body should send symbolic \"READ_RECEIPT_REQUEST\", not internal numeric id; got:\n%s", body)
	}
	// Single-message modify has no message_ids array (that's the batch
	// endpoint's shape); assert we didn't accidentally keep the old payload.
	if strings.Contains(body, `"message_ids"`) {
		t.Errorf("single-message modify body should not contain message_ids (that's batch endpoint shape); got:\n%s", body)
	}

	out := stdout.String()
	if !strings.Contains(out, `"declined":true`) && !strings.Contains(out, `"declined": true`) {
		t.Errorf("expected declined=true in output; got:\n%s", out)
	}
}

// TestMailDeclineReceipt_AlreadyCleared_Integration verifies idempotence:
// when the READ_RECEIPT_REQUEST label is already absent the shortcut
// returns success without issuing a modify call.
func TestMailDeclineReceipt_AlreadyCleared_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubGetMessageWithLabels(reg, "msg_orig", []interface{}{"UNREAD"})

	// Intentionally not registering the modify stub: if Execute issues the
	// POST anyway, httpmock will fail the test loudly instead of silently
	// sending an unmocked request to the network.

	if err := runMountedMailShortcut(t, MailDeclineReceipt, []string{
		"+decline-receipt",
		"--message-id", "msg_orig",
	}, f, stdout); err != nil {
		t.Fatalf("decline-receipt failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "already_cleared") {
		t.Errorf("expected already_cleared=true in output; got:\n%s", out)
	}
	if !strings.Contains(out, `"declined":false`) && !strings.Contains(out, `"declined": false`) {
		t.Errorf("expected declined=false in output; got:\n%s", out)
	}
}

// TestMailForward_RequestReceiptAddsHeader_Integration covers the same path
// on +forward.
func TestMailForward_RequestReceiptAddsHeader_Integration(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetMessageWithFormat(reg, "msg_orig")
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_orig",
		"--to", "eve@example.com",
		"--body", "fyi",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "Disposition-Notification-To: <me@example.com>") {
		t.Errorf("expected DNT header addressed to sender in forward EML; got:\n%s", raw)
	}
}

// TestMailReply_RequestReceiptNoSender_DoesNotFallBackToOrigHeadTo guards
// the CC-only / shared-mailbox regression: when --request-receipt is set
// and no sender can be explicitly resolved (empty profile + no --from),
// +reply MUST fail validation instead of silently falling back to
// orig.headTo (which is some other recipient from the original message
// — in this stub, alice@example.com, the original "To"). Pre-fix, the
// fallback address satisfied the non-empty check and the DNT header was
// silently addressed to the wrong person.
func TestMailReply_RequestReceiptNoSender_DoesNotFallBackToOrigHeadTo(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "") // profile has no primary → resolvedSender == ""
	stubGetMessageWithFormat(reg, "msg_orig")
	// Intentionally not registering drafts.create: if Execute proceeds past
	// validation, httpmock fails the test loudly instead of a silent pass.

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply",
		"--message-id", "msg_orig",
		"--body", "ack",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error for --request-receipt with no resolvable sender; got nil")
	}
	if !strings.Contains(err.Error(), "--request-receipt") {
		t.Errorf("error should mention --request-receipt, got: %v", err)
	}
}

// TestMailForward_RequestReceiptNoSender_DoesNotFallBackToOrigHeadTo is
// the +forward counterpart to the +reply test above — same regression,
// same fix, same assertion.
func TestMailForward_RequestReceiptNoSender_DoesNotFallBackToOrigHeadTo(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "")
	stubGetMessageWithFormat(reg, "msg_orig")

	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_orig",
		"--to", "eve@example.com",
		"--body", "fyi",
		"--request-receipt",
		"--confirm-send",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected validation error for --request-receipt with no resolvable sender; got nil")
	}
	if !strings.Contains(err.Error(), "--request-receipt") {
		t.Errorf("error should mention --request-receipt, got: %v", err)
	}
}
