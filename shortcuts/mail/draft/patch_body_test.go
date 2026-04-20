// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// set_body on HTML-only draft (no text/plain part)
// ---------------------------------------------------------------------------

func TestApplySetBodyHTMLOnlyDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: HTML only
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>hello</p>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<div>updated</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if part == nil {
		t.Fatalf("HTML part missing")
	}
	if got := string(part.Body); got != "<div>updated</div>" {
		t.Fatalf("body = %q", got)
	}
}

// ---------------------------------------------------------------------------
// set_body on draft with no primary body parts
// ---------------------------------------------------------------------------

func TestApplySetBodyNoPrimaryBodyFails(t *testing.T) {
	// A multipart/signed draft has no editable primary body
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/multipart_signed_draft.eml"))
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "anything"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no unique primary body") {
		t.Fatalf("error = %v, want no primary body error", err)
	}
}

// ---------------------------------------------------------------------------
// set_reply_body on reply draft with quote — preserves quote
// ---------------------------------------------------------------------------

func TestApplySetReplyBodyPreservesQuote(t *testing.T) {
	quoteHTML := `<div class="history-quote-wrapper"><div data-html-block="quote"><div class="adit-html-block adit-html-block--collapsed"><div><div>original quoted message</div></div></div></div></div>`
	snapshot := mustParseFixtureDraft(t, `Subject: Re: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>old reply</div>`+quoteHTML+`
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: "<div>new reply</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if part == nil {
		t.Fatalf("HTML part missing")
	}
	got := string(part.Body)
	if !strings.HasPrefix(got, "<div>new reply</div>") {
		t.Fatalf("body should start with new reply content, got %q", got)
	}
	if !strings.Contains(got, "history-quote-wrapper") {
		t.Fatalf("body should preserve history-quote-wrapper, got %q", got)
	}
	if !strings.Contains(got, "original quoted message") {
		t.Fatalf("body should preserve original quoted message, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// set_reply_body on forward draft with id+class — preserves quote
// ---------------------------------------------------------------------------

func TestApplySetReplyBodyPreservesForwardQuote(t *testing.T) {
	quoteHTML := `<div id="lark-mail-quote-cli123456" class="history-quote-wrapper"><div data-html-block="quote"><div class="adit-html-block adit-html-block--header"><div id="lark-mail-quote-cli654321">forwarded content</div></div></div></div>`
	snapshot := mustParseFixtureDraft(t, `Subject: Fwd: Hello
From: Alice <alice@example.com>
To: Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>old note</div>`+quoteHTML+`
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: "<div>updated note</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	got := string(part.Body)
	if !strings.HasPrefix(got, "<div>updated note</div>") {
		t.Fatalf("body should start with updated note, got %q", got)
	}
	if !strings.Contains(got, "forwarded content") {
		t.Fatalf("body should preserve forwarded content, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// set_reply_body on draft without quote — falls back to set_body
// ---------------------------------------------------------------------------

func TestApplySetReplyBodyNoQuoteFallsBackToSetBody(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>original body</p>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: "<div>replaced</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	if got := string(part.Body); got != "<div>replaced</div>" {
		t.Fatalf("body = %q, want <div>replaced</div>", got)
	}
}

// ---------------------------------------------------------------------------
// set_reply_body on coupled (text/plain + text/html) reply draft
// ---------------------------------------------------------------------------

func TestApplySetReplyBodyCoupledDraftPreservesQuote(t *testing.T) {
	quoteHTML := `<div class="history-quote-wrapper"><div>quoted</div></div>`
	snapshot := mustParseFixtureDraft(t, `Subject: Re: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

old reply
--alt
Content-Type: text/html; charset=UTF-8

<div>old reply</div>`+quoteHTML+`
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: "<div>new reply</div>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	htmlPart := findPart(snapshot.Body, snapshot.PrimaryHTMLPartID)
	got := string(htmlPart.Body)
	if !strings.HasPrefix(got, "<div>new reply</div>") {
		t.Fatalf("HTML body should start with new reply, got %q", got)
	}
	if !strings.Contains(got, "history-quote-wrapper") {
		t.Fatalf("HTML body should preserve quote, got %q", got)
	}
	// Check that the plain-text summary was regenerated
	textPart := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	if textPart == nil {
		t.Fatalf("text part missing")
	}
	if !strings.Contains(string(textPart.Body), "new reply") {
		t.Fatalf("plain-text summary not regenerated, got %q", string(textPart.Body))
	}
}

// ---------------------------------------------------------------------------
// set_reply_body on plain-text-only draft — falls back to set_body
// ---------------------------------------------------------------------------

func TestApplySetReplyBodyPlainTextOnlyDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Hello
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

original text
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_reply_body", Value: "replaced text"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	if got := string(part.Body); got != "replaced text" {
		t.Fatalf("body = %q, want replaced text", got)
	}
}

// ---------------------------------------------------------------------------
// replace_body / append_body on plain-text only draft
// ---------------------------------------------------------------------------

func TestApplyReplaceBodyPlainTextOnly(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

original content
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/plain", Value: "replaced content"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	if got := string(part.Body); got != "replaced content" {
		t.Fatalf("body = %q", got)
	}
}

func TestApplyAppendBodyPlainTextOnly(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

original
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Value: " appended"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	part := findPart(snapshot.Body, snapshot.PrimaryTextPartID)
	if got := string(part.Body); got != "original\n appended" {
		t.Fatalf("body = %q", got)
	}
}

// ---------------------------------------------------------------------------
// replace_body with unsupported body_kind
// ---------------------------------------------------------------------------

func TestApplyReplaceBodyUnsupportedKind(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/csv", Value: "data"}},
	})
	if err == nil || !strings.Contains(err.Error(), "body_kind must be text/plain or text/html") {
		t.Fatalf("error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// replace_body on a draft that has no primary HTML part (without rewrite)
// ---------------------------------------------------------------------------

func TestApplyReplaceBodyMissingPartWithoutRewriteFails(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/html", Value: "<p>new</p>"}},
	})
	if err == nil || !strings.Contains(err.Error(), "no primary text/html body part") {
		t.Fatalf("error = %v", err)
	}
}

// ---------------------------------------------------------------------------
// set_body with coupled body parts: non-HTML input rejected
// ---------------------------------------------------------------------------

func TestApplySetBodyCoupledRejectsNonHTML(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

summary
--alt
Content-Type: text/html; charset=UTF-8

<p>real body</p>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "just plain text without any tags"}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires HTML input") {
		t.Fatalf("error = %v, want HTML input required", err)
	}
}

// ---------------------------------------------------------------------------
// Multiple ops in a single patch
// ---------------------------------------------------------------------------

func TestApplyMultipleOpsInOnePatch(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "set_subject", Value: "Updated Subject"},
			{Op: "add_recipient", Field: "cc", Name: "Carol", Address: "carol@example.com"},
			{Op: "set_header", Name: "X-Custom", Value: "custom-value"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if snapshot.Subject != "Updated Subject" {
		t.Fatalf("Subject = %q", snapshot.Subject)
	}
	if len(snapshot.Cc) != 1 || snapshot.Cc[0].Address != "carol@example.com" {
		t.Fatalf("Cc = %#v", snapshot.Cc)
	}
	if got := headerValue(snapshot.Headers, "X-Custom"); got != "custom-value" {
		t.Fatalf("X-Custom = %q", got)
	}
}
