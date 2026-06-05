// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/mail/lint"
)

// jsonDecoderUnmarshal is a thin alias used by helpers in this file to keep
// the import set explicit even when the helper would otherwise be one-line.
func jsonDecoderUnmarshal(b []byte, v interface{}) error { return json.Unmarshal(b, v) }

// =====================================================================
// Writing-path lint integration tests — compose 5 + +draft-edit emit
// `lint_applied[]` and `original_blocked[]` arrays in the stdout envelope
// always.
// =====================================================================

// TestRunWritePathLint_PlainTextReturnsEmptyReport verifies the helper
// short-circuits on plain-text input.
func TestRunWritePathLint_PlainTextReturnsEmptyReport(t *testing.T) {
	cleaned, rep := runWritePathLint("")
	if cleaned != "" {
		t.Errorf("cleaned = %q, want empty", cleaned)
	}
	if rep.Applied == nil || rep.Blocked == nil {
		t.Error("Applied/Blocked must be non-nil")
	}
	if len(rep.Applied) != 0 || len(rep.Blocked) != 0 {
		t.Errorf("expected empty report, got applied=%d blocked=%d",
			len(rep.Applied), len(rep.Blocked))
	}
}

// TestRunWritePathLint_HTMLAlwaysAutofixedWarningNeverElevated verifies the
// writing path always autofixes warnings and never elevates them — the
// writing-path safety contract has no opt-out. The input
// triggers two warning autofixes (<p> paragraph-rewrite + <font> tag
// rewrite); both must surface in `Applied` and never appear in `Blocked`.
func TestRunWritePathLint_HTMLAlwaysAutofixedWarningNeverElevated(t *testing.T) {
	cleaned, rep := runWritePathLint(`<p><font color="red">x</font></p>`)
	if !strings.Contains(cleaned, "<span") {
		t.Errorf("expected autofix to rewrite <font>, cleaned=%q", cleaned)
	}
	if strings.Contains(cleaned, "<p>") || strings.Contains(cleaned, "<font") {
		t.Errorf("expected <p>/<font> rewritten, cleaned=%q", cleaned)
	}
	if len(rep.Applied) < 1 {
		t.Errorf("expected ≥1 warning surfaced (font + paragraph autofix), got %d", len(rep.Applied))
	}
	// Warnings never become errors on the writing-path; --strict no longer
	// exists at the surface either, so the contract is "Applied gathers
	// warnings, Blocked stays empty for warning-only inputs".
	if len(rep.Blocked) != 0 {
		t.Errorf("writing-path must NOT elevate warnings; expected 0 blocked, got %d", len(rep.Blocked))
	}
}

// TestApplyLintToEnvelope_DefaultEmitsNoLintFields verifies the helper writes
// zero keys in the default (non-detail) mode — neither count fields nor the
// full Finding arrays appear; the envelope stays small.
func TestApplyLintToEnvelope_DefaultEmitsNoLintFields(t *testing.T) {
	data := map[string]interface{}{"existing": "value"}
	rep := lint.EmptyReport(`<p>x</p>`)
	applyLintToEnvelope(data, rep.Applied, rep.Blocked, false)

	if data["existing"] != "value" {
		t.Error("existing key was clobbered")
	}
	if _, ok := data["lint_applied_count"]; ok {
		t.Error("lint_applied_count must NOT be present in default mode")
	}
	if _, ok := data["original_blocked_count"]; ok {
		t.Error("original_blocked_count must NOT be present in default mode")
	}
	if _, ok := data["lint_applied"]; ok {
		t.Error("lint_applied[] must NOT be present in default mode")
	}
	if _, ok := data["original_blocked"]; ok {
		t.Error("original_blocked[] must NOT be present in default mode")
	}
}

