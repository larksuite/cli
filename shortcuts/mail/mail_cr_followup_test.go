// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// stubGetMessageWithAttachments registers a messages.get stub returning a
// message that carries the supplied raw attachments[] entries. Use it when a
// forward test needs to exercise the source-message attachment classification
// branch (e.g. a LARGE attachment that should land in the
// X-Lms-Large-Attachment-Ids header).
func stubGetMessageWithAttachments(reg *httpmock.Registry, messageID string, attachments []interface{}) {
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
					"attachments":     attachments,
				},
			},
		},
	})
}

// stubMessageAttachmentDownloadURLs registers the
// /messages/{id}/attachments/download_url stub used by fetchAttachmentURLs.
// The shape mirrors the real API: a download_urls array plus failed_ids. The
// caller's own logic decides whether to actually download — for LARGE
// attachments forward.go skips the download, so an empty/dummy URL works.
func stubMessageAttachmentDownloadURLs(reg *httpmock.Registry, messageID string, idToURL map[string]string) {
	urls := make([]map[string]interface{}, 0, len(idToURL))
	for id, u := range idToURL {
		urls = append(urls, map[string]interface{}{
			"attachment_id": id,
			"download_url":  u,
		})
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/messages/" + messageID + "/attachments/download_url",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"download_urls": urls,
				"failed_ids":    []interface{}{},
			},
		},
	})
}

// stubGetTemplate registers a templates.get stub returning the supplied
// fields. recipients is a triple of optional plural-form lists for tos / ccs
// / bccs (each entry: {mail_address, name}); attachments mirrors the API's
// snake_case shape (id, filename, is_inline, cid, attachment_type).
func stubGetTemplate(
	reg *httpmock.Registry,
	templateID string,
	tos, ccs, bccs []interface{},
	attachments []interface{},
) {
	tpl := map[string]interface{}{
		"template_id":        templateID,
		"name":               "Followup tpl " + templateID,
		"subject":            "tpl subj",
		"template_content":   "<p>tpl body</p>",
		"is_plain_text_mode": false,
	}
	if tos != nil {
		tpl["tos"] = tos
	}
	if ccs != nil {
		tpl["ccs"] = ccs
	}
	if bccs != nil {
		tpl["bccs"] = bccs
	}
	if attachments != nil {
		tpl["attachments"] = attachments
	}
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/" + templateID,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": tpl},
		},
	})
}

// extractLargeAttachmentHeaders pulls every X-Lms-Large-Attachment-Ids header
// line out of a raw EML and returns the base64-decoded JSON payload of each.
// Used to assert the forward post-fix invariant: exactly one header line is
// emitted, with the merged ID set.
func extractLargeAttachmentHeaders(t *testing.T, raw string) [][]map[string]interface{} {
	t.Helper()
	var out [][]map[string]interface{}
	// EML lines are CRLF-separated per RFC 5322; split on "\n" and trim a
	// trailing "\r" so the matcher works regardless of how the EML was
	// normalized between emit and capture.
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		const prefix = "X-Lms-Large-Attachment-Ids:"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		decoded, err := base64.StdEncoding.DecodeString(val)
		if err != nil {
			t.Fatalf("decode header value %q: %v", val, err)
		}
		var entries []map[string]interface{}
		if err := json.Unmarshal(decoded, &entries); err != nil {
			t.Fatalf("unmarshal header value %q: %v (raw=%s)", val, err, decoded)
		}
		out = append(out, entries)
	}
	return out
}

// ---------------------------------------------------------------------------
// CR follow-up #1 — +forward --confirm-send + --template-id: recipient check
// must be deferred until after applyTemplate has merged template addresses,
// so a template-only recipient set is not pre-rejected.
// ---------------------------------------------------------------------------

// TestMailForward_ConfirmSendTemplateOnlyRecipients_Allowed asserts that
// running +forward --confirm-send --template-id <id> with no --to/--cc/--bcc
// succeeds when the template carries the only recipient list. Pre-fix the
// recipient check fired in Validate (before fetchTemplate) and aborted with
// "at least one recipient is required" — this test pins the deferred-check
// behavior so future Validate-stage refactors can't silently re-introduce
// the pre-merge rejection.
func TestMailForward_ConfirmSendTemplateOnlyRecipients_Allowed(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetMessageWithAttachments(reg, "msg_orig", nil)
	// Template carries the only recipient.
	stubGetTemplate(reg, "7",
		[]interface{}{map[string]interface{}{"mail_address": "tpl-to@example.com", "name": "TplTo"}},
		nil, nil, nil,
	)
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_orig",
		"--template-id", "7",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("forward should succeed when template provides recipients; got: %v", err)
	}
	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	if !strings.Contains(raw, "tpl-to@example.com") {
		t.Errorf("EML missing template-derived recipient; got:\n%s", raw)
	}
}

