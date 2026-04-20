// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// insert_signature — basic insertion into HTML body
// ---------------------------------------------------------------------------

func TestInsertSignature_BasicHTML(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Sig test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:                    "insert_signature",
			SignatureID:           "sig-123",
			RenderedSignatureHTML: "<div>-- My Signature</div>",
		}},
	})
	if err != nil {
		t.Fatalf("Apply insert_signature: %v", err)
	}

	html := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body)
	if !strings.Contains(html, "My Signature") {
		t.Error("signature not found in HTML body")
	}
	if !strings.Contains(html, `class="lark-mail-signature"`) {
		t.Error("signature wrapper class not found")
	}
	if !strings.Contains(html, `id="sig-123"`) {
		t.Error("signature ID not found")
	}
	// Body text should come before signature
	bodyIdx := strings.Index(html, "Hello")
	sigIdx := strings.Index(html, "My Signature")
	if bodyIdx > sigIdx {
		t.Error("signature should appear after body text")
	}
}

// ---------------------------------------------------------------------------
// insert_signature — with quoted content (reply/forward)
// ---------------------------------------------------------------------------

func TestInsertSignature_BeforeQuote(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Reply with sig
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>My reply</p><div id="lark-mail-quote-cli123" class="history-quote-wrapper"><div>quoted content</div></div>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:                    "insert_signature",
			SignatureID:           "sig-456",
			RenderedSignatureHTML: "<div>-- Reply Sig</div>",
		}},
	})
	if err != nil {
		t.Fatalf("Apply insert_signature: %v", err)
	}

	html := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body)
	sigIdx := strings.Index(html, "Reply Sig")
	quoteIdx := strings.Index(html, "quoted content")
	if sigIdx < 0 || quoteIdx < 0 {
		t.Fatalf("missing signature or quote in: %s", html)
	}
	if sigIdx > quoteIdx {
		t.Error("signature should appear before quote block")
	}
}

// ---------------------------------------------------------------------------
// insert_signature — replaces existing signature
// ---------------------------------------------------------------------------

func TestInsertSignature_ReplacesExisting(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Replace sig
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p><div id="old-sig" class="lark-mail-signature" style="padding-top:6px;padding-bottom:6px"><div>-- Old Sig</div></div>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:                    "insert_signature",
			SignatureID:           "new-sig",
			RenderedSignatureHTML: "<div>-- New Sig</div>",
		}},
	})
	if err != nil {
		t.Fatalf("Apply insert_signature: %v", err)
	}

	html := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body)
	if strings.Contains(html, "Old Sig") {
		t.Error("old signature should have been removed")
	}
	if !strings.Contains(html, "New Sig") {
		t.Error("new signature not found")
	}
}

// ---------------------------------------------------------------------------
// insert_signature — no HTML body
// ---------------------------------------------------------------------------

func TestInsertSignature_NoHTMLBody(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Plain text
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

Just plain text`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:                    "insert_signature",
			SignatureID:           "sig-x",
			RenderedSignatureHTML: "<div>sig</div>",
		}},
	})
	if err == nil {
		t.Fatal("expected error for insert_signature on plain text draft")
	}
	if !strings.Contains(err.Error(), "no HTML body") {
		t.Fatalf("expected 'no HTML body' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// remove_signature — removes existing signature
// ---------------------------------------------------------------------------

func TestRemoveSignature_Basic(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Remove sig
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p><div id="sig-rm" class="lark-mail-signature" style="padding-top:6px;padding-bottom:6px"><div>-- My Sig</div></div>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_signature"}},
	})
	if err != nil {
		t.Fatalf("Apply remove_signature: %v", err)
	}

	html := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body)
	if strings.Contains(html, "My Sig") {
		t.Error("signature should have been removed")
	}
	if strings.Contains(html, "lark-mail-signature") {
		t.Error("signature wrapper should have been removed")
	}
	if !strings.Contains(html, "Hello") {
		t.Error("body text should be preserved")
	}
}

// ---------------------------------------------------------------------------
// remove_signature — no signature present
// ---------------------------------------------------------------------------

func TestRemoveSignature_NoSignature(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: No sig
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>No signature here</p>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_signature"}},
	})
	if err == nil {
		t.Fatal("expected error when removing non-existent signature")
	}
	if !strings.Contains(err.Error(), "no signature found") {
		t.Fatalf("expected 'no signature found' error, got: %v", err)
	}
}