// TestApplyLintToEnvelope_DetailModeIncludesArrays verifies the detail mode
// (showDetails=true) attaches the two non-nil Finding arrays only. The
// `*_count` fields are no longer emitted (callers can compute counts via
// `len(arr)` themselves).
func TestApplyLintToEnvelope_DetailModeIncludesArrays(t *testing.T) {
	data := map[string]interface{}{}
	rep := lint.EmptyReport(`<p>x</p>`)
	applyLintToEnvelope(data, rep.Applied, rep.Blocked, true)

	if _, ok := data["lint_applied_count"]; ok {
		t.Error("lint_applied_count must NOT be present (count fields removed)")
	}
	if _, ok := data["original_blocked_count"]; ok {
		t.Error("original_blocked_count must NOT be present (count fields removed)")
	}
	la, ok := data["lint_applied"].([]lint.Finding)
	if !ok {
		t.Fatalf("lint_applied wrong type: %T", data["lint_applied"])
	}
	if la == nil {
		t.Error("lint_applied is nil — must be empty slice in detail mode")
	}
	ob, ok := data["original_blocked"].([]lint.Finding)
	if !ok {
		t.Fatalf("original_blocked wrong type: %T", data["original_blocked"])
	}
	if ob == nil {
		t.Error("original_blocked is nil — must be empty slice in detail mode")
	}
}

// =====================================================================
// End-to-end: +draft-create writing path emits envelope with lint fields.
// =====================================================================

// TestMailDraftCreate_WritePathLintEnvelopeDefault verifies +draft-create's
// default envelope contains the three always-present hint/id fields
// (compose_hint + draft_edit_hint + draft_id) and carries NO lint fields at
// all — neither `*_count` nor the full Finding arrays.
func TestMailDraftCreate_WritePathLintEnvelopeDefault(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	registerDraftCreateOK(reg)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Test",
		"--body", `<p>safe</p><script>alert(1)</script><font color="red">red</font>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)

	// The three always-present hint/id fields must appear.
	if hint, _ := data["compose_hint"].(string); hint == "" {
		t.Error("compose_hint must be present in default envelope")
	}
	if hint, _ := data["draft_edit_hint"].(string); hint == "" {
		t.Error("draft_edit_hint must be present in +draft-create default envelope")
	} else if hint != draftEditHintConst {
		t.Errorf("draft_edit_hint = %q, want exact const value", hint)
	}
	if id, _ := data["draft_id"].(string); id == "" {
		t.Error("draft_id must be present in default envelope")
	}

	// No lint fields (neither count nor arrays) in default mode.
	if _, present := data["lint_applied_count"]; present {
		t.Error("lint_applied_count must NOT appear (count fields removed)")
	}
	if _, present := data["original_blocked_count"]; present {
		t.Error("original_blocked_count must NOT appear (count fields removed)")
	}
	if _, present := data["lint_applied"]; present {
		t.Error("lint_applied[] must be hidden in default mode")
	}
	if _, present := data["original_blocked"]; present {
		t.Error("original_blocked[] must be hidden in default mode")
	}
}

// TestMailDraftCreate_WritePathLintEnvelopeWithDetails verifies that passing
// --show-lint-details attaches the two Finding arrays only — no `*_count`
// fields — while still keeping compose_hint + draft_edit_hint + draft_id.
func TestMailDraftCreate_WritePathLintEnvelopeWithDetails(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	registerDraftCreateOK(reg)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Test",
		"--body", `<p>safe</p><script>alert(1)</script><font color="red">red</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)

	// Always-present hint/id fields survive in detail mode.
	if hint, _ := data["compose_hint"].(string); hint == "" {
		t.Error("compose_hint must be present in detail envelope")
	}
	if hint, _ := data["draft_edit_hint"].(string); hint == "" {
		t.Error("draft_edit_hint must be present in +draft-create detail envelope")
	} else if hint != draftEditHintConst {
		t.Errorf("draft_edit_hint = %q, want exact const value", hint)
	}
	if id, _ := data["draft_id"].(string); id == "" {
		t.Error("draft_id must be present in detail envelope")
	}

	// `*_count` fields are gone — callers compute counts via len(arr).
	if _, present := data["lint_applied_count"]; present {
		t.Error("lint_applied_count must NOT appear (count fields removed)")
	}
	if _, present := data["original_blocked_count"]; present {
		t.Error("original_blocked_count must NOT appear (count fields removed)")
	}

	la, ok := data["lint_applied"].([]interface{})
	if !ok {
		t.Fatalf("lint_applied missing or wrong type: %T", data["lint_applied"])
	}
	ob, ok := data["original_blocked"].([]interface{})
	if !ok {
		t.Fatalf("original_blocked missing or wrong type: %T", data["original_blocked"])
	}
	if len(la) < 1 {
		t.Errorf("expected ≥1 lint_applied entry, got %d", len(la))
	}
	if len(ob) < 1 {
		t.Errorf("expected ≥1 original_blocked entry, got %d", len(ob))
	}
}