// TestMailForward_ConfirmSendTemplateNoRecipients_Rejected asserts the dual
// invariant: when --template-id is set but neither the user flags nor the
// template carry any recipient, Execute must reject the call with the
// validation error after the merge. Otherwise the deferred check would let
// an empty-recipient draft slip through to drafts.create.
func TestMailForward_ConfirmSendTemplateNoRecipients_Rejected(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	stubGetMessageWithAttachments(reg, "msg_orig", nil)
	// Template has no recipients at all.
	stubGetTemplate(reg, "8", nil, nil, nil, nil)
	// Intentionally omit drafts.create stub: if Execute proceeds past the
	// post-merge recipient check, httpmock fails with "no stub" instead of
	// silently passing.

	err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_orig",
		"--template-id", "8",
		"--confirm-send",
	}, f, stdout)
	if err == nil {
		t.Fatal("expected post-merge recipient validation error; got nil")
	}
	if !strings.Contains(err.Error(), "at least one recipient") {
		t.Errorf("error should mention recipient requirement, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CR follow-up #2 — +forward must emit a single X-Lms-Large-Attachment-Ids
// header carrying both forward-derived and template-derived LARGE file_keys.
// Pre-fix the code called bld.Header() twice (append semantics in
// emlbuilder), producing two header lines — most RFC 5322 parsers (and the
// Lark gateway) only read the first, silently dropping one set.
// ---------------------------------------------------------------------------

// TestMailForward_LargeAttachmentHeader_MergedSingleHeader pins exactly one
// header line in the outgoing EML and verifies its decoded JSON body lists
// every expected ID — both the source-message LARGE attachment and the two
// template LARGE entries.
func TestMailForward_LargeAttachmentHeader_MergedSingleHeader(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	stubMailboxProfile(reg, "me@example.com")
	// Source message carries a LARGE attachment (already a Drive link in
	// the body — forward extracts its id into largeAttIDs without
	// re-downloading).
	stubGetMessageWithAttachments(reg, "msg_orig", []interface{}{
		map[string]interface{}{
			"id":              "src_large_1",
			"filename":        "src.bin",
			"is_inline":       false,
			"attachment_type": 2,
		},
	})
	// fetchComposeSourceMessage still calls /attachments/download_url for
	// every attachment_id, even the LARGE ones — the result map is consumed
	// only for non-LARGE entries, but the call must not 404.
	stubMessageAttachmentDownloadURLs(reg, "msg_orig", map[string]string{
		"src_large_1": "https://storage.example.com/src_large_1",
	})
	// Template carries two LARGE entries and no inline / SMALL refs, so the
	// embed-attachments code paths (which would issue extra
	// template/.../attachments/download_url calls) stay dormant.
	stubGetTemplate(reg, "9",
		[]interface{}{map[string]interface{}{"mail_address": "alice@example.com", "name": "Alice"}},
		nil, nil,
		[]interface{}{
			map[string]interface{}{
				"id": "tpl_large_a", "filename": "ta.bin", "is_inline": false, "attachment_type": 2,
			},
			map[string]interface{}{
				"id": "tpl_large_b", "filename": "tb.bin", "is_inline": false, "attachment_type": 2,
			},
		},
	)
	createStub := registerDraftCaptureStubs(reg)

	if err := runMountedMailShortcut(t, MailForward, []string{
		"+forward",
		"--message-id", "msg_orig",
		"--template-id", "9",
		"--confirm-send",
	}, f, stdout); err != nil {
		t.Fatalf("forward failed: %v", err)
	}

	raw := decodeCapturedRawEML(t, createStub.CapturedBody)
	headers := extractLargeAttachmentHeaders(t, raw)
	if len(headers) != 1 {
		t.Fatalf("expected exactly 1 X-Lms-Large-Attachment-Ids header, got %d (raw=%s)", len(headers), raw)
	}
	gotIDs := make(map[string]bool, len(headers[0]))
	for _, e := range headers[0] {
		if id, _ := e["id"].(string); id != "" {
			gotIDs[id] = true
		}
	}
	for _, want := range []string{"src_large_1", "tpl_large_a", "tpl_large_b"} {
		if !gotIDs[want] {
			t.Errorf("merged header missing id %q; got=%v (raw=%s)", want, gotIDs, raw)
		}
	}
	if len(gotIDs) != 3 {
		t.Errorf("merged header has %d unique ids, want 3; got=%v", len(gotIDs), gotIDs)
	}
}

// ---------------------------------------------------------------------------
// CR follow-up #3 — +template-update must drop existing inline attachments
// whose CID is no longer referenced by the new template_content. Without
// pruning, every <img> replace/delete leaves an orphan Drive-backed row in
// tpl.Attachments and the template eventually trips TemplateTotalSizeLimit.
// ---------------------------------------------------------------------------

// TestMailTemplateUpdate_OrphanInlineCIDPruned covers the core regression:
// the GET response carries an inline attachment referenced by a cid: link in
// the old template_content; --set-template-content rewrites the body to one
// that no longer references that cid; the PUT body must omit the orphan.
func TestMailTemplateUpdate_OrphanInlineCIDPruned(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "100",
					"name":               "Tpl",
					"subject":            "subj",
					"template_content":   `<p>old</p><img src="cid:abc">`,
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{
							"id":              "img_abc",
							"filename":        "abc.png",
							"is_inline":       true,
							"cid":             "abc",
							"attachment_type": 1,
						},
					},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/100",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": map[string]interface{}{"template_id": "100"}},
		},
	}
	reg.Register(putStub)

	if err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "100",
		"--set-template-content", "<p>new body without any inline image</p>",
	}, f, stdout); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tpl := body["template"].(map[string]interface{})
	atts, _ := tpl["attachments"].([]interface{})
	if len(atts) != 0 {
		t.Errorf("expected attachments[] to be empty after orphan prune, got %d entries: %#v", len(atts), atts)
	}
}

