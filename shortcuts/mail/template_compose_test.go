// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseLocalImgs
// ---------------------------------------------------------------------------

// TestParseLocalImgs_OrderAndDedup verifies parse local imgs order and dedup.
func TestParseLocalImgs_OrderAndDedup(t *testing.T) {
	html := `<p>hi</p><img src="a.png"><img src='b.png'><IMG SRC="a.png">`
	got := parseLocalImgs(html)
	if len(got) != 3 {
		t.Fatalf("expected 3 imgs (duplicates preserved), got %d: %#v", len(got), got)
	}
	if got[0].Path != "a.png" || got[1].Path != "b.png" || got[2].Path != "a.png" {
		t.Fatalf("unexpected order: %#v", got)
	}
}

// TestParseLocalImgs_SkipRemoteAndSchemes verifies parse local imgs skip remote and schemes.
func TestParseLocalImgs_SkipRemoteAndSchemes(t *testing.T) {
	html := `<img src="https://example.com/x.png"><img src="//cdn/y.png"><img src="data:image/png;base64,AAAA"><img src="cid:foo"><img src="local.png">`
	got := parseLocalImgs(html)
	if len(got) != 1 || got[0].Path != "local.png" {
		t.Fatalf("expected only local.png, got %#v", got)
	}
}

// TestParseLocalImgs_EmptySrcDropped verifies parse local imgs empty src dropped.
func TestParseLocalImgs_EmptySrcDropped(t *testing.T) {
	html := `<img src="">`
	if got := parseLocalImgs(html); len(got) != 0 {
		t.Fatalf("expected empty, got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// replaceImgSrcOnce
// ---------------------------------------------------------------------------

// TestReplaceImgSrcOnce_Basic verifies replace img src once basic.
func TestReplaceImgSrcOnce_Basic(t *testing.T) {
	html := `<img src="a.png"><img src="a.png">`
	got := replaceImgSrcOnce(html, "a.png", "cid:1")
	want := `<img src="cid:1"><img src="a.png">`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestReplaceImgSrcOnce_NoMatch verifies replace img src once no match.
func TestReplaceImgSrcOnce_NoMatch(t *testing.T) {
	html := `<img src="a.png">`
	got := replaceImgSrcOnce(html, "missing.png", "cid:x")
	if got != html {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// templateMailboxPath / validateTemplateID
// ---------------------------------------------------------------------------

// TestTemplateMailboxPath verifies template mailbox path.
func TestTemplateMailboxPath(t *testing.T) {
	cases := []struct {
		mbox     string
		segments []string
		want     string
	}{
		{"u1", nil, "/open-apis/mail/v1/user_mailboxes/u1/templates"},
		{"u1", []string{"42"}, "/open-apis/mail/v1/user_mailboxes/u1/templates/42"},
		{"u 1", []string{"42", "", "attachments"}, "/open-apis/mail/v1/user_mailboxes/u%201/templates/42/attachments"},
	}
	for _, c := range cases {
		if got := templateMailboxPath(c.mbox, c.segments...); got != c.want {
			t.Errorf("templateMailboxPath(%q, %v) = %q; want %q", c.mbox, c.segments, got, c.want)
		}
	}
}

// TestValidateTemplateID verifies validate template id.
func TestValidateTemplateID(t *testing.T) {
	if err := validateTemplateID(""); err != nil {
		t.Errorf("empty id should be allowed, got %v", err)
	}
	if err := validateTemplateID("12345"); err != nil {
		t.Errorf("decimal id should be allowed, got %v", err)
	}
	if err := validateTemplateID("abc"); err == nil {
		t.Errorf("non-decimal id should be rejected")
	}
	if err := validateTemplateID("-1"); err != nil {
		t.Errorf("negative decimal should parse, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// renderTemplateAddresses / joinTemplateAddresses / appendAddrList
// ---------------------------------------------------------------------------

// TestRenderAndJoinAddresses verifies render and join addresses.
func TestRenderAndJoinAddresses(t *testing.T) {
	addrs := renderTemplateAddresses("Alice <a@x>, b@x")
	if len(addrs) != 2 {
		t.Fatalf("expected 2 addrs, got %#v", addrs)
	}
	if addrs[0].Name != "Alice" || addrs[0].Address != "a@x" {
		t.Fatalf("unexpected addr[0]: %#v", addrs[0])
	}
	if addrs[1].Name != "" || addrs[1].Address != "b@x" {
		t.Fatalf("unexpected addr[1]: %#v", addrs[1])
	}

	joined := joinTemplateAddresses(addrs)
	if !strings.Contains(joined, "a@x") || !strings.Contains(joined, "b@x") {
		t.Fatalf("joined missing addresses: %q", joined)
	}

	if got := renderTemplateAddresses(""); got != nil {
		t.Errorf("empty input should return nil, got %#v", got)
	}
	if got := joinTemplateAddresses(nil); got != "" {
		t.Errorf("nil input should return empty, got %q", got)
	}
	// Skip entries with empty Address.
	mix := []templateMailAddr{{Address: ""}, {Address: "x@x"}}
	if got := joinTemplateAddresses(mix); got != "x@x" {
		t.Errorf("expected 'x@x', got %q", got)
	}
}

// TestAppendAddrList verifies append addr list.
func TestAppendAddrList(t *testing.T) {
	if got := appendAddrList("", "b@x"); got != "b@x" {
		t.Errorf("empty base, got %q", got)
	}
	if got := appendAddrList("a@x", ""); got != "a@x" {
		t.Errorf("empty extra, got %q", got)
	}
	if got := appendAddrList("a@x", "b@x"); got != "a@x, b@x" {
		t.Errorf("concat, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// generateTemplateCID / b64StdEncode
// ---------------------------------------------------------------------------

// TestGenerateTemplateCID verifies generate template c i d.
func TestGenerateTemplateCID(t *testing.T) {
	a, err := generateTemplateCID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := generateTemplateCID()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == b {
		t.Errorf("expected unique cids, got duplicate %q", a)
	}
	if len(a) < 32 {
		t.Errorf("cid too short: %q", a)
	}
}

// TestB64StdEncode verifies b64 std encode.
func TestB64StdEncode(t *testing.T) {
	got := b64StdEncode([]byte("hello"))
	want := base64.StdEncoding.EncodeToString([]byte("hello"))
	if got != want {
		t.Errorf("b64StdEncode = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// sortStrings / countAddresses / countAttachmentsByType
// ---------------------------------------------------------------------------

// TestSortStrings verifies sort strings.
func TestSortStrings(t *testing.T) {
	s := []string{"c", "a", "b", "a"}
	sortStrings(s)
	want := []string{"a", "a", "b", "c"}
	for i := range s {
		if s[i] != want[i] {
			t.Fatalf("sortStrings = %v; want %v", s, want)
		}
	}
}

// TestCountAddresses verifies count addresses.
func TestCountAddresses(t *testing.T) {
	if got := countAddresses(""); got != 0 {
		t.Errorf("empty -> 0, got %d", got)
	}
	if got := countAddresses("a@x, b@x"); got != 2 {
		t.Errorf("two -> 2, got %d", got)
	}
}

// TestCountAttachmentsByType verifies count attachments by type.
func TestCountAttachmentsByType(t *testing.T) {
	atts := []templateAttachment{
		{IsInline: true, AttachmentType: attachmentTypeSmall},
		{IsInline: false, AttachmentType: attachmentTypeSmall},
		{IsInline: false, AttachmentType: attachmentTypeLarge},
		{IsInline: true, AttachmentType: attachmentTypeSmall},
	}
	inlineCnt, largeCnt := countAttachmentsByType(atts)
	if inlineCnt != 2 {
		t.Errorf("inlineCnt = %d; want 2", inlineCnt)
	}
	if largeCnt != 1 {
		t.Errorf("largeCnt = %d; want 1", largeCnt)
	}
}

// ---------------------------------------------------------------------------
// templateAttachmentBuilder
// ---------------------------------------------------------------------------

// TestTemplateAttachmentBuilder_Small verifies small attachments stay SMALL and in projectedSize.
func TestTemplateAttachmentBuilder_Small(t *testing.T) {
	b := newTemplateAttachmentBuilder("n", "s", "c", nil, nil, nil)
	b.append("k1", "a.png", "cid1", true, 1024)
	b.append("k2", "b.bin", "", false, 2048)
	if err := b.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if len(b.attachments) != 2 {
		t.Fatalf("attachments: %#v", b.attachments)
	}
	if b.attachments[0].AttachmentType != attachmentTypeSmall {
		t.Errorf("inline should be SMALL, got %d", b.attachments[0].AttachmentType)
	}
	if b.attachments[1].AttachmentType != attachmentTypeSmall {
		t.Errorf("non-inline small should be SMALL, got %d", b.attachments[1].AttachmentType)
	}
	if b.attachments[0].Body != "k1" {
		t.Errorf("body should mirror ID, got %q", b.attachments[0].Body)
	}
}

// TestTemplateAttachmentBuilder_LargeSwitch verifies non-inline flips to LARGE once projection exceeds threshold.
func TestTemplateAttachmentBuilder_LargeSwitch(t *testing.T) {
	b := newTemplateAttachmentBuilder("n", "s", strings.Repeat("x", 1024), nil, nil, nil)
	// One big non-inline pushes the cumulative projection past 25 MB.
	big := int64(30 * 1024 * 1024)
	b.append("k1", "huge.bin", "", false, big)
	if err := b.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if got := b.attachments[0].AttachmentType; got != attachmentTypeLarge {
		t.Errorf("expected LARGE for big file, got %d", got)
	}
	// Once bucket flips, subsequent non-inline also LARGE.
	b.append("k2", "small.bin", "", false, 1024)
	if got := b.attachments[1].AttachmentType; got != attachmentTypeLarge {
		t.Errorf("sticky LARGE bucket should apply, got %d", got)
	}
}

// TestTemplateAttachmentBuilder_InlineOverflowSurfaces verifies inline-only overflow is surfaced at finalize.
func TestTemplateAttachmentBuilder_InlineOverflowSurfaces(t *testing.T) {
	b := newTemplateAttachmentBuilder("n", "s", "", nil, nil, nil)
	// Inline images cannot flip to LARGE; their raw bytes count toward the 25 MB cap.
	b.append("k1", "big.png", "cid1", true, 30*1024*1024)
	if err := b.finalize(); err == nil {
		t.Errorf("expected finalize error for inline overflow")
	}
}

// ---------------------------------------------------------------------------
// wrapTemplateContentIfNeeded
// ---------------------------------------------------------------------------

// TestWrapTemplateContentIfNeeded verifies the wrap behavior. Plain-text
// templates also get HTML-wrapped here so the preview keeps line breaks; the
// is_plain_text_mode flag is honored on the apply/send side via a HTML→text
// strip pass in mergeTemplateBody.
func TestWrapTemplateContentIfNeeded(t *testing.T) {
	if got := wrapTemplateContentIfNeeded("", false); got != "" {
		t.Errorf("empty pass-through, got %q", got)
	}
	if got := wrapTemplateContentIfNeeded("<p>x</p>", false); got != "<p>x</p>" {
		t.Errorf("already-HTML should pass through, got %q", got)
	}
	// Plain text body in HTML mode → transformed.
	got := wrapTemplateContentIfNeeded("line1\nline2", false)
	if got == "line1\nline2" || !strings.Contains(got, "line1") || !strings.Contains(got, "<br>") {
		t.Errorf("expected wrapped body with <br>, got %q", got)
	}
	// Plain text body in plain-text mode → ALSO transformed so the preview
	// shows line breaks. The flag does not gate the wrap.
	gotPT := wrapTemplateContentIfNeeded("hi\nthere", true)
	if !strings.Contains(gotPT, "hi") || !strings.Contains(gotPT, "<br>") || !strings.Contains(gotPT, "there") {
		t.Errorf("plain-text should also be wrapped, got %q", gotPT)
	}
}

// ---------------------------------------------------------------------------
// encodeTemplateLargeAttachmentHeader
// ---------------------------------------------------------------------------

// TestEncodeTemplateLargeAttachmentHeader verifies encode template large attachment header.
func TestEncodeTemplateLargeAttachmentHeader(t *testing.T) {
	got, err := encodeTemplateLargeAttachmentHeader(nil)
	if err != nil || got != "" {
		t.Fatalf("nil -> empty, got %q err=%v", got, err)
	}
	got, err = encodeTemplateLargeAttachmentHeader([]string{"", "a", "a", "b"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	var ids []largeAttID
	if err := json.Unmarshal(dec, &ids); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(ids) != 2 || ids[0].ID != "a" || ids[1].ID != "b" {
		t.Errorf("unexpected dedup/order: %#v", ids)
	}
}

// ---------------------------------------------------------------------------
// extractTemplatePayload
// ---------------------------------------------------------------------------

// TestExtractTemplatePayload_Wrapped verifies the common "template" wrapper.
func TestExtractTemplatePayload_Wrapped(t *testing.T) {
	data := map[string]interface{}{
		"template": map[string]interface{}{
			"template_id": "42",
			"name":        "Quarterly",
			"subject":     "Q4",
			"tos": []interface{}{
				map[string]interface{}{"mail_address": "a@x", "name": "Alice"},
			},
		},
	}
	tpl, err := extractTemplatePayload(data)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if tpl.TemplateID != "42" || tpl.Name != "Quarterly" || tpl.Subject != "Q4" {
		t.Errorf("unexpected payload: %+v", tpl)
	}
	if len(tpl.Tos) != 1 || tpl.Tos[0].Address != "a@x" {
		t.Errorf("unexpected tos: %#v", tpl.Tos)
	}
}

// TestExtractTemplatePayload_Unwrapped verifies the unwrapped form.
func TestExtractTemplatePayload_Unwrapped(t *testing.T) {
	data := map[string]interface{}{
		"template_id": "7",
		"name":        "Direct",
	}
	tpl, err := extractTemplatePayload(data)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if tpl.TemplateID != "7" || tpl.Name != "Direct" {
		t.Errorf("unexpected payload: %+v", tpl)
	}
}

// ---------------------------------------------------------------------------
// applyTemplate — recipient / body / attachment classification matrix
// ---------------------------------------------------------------------------

// TestApplyTemplate_SendRecipientsFlow exercises the send/draft-create path:
// user flags override draft-derived, template tos/ccs/bccs are appended, and
// subject follows precedence user > draft > template.
func TestApplyTemplate_SendRecipientsFlow(t *testing.T) {
	tpl := &templatePayload{
		Subject:         "tpl-subject",
		TemplateContent: "<p>tpl body</p>",
		Tos:             []templateMailAddr{{Address: "t@x"}},
		Ccs:             []templateMailAddr{{Address: "c@x"}},
		Bccs:            []templateMailAddr{{Address: "b@x"}},
	}
	merged := applyTemplate(
		templateShortcutSend, tpl,
		"", "", "", // no draft-derived
		"", "", // no draft subject/body
		"user-to@x", "user-cc@x", "user-bcc@x", "", "", // user flags
	)
	if !strings.Contains(merged.To, "user-to@x") || !strings.Contains(merged.To, "t@x") {
		t.Errorf("To missing entries: %q", merged.To)
	}
	if !strings.Contains(merged.Cc, "user-cc@x") || !strings.Contains(merged.Cc, "c@x") {
		t.Errorf("Cc missing entries: %q", merged.Cc)
	}
	if !strings.Contains(merged.Bcc, "user-bcc@x") || !strings.Contains(merged.Bcc, "b@x") {
		t.Errorf("Bcc missing entries: %q", merged.Bcc)
	}
	if merged.Subject != "tpl-subject" {
		t.Errorf("Subject fallback should be template, got %q", merged.Subject)
	}
	if merged.Body != "<p>tpl body</p>" {
		t.Errorf("empty draft body should use template, got %q", merged.Body)
	}
}

// TestApplyTemplate_UserSubjectWins verifies that an explicit user subject
// takes precedence over both draft and template subjects.
func TestApplyTemplate_UserSubjectWins(t *testing.T) {
	tpl := &templatePayload{Subject: "tpl"}
	merged := applyTemplate(
		templateShortcutSend, tpl,
		"", "", "", "draft-subj", "",
		"", "", "", "user-subj", "",
	)
	if merged.Subject != "user-subj" {
		t.Errorf("user subject should win, got %q", merged.Subject)
	}
}

// TestApplyTemplate_DraftSubjectWinsOverTemplate verifies draft > template
// when no user subject.
func TestApplyTemplate_DraftSubjectWinsOverTemplate(t *testing.T) {
	tpl := &templatePayload{Subject: "tpl"}
	merged := applyTemplate(
		templateShortcutSend, tpl,
		"", "", "", "draft-subj", "",
		"", "", "", "", "",
	)
	if merged.Subject != "draft-subj" {
		t.Errorf("draft subject should win over template, got %q", merged.Subject)
	}
}

// TestApplyTemplate_ReplyWarnsWhenTemplateHasRecipients verifies the warning
// emitted for reply/reply-all with template-side tos/ccs/bccs.
func TestApplyTemplate_ReplyWarnsWhenTemplateHasRecipients(t *testing.T) {
	tpl := &templatePayload{
		Tos:             []templateMailAddr{{Address: "t@x"}},
		TemplateContent: "body",
	}
	merged := applyTemplate(
		templateShortcutReplyAll, tpl,
		"orig-to@x", "", "", "Re: foo", "",
		"", "", "", "", "",
	)
	if len(merged.Warnings) == 0 {
		t.Errorf("expected warning, got none")
	}
}

// TestApplyTemplate_AttachmentClassification verifies inline/SMALL/LARGE are
// routed into the correct output channels and anomalies surface warnings.
func TestApplyTemplate_AttachmentClassification(t *testing.T) {
	tpl := &templatePayload{
		Attachments: []templateAttachment{
			{ID: "k1", Filename: "img.png", CID: "cid1", IsInline: true, AttachmentType: attachmentTypeSmall},
			{ID: "k2", Filename: "file.pdf", IsInline: false, AttachmentType: attachmentTypeSmall},
			{ID: "k3", Filename: "big.zip", IsInline: false, AttachmentType: attachmentTypeLarge},
			// Anomaly: inline without CID → dropped with warning.
			{ID: "k4", Filename: "nocid.png", IsInline: true, AttachmentType: attachmentTypeSmall},
			// Anomaly: inline but LARGE → dropped with warning.
			{ID: "k5", Filename: "huge.png", CID: "cid5", IsInline: true, AttachmentType: attachmentTypeLarge},
			// Entry with no ID → silently dropped.
			{ID: "", Filename: "nope"},
		},
	}
	merged := applyTemplate(
		templateShortcutSend, tpl,
		"", "", "", "", "",
		"to@x", "", "", "", "",
	)
	if len(merged.InlineAttachments) != 1 || merged.InlineAttachments[0].FileKey != "k1" {
		t.Errorf("expected 1 inline ref k1, got %#v", merged.InlineAttachments)
	}
	if len(merged.SmallAttachments) != 1 || merged.SmallAttachments[0].FileKey != "k2" {
		t.Errorf("expected 1 small ref k2, got %#v", merged.SmallAttachments)
	}
	if len(merged.LargeAttachmentIDs) != 1 || merged.LargeAttachmentIDs[0] != "k3" {
		t.Errorf("expected 1 large id k3, got %#v", merged.LargeAttachmentIDs)
	}
	// Two warnings (no-cid, inline-LARGE). Unknown IDs are silent.
	warnCnt := 0
	for _, w := range merged.Warnings {
		if strings.Contains(w, "nocid.png") || strings.Contains(w, "huge.png") {
			warnCnt++
		}
	}
	if warnCnt != 2 {
		t.Errorf("expected 2 anomaly warnings, got %d (%v)", warnCnt, merged.Warnings)
	}
}

// TestApplyTemplate_IsPlainTextPropagation verifies propagation of the
// template's is_plain_text_mode into the merged result.
func TestApplyTemplate_IsPlainTextPropagation(t *testing.T) {
	tpl := &templatePayload{IsPlainTextMode: true}
	merged := applyTemplate(templateShortcutSend, tpl, "", "", "", "", "", "to@x", "", "", "", "")
	if !merged.IsPlainTextMode {
		t.Errorf("expected IsPlainTextMode propagated")
	}
}

// ---------------------------------------------------------------------------
// mergeTemplateBody — HTML & plain-text paths for send/reply/forward
// ---------------------------------------------------------------------------

// TestMergeTemplateBody_SendHTMLWithSeparator verifies the <br><br> separator
// is inserted between user/draft body and template body for HTML sends.
func TestMergeTemplateBody_SendHTMLWithSeparator(t *testing.T) {
	tpl := &templatePayload{TemplateContent: "<p>tpl</p>"}
	got := mergeTemplateBody(templateShortcutSend, tpl, "<p>draft</p>", "")
	if !strings.Contains(got, "<br><br>") {
		t.Errorf("expected <br><br> separator, got %q", got)
	}
	if !strings.Contains(got, "<p>draft</p>") || !strings.Contains(got, "<p>tpl</p>") {
		t.Errorf("both fragments should appear, got %q", got)
	}
}

// TestMergeTemplateBody_PlainTextSend verifies plain-text send uses \n\n
// separator when draft body is non-empty.
func TestMergeTemplateBody_PlainTextSend(t *testing.T) {
	tpl := &templatePayload{IsPlainTextMode: true, TemplateContent: "tpl"}
	if got := mergeTemplateBody(templateShortcutSend, tpl, "draft", ""); got != "draft\n\ntpl" {
		t.Errorf("got %q; want %q", got, "draft\n\ntpl")
	}
	if got := mergeTemplateBody(templateShortcutSend, tpl, "   ", ""); got != "tpl" {
		t.Errorf("empty-draft should return tpl only, got %q", got)
	}
}

// TestMergeTemplateBody_UserBodyReplacesDraft verifies that a non-empty
// userBody takes precedence over draftBody for merging.
func TestMergeTemplateBody_UserBodyReplacesDraft(t *testing.T) {
	tpl := &templatePayload{TemplateContent: "<p>tpl</p>"}
	got := mergeTemplateBody(templateShortcutSend, tpl, "<p>draft</p>", "<p>user</p>")
	if !strings.Contains(got, "<p>user</p>") || strings.Contains(got, "<p>draft</p>") {
		t.Errorf("userBody should replace draftBody, got %q", got)
	}
}

// TestMergeTemplateBody_ReplyPlainText verifies reply plain-text prepend.
func TestMergeTemplateBody_ReplyPlainText(t *testing.T) {
	tpl := &templatePayload{IsPlainTextMode: true, TemplateContent: "tpl"}
	if got := mergeTemplateBody(templateShortcutReply, tpl, "draft", ""); got != "tpl\n\ndraft" {
		t.Errorf("reply plain-text: got %q; want %q", got, "tpl\n\ndraft")
	}
	if got := mergeTemplateBody(templateShortcutReply, tpl, "", ""); got != "tpl" {
		t.Errorf("reply empty draft should return tpl, got %q", got)
	}
}

// TestMergeTemplateBody_PlainTextStripsHTML verifies plain-text-mode templates
// whose stored content is HTML-wrapped (per the preview-friendly storage
// format) get their HTML stripped back to real newlines before injection,
// so the recipient sees plain text instead of literal <div>...</div> markup.
func TestMergeTemplateBody_PlainTextStripsHTML(t *testing.T) {
	tpl := &templatePayload{
		IsPlainTextMode: true,
		TemplateContent: "<div>第一行</div><div>第二行</div><div>第三行</div>",
	}
	got := mergeTemplateBody(templateShortcutSend, tpl, "", "")
	want := "第一行\n第二行\n第三行"
	if got != want {
		t.Errorf("HTML-wrapped plain-text template should strip back to newlines\n got: %q\nwant: %q", got, want)
	}
	// buildBodyDiv-wrapped form produced by wrapTemplateContentIfNeeded for
	// CLI-created plain-text templates: round-trip should also yield clean
	// newlines.
	tpl2 := &templatePayload{
		IsPlainTextMode: true,
		TemplateContent: `<div style="word-break:break-word;line-height:1.6;font-size:14px;color:rgb(0,0,0);">a<br>b<br>c</div>`,
	}
	got = mergeTemplateBody(templateShortcutSend, tpl2, "", "")
	if got != "a\nb\nc" {
		t.Errorf("buildBodyDiv-wrapped → plain text: got %q want %q", got, "a\nb\nc")
	}
}

// TestMergeTemplateBody_ReplyHTML verifies the reply HTML merge carries both
// fragments. InsertBeforeQuoteOrAppend owns the exact placement; this test
// asserts only that the content survives a round-trip through the merge.
func TestMergeTemplateBody_ReplyHTML(t *testing.T) {
	tpl := &templatePayload{TemplateContent: "<p>tpl</p>"}
	got := mergeTemplateBody(templateShortcutReply, tpl, "<p>draft</p>", "")
	if !strings.Contains(got, "<p>tpl</p>") || !strings.Contains(got, "<p>draft</p>") {
		t.Errorf("reply HTML should contain both fragments, got %q", got)
	}
	// Empty draft body → just tpl.
	if got := mergeTemplateBody(templateShortcutReply, tpl, "", ""); got != "<p>tpl</p>" {
		t.Errorf("empty draft body should return tpl, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// logTemplateInfo — uses a runtime with a stderr buffer
// ---------------------------------------------------------------------------

// TestLogTemplateInfo verifies log template info emits info: phase: k=v lines
// in deterministic key order.
func TestLogTemplateInfo(t *testing.T) {
	rt, _, stderr := newOutputRuntime(t)
	logTemplateInfo(rt, "create.dry_run", map[string]interface{}{
		"name_len":          7,
		"attachments_total": 2,
		"inline_count":      1,
	})
	got := stderr.String()
	if !strings.HasPrefix(got, "info: template create.dry_run: ") {
		t.Errorf("prefix wrong: %q", got)
	}
	// Keys are sorted alphabetically.
	if idx1, idx2, idx3 := strings.Index(got, "attachments_total="), strings.Index(got, "inline_count="), strings.Index(got, "name_len="); idx1 < 0 || idx2 < 0 || idx3 < 0 || !(idx1 < idx2 && idx2 < idx3) {
		t.Errorf("keys out of order: %q", got)
	}
	// nil runtime shouldn't panic.
	logTemplateInfo(nil, "x", nil)
}

// ---------------------------------------------------------------------------
// applyTemplatePatchFile / buildTemplatePatchSkeleton — from mail_template_update.go
// ---------------------------------------------------------------------------

// TestApplyTemplatePatchFile_Overlay verifies that only non-nil fields overlay.
func TestApplyTemplatePatchFile_Overlay(t *testing.T) {
	base := &templatePayload{
		Name:            "orig",
		Subject:         "orig-subj",
		TemplateContent: "orig-body",
		IsPlainTextMode: false,
		Tos:             []templateMailAddr{{Address: "t@x"}},
	}
	newName := "new-name"
	applyTemplatePatchFile(base, &templatePatchFile{Name: &newName})
	if base.Name != "new-name" {
		t.Errorf("Name not overlaid: %q", base.Name)
	}
	if base.Subject != "orig-subj" || base.TemplateContent != "orig-body" {
		t.Errorf("non-patched fields mutated: %+v", base)
	}

	// All fields patched at once.
	newSubject := "s"
	newContent := "c"
	truth := true
	newTos := []templateMailAddr{{Address: "new@x"}}
	newCcs := []templateMailAddr{{Address: "newcc@x"}}
	newBccs := []templateMailAddr{{Address: "newbcc@x"}}
	applyTemplatePatchFile(base, &templatePatchFile{
		Subject:         &newSubject,
		TemplateContent: &newContent,
		IsPlainTextMode: &truth,
		Tos:             &newTos,
		Ccs:             &newCcs,
		Bccs:            &newBccs,
	})
	if base.Subject != "s" || base.TemplateContent != "c" || !base.IsPlainTextMode {
		t.Errorf("full overlay failed: %+v", base)
	}
	if len(base.Tos) != 1 || base.Tos[0].Address != "new@x" {
		t.Errorf("Tos overlay failed: %#v", base.Tos)
	}
	if len(base.Ccs) != 1 || base.Ccs[0].Address != "newcc@x" {
		t.Errorf("Ccs overlay failed: %#v", base.Ccs)
	}
	if len(base.Bccs) != 1 || base.Bccs[0].Address != "newbcc@x" {
		t.Errorf("Bccs overlay failed: %#v", base.Bccs)
	}

	// nil patch is a no-op and must not panic.
	applyTemplatePatchFile(base, nil)
}

// TestBuildTemplatePatchSkeleton verifies build template patch skeleton.
func TestBuildTemplatePatchSkeleton(t *testing.T) {
	sk := buildTemplatePatchSkeleton()
	for _, key := range []string{"name", "subject", "template_content", "is_plain_text_mode", "tos", "ccs", "bccs"} {
		if _, ok := sk[key]; !ok {
			t.Errorf("skeleton missing %q", key)
		}
	}
	// Should round-trip through json without errors.
	buf, err := json.Marshal(sk)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(buf) < 50 {
		t.Errorf("suspiciously small skeleton: %s", buf)
	}
}