// TestMailDraftCreate_PlainTextWritePathOmitsLintFields verifies the
// plain-text path's default envelope contains the always-present
// compose_hint + draft_edit_hint + draft_id and emits no lint fields at all.
func TestMailDraftCreate_PlainTextWritePathOmitsLintFields(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	registerDraftCreateOK(reg)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Test",
		"--body", "plain text only",
		"--plain-text",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)

	// Always-present hint/id fields on the plain-text branch.
	if hint, _ := data["compose_hint"].(string); hint == "" {
		t.Error("compose_hint must be present on plain-text path")
	}
	if hint, _ := data["draft_edit_hint"].(string); hint == "" {
		t.Error("draft_edit_hint must be present on +draft-create plain-text path")
	} else if hint != draftEditHintConst {
		t.Errorf("draft_edit_hint = %q, want exact const value", hint)
	}
	if id, _ := data["draft_id"].(string); id == "" {
		t.Error("draft_id must be present on plain-text path")
	}

	// No lint fields at all on the default plain-text path.
	if _, present := data["lint_applied_count"]; present {
		t.Error("lint_applied_count must NOT appear on plain-text default path")
	}
	if _, present := data["original_blocked_count"]; present {
		t.Error("original_blocked_count must NOT appear on plain-text default path")
	}
	if _, present := data["lint_applied"]; present {
		t.Error("lint_applied[] must be hidden in default mode (plain-text)")
	}
	if _, present := data["original_blocked"]; present {
		t.Error("original_blocked[] must be hidden in default mode (plain-text)")
	}
}