// TestMailTemplateUpdate_StillReferencedInlineKept confirms the prune is not
// over-eager: an inline whose cid: link is preserved by the new body must
// stay in attachments[] so the rendered preview / send still resolves it.
func TestMailTemplateUpdate_StillReferencedInlineKept(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/101",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "101",
					"name":               "Tpl",
					"template_content":   `<p>old</p><img src="cid:keep">`,
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{
							"id":              "img_keep",
							"filename":        "k.png",
							"is_inline":       true,
							"cid":             "keep",
							"attachment_type": 1,
						},
					},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/101",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": map[string]interface{}{"template_id": "101"}},
		},
	}
	reg.Register(putStub)

	if err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "101",
		// New body still uses cid:keep → the inline must survive the prune.
		"--set-template-content", `<p>updated body</p><img src="cid:keep">`,
	}, f, stdout); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tpl := body["template"].(map[string]interface{})
	atts, _ := tpl["attachments"].([]interface{})
	if len(atts) != 1 {
		t.Fatalf("expected 1 inline attachment kept, got %d: %#v", len(atts), atts)
	}
	att := atts[0].(map[string]interface{})
	if att["cid"] != "keep" {
		t.Errorf("kept attachment cid = %v, want \"keep\"", att["cid"])
	}
}

// TestMailTemplateUpdate_NonInlinePreservedOnContentChange guards the
// non-inline branch of the prune: SMALL non-inline rows have no cid: ref to
// match against and must always be carried forward, even when the body
// changed.
func TestMailTemplateUpdate_NonInlinePreservedOnContentChange(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/102",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "102",
					"name":               "Tpl",
					"template_content":   "<p>old</p>",
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{
							"id":              "doc_1",
							"filename":        "report.pdf",
							"is_inline":       false,
							"attachment_type": 1,
						},
					},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/102",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": map[string]interface{}{"template_id": "102"}},
		},
	}
	reg.Register(putStub)

	if err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "102",
		"--set-template-content", "<p>completely new body</p>",
	}, f, stdout); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tpl := body["template"].(map[string]interface{})
	atts, _ := tpl["attachments"].([]interface{})
	if len(atts) != 1 {
		t.Fatalf("expected 1 non-inline attachment preserved, got %d: %#v", len(atts), atts)
	}
	att := atts[0].(map[string]interface{})
	if att["id"] != "doc_1" || att["is_inline"] != false {
		t.Errorf("non-inline attachment lost or mutated: %#v", att)
	}
}

// TestMailTemplateUpdate_NoContentChangeKeepsAllInline guards the body-not-
// touched escape hatch: the prune must skip itself when --set-template-
// content / --set-template-content-file / patch-file did not modify the
// content. The fetched cid: refs in the stored body still address every
// existing inline row, so removing any would break the template.
func TestMailTemplateUpdate_NoContentChangeKeepsAllInline(t *testing.T) {
	f, stdout, _, reg := mailShortcutTestFactoryWithSendScope(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/templates/103",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"template": map[string]interface{}{
					"template_id":        "103",
					"name":               "Tpl",
					"template_content":   `<p>has body</p><img src="cid:keep1"><img src="cid:keep2">`,
					"is_plain_text_mode": false,
					"attachments": []interface{}{
						map[string]interface{}{
							"id":              "img_1",
							"filename":        "a.png",
							"is_inline":       true,
							"cid":             "keep1",
							"attachment_type": 1,
						},
						map[string]interface{}{
							"id":              "img_2",
							"filename":        "b.png",
							"is_inline":       true,
							"cid":             "keep2",
							"attachment_type": 1,
						},
					},
				},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/templates/103",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"template": map[string]interface{}{"template_id": "103"}},
		},
	}
	reg.Register(putStub)

	// Only --set-subject is changed — content path is untouched.
	if err := runMountedMailShortcut(t, MailTemplateUpdate, []string{
		"+template-update",
		"--template-id", "103",
		"--set-subject", "renamed",
	}, f, stdout); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	body := decodeCapturedBody(t, putStub)
	tpl := body["template"].(map[string]interface{})
	atts, _ := tpl["attachments"].([]interface{})
	if len(atts) != 2 {
		t.Fatalf("expected 2 inline attachments preserved, got %d: %#v", len(atts), atts)
	}
}
