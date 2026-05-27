// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
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
// always (spec §4.3 contract).
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

// TestRunWritePathLint_HTMLAlwaysAutofixesNeverStrict verifies the writing
// path uses {AutoFix: true, Strict: false} — strict warnings would block
// users on legitimate <font> tags, which spec §4.3 forbids.
func TestRunWritePathLint_HTMLAlwaysAutofixesNeverStrict(t *testing.T) {
	cleaned, rep := runWritePathLint(`<p><font color="red">x</font></p>`)
	if !strings.Contains(cleaned, "<span") {
		t.Errorf("expected autofix to rewrite <font>, cleaned=%q", cleaned)
	}
	if len(rep.Applied) != 1 {
		t.Errorf("expected 1 warning surfaced, got %d", len(rep.Applied))
	}
	// In strict mode the warning would be in Blocked instead. Confirm the
	// writing-path path does NOT promote.
	if len(rep.Blocked) != 0 {
		t.Errorf("writing-path must NOT use strict; expected 0 blocked, got %d", len(rep.Blocked))
	}
}

// TestApplyLintToEnvelope_DefaultOmitsAllLintFields verifies the helper
// writes NONE of the 4 lint fields in the default (non-detail) mode so the
// envelope stays token-frugal (only the 3 core keys: compose_hint /
// draft_id|message_id / reference). Honors tech-design §4.1.5 «field
// same-in-same-out» rule.
func TestApplyLintToEnvelope_DefaultOmitsAllLintFields(t *testing.T) {
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

// TestApplyLintToEnvelope_DetailModeIncludesAllFour verifies the detail mode
// (showDetails=true) attaches ALL 4 lint fields together: the 2 count fields
// alongside the 2 non-nil Finding arrays. The 4 fields appear and disappear
// together (tech-design §4.1.5 same-in-same-out rule).
func TestApplyLintToEnvelope_DetailModeIncludesAllFour(t *testing.T) {
	data := map[string]interface{}{}
	rep := lint.EmptyReport(`<p>x</p>`)
	applyLintToEnvelope(data, rep.Applied, rep.Blocked, true)

	if data["lint_applied_count"] != 0 {
		t.Errorf("lint_applied_count = %v, want 0", data["lint_applied_count"])
	}
	if data["original_blocked_count"] != 0 {
		t.Errorf("original_blocked_count = %v, want 0", data["original_blocked_count"])
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
// default envelope hides ALL 4 lint fields (counts + arrays) so the response
// stays token-frugal — even when the input body has warnings (<font>) and
// errors (<script>) that the writing path autofixes. Honors tech-design
// §4.1.5 «field same-in-same-out» rule.
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

	// All 4 lint fields must be absent in default mode (even when the writing
	// path actually applied autofixes / blocked tags under the hood).
	if _, present := data["lint_applied_count"]; present {
		t.Error("lint_applied_count must be hidden in default mode")
	}
	if _, present := data["original_blocked_count"]; present {
		t.Error("original_blocked_count must be hidden in default mode")
	}
	if _, present := data["lint_applied"]; present {
		t.Error("lint_applied[] must be hidden in default mode")
	}
	if _, present := data["original_blocked"]; present {
		t.Error("original_blocked[] must be hidden in default mode")
	}
}

// TestMailDraftCreate_WritePathLintEnvelopeWithDetails verifies that passing
// --show-lint-details attaches the full Finding arrays alongside counts.
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

// TestMailDraftCreate_PlainTextWritePathLintFieldsHidden verifies the
// envelope omits ALL 4 lint fields on the plain-text path (in default mode,
// no --show-lint-details). Same-in-same-out rule applies regardless of body
// kind.
func TestMailDraftCreate_PlainTextWritePathLintFieldsHidden(t *testing.T) {
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
	if _, present := data["lint_applied_count"]; present {
		t.Error("lint_applied_count must be hidden in default mode (plain-text)")
	}
	if _, present := data["original_blocked_count"]; present {
		t.Error("original_blocked_count must be hidden in default mode (plain-text)")
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