// TestMailDraftCreate_AutofixApplied verifies that the writing path actually
// rewrites the body before sending it to drafts.create — the user's <font>
// tag must NOT reach the network as <font>.
func TestMailDraftCreate_AutofixApplied(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_test"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Test",
		"--body", `<font color="red">x</font>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Decode the raw EML and confirm <font> was rewritten before reaching
	// emlbuilder. The base64url payload contains the HTML body in raw form.
	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("write-path should have rewritten <font>, EML still contains it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> wrapper in EML, got %q", captured)
	}
}

// TestMailDraftCreate_ScriptStrippedBeforeSend verifies <script> is removed
// from the EML before drafts.create is invoked (writing-path safety floor).
func TestMailDraftCreate_ScriptStrippedBeforeSend(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_test"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Test",
		"--body", `<p>before</p><script>alert(1)</script><p>after</p>`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eml := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(eml, "<script") {
		t.Errorf("script should be stripped before EML send, got %q", eml)
	}
	if strings.Contains(eml, "alert(1)") {
		t.Errorf("script content should be removed, got %q", eml)
	}
	if !strings.Contains(eml, "before") || !strings.Contains(eml, "after") {
		t.Errorf("surrounding paragraphs should survive, got %q", eml)
	}
}

// =====================================================================
// Helpers — mail_shortcut_test.go ships the factory; these are local
// httpmock registrations specific to the lint integration tests.
// =====================================================================

// registerMailboxProfileMock registers a stock GET .../profile response so
// resolveComposeSenderEmail finds an address.
func registerMailboxProfileMock(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "sender@example.com",
				"send_as":               []interface{}{},
			},
		},
	})
}

// registerDraftCreateOK registers a successful drafts.create response.
func registerDraftCreateOK(reg *httpmock.Registry) {
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "d_test123",
			},
		},
	})
}

// mustDecodeRawEMLFromStub extracts the `raw` field from a captured body and
// base64url-decodes it. The stub.CapturedBody is populated by the httpmock
// after a match (registry.go:42 — the stub records every captured request).
func mustDecodeRawEMLFromStub(t *testing.T, stub *httpmock.Stub) string {
	t.Helper()
	if len(stub.CapturedBody) == 0 {
		t.Fatal("stub did not capture any request body")
	}
	var captured map[string]interface{}
	if err := jsonUnmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	raw, ok := captured["raw"].(string)
	if !ok {
		t.Fatalf("captured body has no `raw` string field: %#v", captured)
	}
	return decodeBase64URL(raw)
}

func jsonUnmarshal(b []byte, v interface{}) error {
	return jsonDecoderUnmarshal(b, v)
}

// =====================================================================
// End-to-end coverage for the 5 other compose shortcuts. Each test feeds
// HTML containing a <font> tag (warning-tier autofix target) through the
// shortcut and asserts (a) the EML sent on the wire has the <font>
// rewritten to <span>, and (b) the envelope honours `--show-lint-details`.
// =====================================================================

// stubSourceMessageHTML registers a minimal source-message GET stub that
// `+reply` / `+reply-all` / `+forward` use to derive the parent message
// headers + body. The original body is plain HTML so the reply lint path
// is exercised on the user-authored body only (the writing-path contract:
// quoted block is never re-linted).
func stubSourceMessageHTML(reg *httpmock.Registry, bodyHTML string) {
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/profile",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"primary_email_address": "me@example.com",
			},
		},
	})
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/messages/msg_w1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"message": map[string]interface{}{
					"message_id":      "msg_w1",
					"thread_id":       "thread_w1",
					"smtp_message_id": "<msg_w1@example.com>",
					"subject":         "Original",
					"head_from":       map[string]interface{}{"mail_address": "sender@example.com", "name": "Sender"},
					"to":              []map[string]interface{}{{"mail_address": "me@example.com", "name": "Me"}},
					"cc":              []interface{}{},
					"bcc":             []interface{}{},
					"body_html":       base64URLEncode(bodyHTML),
					"body_plain_text": base64URLEncode("plain"),
					"internal_date":   "1704067200000",
					"attachments":     []map[string]interface{}{},
				},
			},
		},
	})
}

// base64URLEncode wraps encoding/base64.URLEncoding.EncodeToString to keep
// the new tests readable inline.
func base64URLEncode(s string) string {
	return base64.URLEncoding.EncodeToString([]byte(s))
}

// TestMailSend_WritePathLintAutofixesFontInEML drives +send end-to-end with
// HTML containing a <font> tag and asserts the body in the captured EML has
// been rewritten to <span> before the drafts.create POST.
func TestMailSend_WritePathLintAutofixesFontInEML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_send"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSend, []string{
		"+send",
		"--to", "alice@example.com",
		"--subject", "Send",
		"--body", `<font color="red">payload</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("+send writing-path should rewrite <font>, EML still has it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> in EML, got %q", captured)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	la, ok := data["lint_applied"].([]interface{})
	if !ok {
		t.Fatalf("lint_applied missing or wrong type: %T", data["lint_applied"])
	}
	if len(la) < 1 {
		t.Errorf("expected ≥1 lint_applied entry, got %d", len(la))
	}
}

// TestMailReply_WritePathLintAutofixesFontInEML drives +reply end-to-end.
func TestMailReply_WritePathLintAutofixesFontInEML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	stubSourceMessageHTML(reg, `<p>Original</p>`)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_reply"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailReply, []string{
		"+reply",
		"--message-id", "msg_w1",
		"--body", `<font color="red">reply text</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply failed: %v", err)
	}

	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("+reply writing-path should rewrite <font>, EML still has it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> in EML, got %q", captured)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if _, present := data["lint_applied"]; !present {
		t.Error("lint_applied should appear under --show-lint-details")
	}
}

// TestMailReplyAll_WritePathLintAutofixesFontInEML drives +reply-all e2e.
func TestMailReplyAll_WritePathLintAutofixesFontInEML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	stubSourceMessageHTML(reg, `<p>Original</p>`)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_replyall"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailReplyAll, []string{
		"+reply-all",
		"--message-id", "msg_w1",
		"--body", `<font color="red">reply-all text</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("reply-all failed: %v", err)
	}

	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("+reply-all writing-path should rewrite <font>, EML still has it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> in EML, got %q", captured)
	}
}

// TestMailForward_WritePathLintAutofixesFontInEML drives +forward e2e.
func TestMailForward_WritePathLintAutofixesFontInEML(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	stubSourceMessageHTML(reg, `<p>Original</p>`)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/drafts",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_forward"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_w1",
		"--to", "bob@example.com",
		"--body", `<font color="red">forward note</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("+forward writing-path should rewrite <font>, EML still has it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> in EML, got %q", captured)
	}
}

// TestMailDraftEdit_WritePathLintAutofixesFontViaBodyFlag verifies the
// `--body` shortcut on +draft-edit (which lowers to a set_body patch op)
// runs the writing-path lint before PUT-ing the updated EML.
func TestMailDraftEdit_WritePathLintAutofixesFontViaBodyFlag(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)

	// drafts.get(format=raw) returns a minimal multipart EML so the parser
	// has a body to patch.
	originalEML := "MIME-Version: 1.0\r\n" +
		"From: me@example.com\r\n" +
		"To: alice@example.com\r\n" +
		"Subject: Edit\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>original body</p>\r\n"
	reg.Register(&httpmock.Stub{
		URL: "/user_mailboxes/me/drafts/d_edit",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"draft_id": "d_edit",
				"raw":      base64URLEncode(originalEML),
			},
		},
	})
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/drafts/d_edit",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"draft_id": "d_edit"},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailDraftEdit, []string{
		"+draft-edit",
		"--draft-id", "d_edit",
		"--body", `<font color="red">new body</font>`,
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("draft-edit failed: %v", err)
	}

	captured := mustDecodeRawEMLFromStub(t, stub)
	if strings.Contains(captured, "<font") {
		t.Errorf("+draft-edit writing-path should rewrite <font>, EML still has it: %q", captured)
	}
	if !strings.Contains(captured, "<span") {
		t.Errorf("expected <span> in EML, got %q", captured)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if _, present := data["lint_applied"]; !present {
		t.Error("lint_applied should appear under --show-lint-details on +draft-edit")
	}
}

// TestMailDraftCreate_PlainTextShowLintDetailsEmitsEmptyArrays locks the
// 2×2 corner: plain-text body + --show-lint-details. The envelope must
// surface the two contract arrays as empty (non-nil) slices because the
// detail flag toggles their presence; the plain-text branch produces zero
// findings but the keys must still appear so consumers can rely on them
// unconditionally.
func TestMailDraftCreate_PlainTextShowLintDetailsEmitsEmptyArrays(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactory(t)
	chdirTemp(t)
	registerMailboxProfileMock(reg)
	registerDraftCreateOK(reg)

	err := runMountedMailShortcut(t, MailDraftCreate, []string{
		"+draft-create",
		"--to", "alice@example.com",
		"--subject", "Plain",
		"--body", "plain text body, no html",
		"--plain-text",
		"--show-lint-details",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	la, ok := data["lint_applied"].([]interface{})
	if !ok {
		t.Fatalf("lint_applied missing or wrong type on plain-text + show-lint-details: %T", data["lint_applied"])
	}
	if len(la) != 0 {
		t.Errorf("plain-text body should produce 0 lint_applied entries, got %d", len(la))
	}
	ob, ok := data["original_blocked"].([]interface{})
	if !ok {
		t.Fatalf("original_blocked missing or wrong type: %T", data["original_blocked"])
	}
	if len(ob) != 0 {
		t.Errorf("plain-text body should produce 0 original_blocked entries, got %d", len(ob))
	}
}
