// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

var testFIO = &localfileio.LocalFileIO{}

func chdirTemp(t *testing.T) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
}

func TestApplySubjectPatchKeepsReplyHeadersAndBcc(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Bcc: Hidden <hidden@example.com>
Message-ID: <draft-1@example.com>
In-Reply-To: <orig-1@example.com>
References: <root@example.com> <orig-1@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 7bit

hello
`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Updated"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := headerValue(snapshot.Headers, "Subject"); got != "Updated" {
		t.Fatalf("Subject header = %q", got)
	}
	if got := headerValue(snapshot.Headers, "In-Reply-To"); got != "<orig-1@example.com>" {
		t.Fatalf("In-Reply-To = %q", got)
	}
	if got := headerValue(snapshot.Headers, "References"); got != "<root@example.com> <orig-1@example.com>" {
		t.Fatalf("References = %q", got)
	}
	if got := headerValue(snapshot.Headers, "Bcc"); got == "" {
		t.Fatalf("Bcc header unexpectedly dropped")
	}
}

func TestApplyProtectedHeaderBlocked(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Message-ID: <draft-1@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_header", Name: "Message-ID", Value: "<changed@example.com>"}},
	})
	if err == nil {
		t.Fatalf("expected protected header edit to fail")
	}
}

func TestApplySetRecipientsOverwritesHeader(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>, Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:    "set_recipients",
			Field: "to",
			Addresses: []Address{
				{Name: "Dave", Address: "dave@example.com"},
				{Name: "Dave Duplicate", Address: "dave@example.com"},
				{Name: "Erin", Address: "erin@example.com"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.To) != 2 {
		t.Fatalf("To addresses = %#v", snapshot.To)
	}
	if snapshot.To[0].Address != "dave@example.com" || snapshot.To[1].Address != "erin@example.com" {
		t.Fatalf("To addresses = %#v", snapshot.To)
	}
}

func TestApplySetBodyUsesOnlyPrimaryTextBody(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "updated"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryTextPartID).Body); got != "updated" {
		t.Fatalf("text body = %q", got)
	}
}

func TestApplySetBodyUpdatesPairedPlainAndHTMLDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

plain text differs
--alt
Content-Type: text/html; charset=UTF-8

<p>hello</p>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<section>updated <strong>body</strong></section>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body); got != "<section>updated <strong>body</strong></section>" {
		t.Fatalf("html body = %q", got)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryTextPartID).Body); got != "updated body" {
		t.Fatalf("text body = %q", got)
	}
}

func TestApplySetBodyRejectsPlainTextForPairedHTMLDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

summary text
--alt
Content-Type: text/html; charset=UTF-8

<p>hello</p>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "updated plain text"}},
	})
	if err == nil || !strings.Contains(err.Error(), "draft main body is text/html") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplySetBodyUpdatesHTMLDraftWithDerivedPlainFallback(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

hello world
--alt
Content-Type: text/html; charset=UTF-8

<div>hello <b>world</b></div>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<section>updated <strong>body</strong></section>"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body); got != "<section>updated <strong>body</strong></section>" {
		t.Fatalf("html body = %q", got)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryTextPartID).Body); got != "updated body" {
		t.Fatalf("text body = %q", got)
	}
}

func TestApplyRewriteEntireDraftAddsHTMLPartToPlainTextDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 7bit

hello
`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/html", Selector: "primary", Value: "<p>hello</p>"}},
		Options: PatchOptions{
			RewriteEntireDraft: true,
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if snapshot.PrimaryTextPartID == "" || snapshot.PrimaryHTMLPartID == "" {
		t.Fatalf("expected both text and html body parts after rewrite, snapshot=%#v", snapshot)
	}
	if !snapshot.Body.IsMultipart() || snapshot.Body.MediaType != "multipart/alternative" {
		t.Fatalf("body root = %#v", snapshot.Body)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryHTMLPartID).Body); got != "<p>hello</p>" {
		t.Fatalf("html body = %q", got)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryTextPartID).Body); got != "hello\n" {
		t.Fatalf("text body = %q", got)
	}
}

func TestReplaceBodyRejectsPairedPlainAndHTMLDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

plain text differs
--alt
Content-Type: text/html; charset=UTF-8

<p>hello</p>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/html", Selector: "primary", Value: "<div>updated</div>"}},
	})
	if err == nil || !strings.Contains(err.Error(), "edit them together with set_body") {
		t.Fatalf("error = %v", err)
	}
}

func TestAppendBodyRejectsPairedPlainAndHTMLDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8

plain text differs
--alt
Content-Type: text/html; charset=UTF-8

<p>hello</p>
--alt--
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: "\nappend"}},
	})
	if err == nil || !strings.Contains(err.Error(), "edit them together with set_body") {
		t.Fatalf("error = %v", err)
	}
}

func TestApplyRewriteEntireDraftAddsTextPartInsideRelatedDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary=rel

--rel
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 7bit

<div>hello<img src="cid:logo"></div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

aGVsbG8=
--rel--
`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/plain", Selector: "primary", Value: "hello plain"}},
		Options: PatchOptions{
			RewriteEntireDraft: true,
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if snapshot.Body.MediaType != "multipart/related" {
		t.Fatalf("body root = %#v", snapshot.Body)
	}
	if snapshot.PrimaryTextPartID == "" || snapshot.PrimaryHTMLPartID == "" {
		t.Fatalf("expected both body parts, snapshot=%#v", snapshot)
	}
	inline := findPart(snapshot.Body, "1.2")
	if inline == nil || inline.ContentID != "logo" {
		t.Fatalf("inline part not preserved: %#v", inline)
	}
	if got := string(findPart(snapshot.Body, snapshot.PrimaryTextPartID).Body); got != "hello plain" {
		t.Fatalf("text body = %q", got)
	}
}

func TestRemoveAttachmentKeepsRemainingOrder(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/forward_draft.eml"))
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_attachment", Target: AttachmentTarget{PartID: "1.3"}}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if snapshot.Body == nil || len(snapshot.Body.Children) != 2 {
		t.Fatalf("unexpected children after remove: %#v", snapshot.Body)
	}
	remaining := snapshot.Body.Children[1]
	if remaining.FileName() != "one.pdf" {
		t.Fatalf("remaining attachment = %#v", remaining)
	}
}

func TestRemoveInlineByCID(t *testing.T) {
	// Use a draft where HTML does NOT reference the CID, so removal succeeds.
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div>no cid reference</div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo-cid>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_inline", Target: AttachmentTarget{CID: "logo-cid"}}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Body.Children) != 1 {
		t.Fatalf("expected 1 child after remove, got %d", len(snapshot.Body.Children))
	}
}

func TestAddInlineWrapsHTMLBodyIntoRelated(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 7bit

<div>hello<img src="cid:logo" /></div>
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "add_inline", Path: "logo.png", CID: "logo"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if snapshot.Body.MediaType != "multipart/related" {
		t.Fatalf("body root = %#v", snapshot.Body)
	}
	if len(snapshot.Body.Children) != 2 {
		t.Fatalf("children len = %d", len(snapshot.Body.Children))
	}
	inline := snapshot.Body.Children[1]
	if inline.ContentID != "logo" || !isInlinePart(inline) {
		t.Fatalf("inline part = %#v", inline)
	}
}

func TestReplaceInlineKeepsCIDByDefault(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/html_inline_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("updated.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated.png"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	inline := findPart(snapshot.Body, "1.2")
	if inline == nil || inline.ContentID != "logo" {
		t.Fatalf("inline part = %#v", inline)
	}
	if got := inline.Body; len(got) != 8 || got[0] != 0x89 || got[1] != 'P' {
		t.Fatalf("inline body = %q", got)
	}
}

func TestRemoveInlineFailsWhenHTMLStillReferencesCID(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/html_inline_draft.eml"))
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "remove_inline", Target: AttachmentTarget{PartID: "1.2"}},
		},
	})
	if err == nil {
		t.Fatalf("expected remove_inline to fail while HTML still references cid")
	}
}

func TestApplySetBodyOrphanedInlineCIDIsAutoRemoved(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:logo" /></div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// set_body that drops the existing cid:logo reference → logo is auto-removed
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<div>replaced body without cid reference</div>"}},
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	// The orphaned inline part should be removed from the MIME tree.
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "logo" {
			t.Fatal("expected orphaned inline part 'logo' to be removed")
		}
	}
}

func TestApplySetBodyPreservingCIDRefsSucceeds(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=UTF-8

<div>hello<img src="cid:logo" /></div>
--rel
Content-Type: image/png; name=logo.png
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>
Content-Transfer-Encoding: base64

cG5n
--rel--
`)
	// set_body that preserves the existing cid:logo reference → should succeed
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div>updated body<img src="cid:logo" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func TestApplySetBodyRejectsSignedDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/multipart_signed_draft.eml"))
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "updated"}},
	})
	if err == nil {
		t.Fatalf("expected set_body to fail for multipart/signed draft")
	}
}

func TestApplyAppendTextKeepsCalendarPart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/calendar_draft.eml"))
	calendar := findPart(snapshot.Body, "1.2")
	if calendar == nil {
		t.Fatalf("calendar part missing before patch")
	}
	originalCalendar := string(calendar.RawEntity)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: "\nupdated"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	calendar = findPart(snapshot.Body, "1.2")
	if calendar == nil || string(calendar.RawEntity) != originalCalendar {
		t.Fatalf("calendar part changed unexpectedly: %#v", calendar)
	}
}

func TestAddAttachmentUsesBackendCompatibleContentType(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("note.txt", []byte("hello attachment\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "add_attachment", Path: "note.txt"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	attachment := findPart(snapshot.Body, "1.2")
	if attachment == nil {
		t.Fatalf("attachment missing after add")
	}
	if attachment.FileName() != "note.txt" {
		t.Fatalf("attachment filename = %q", attachment.FileName())
	}
	if attachment.MediaType != "application/octet-stream" {
		t.Fatalf("attachment media type = %q", attachment.MediaType)
	}
	if got := headerValue(attachment.Headers, "Content-Type"); got == "" || !strings.Contains(got, "application/octet-stream") || !strings.Contains(got, "name=") {
		t.Fatalf("attachment Content-Type header = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Header injection rejection tests (CID / fileName CR/LF)
// ---------------------------------------------------------------------------

func TestAddInlineRejectsInvalidCharactersInCID(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello</div>
`)
	for _, bad := range []string{"my logo", "cid\there", "lo<go>id", "img(1)"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "add_inline", Path: "logo.png", CID: bad}},
		})
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		} else if !strings.Contains(err.Error(), "invalid characters") {
			t.Errorf("CID %q: expected 'invalid characters' error, got: %v", bad, err)
		}
	}
}

func TestValidateInlineCIDAfterSetBody(t *testing.T) {
	// Verify that inline CID validation works even after set_body restructures
	// the MIME tree (which can change PartIDs). This tests the fix that uses
	// findPrimaryBodyPart (by media type) instead of findPart (by stale PartID).
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Inline
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div><img src="cid:logo" /></div>
`)
	// Step 1: add inline — this wraps body into multipart/related
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "add_inline", Path: "logo.png", CID: "logo"}},
	})
	if err != nil {
		t.Fatalf("Apply(add_inline) error = %v", err)
	}

	// Step 2: set_body — this restructures the MIME tree, potentially making
	// PrimaryHTMLPartID stale
	err = Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div>updated<img src="cid:logo" /></div>`}},
	})
	if err != nil {
		t.Fatalf("Apply(set_body) error = %v", err)
	}

	// Step 3: set_body again dropping the CID reference — orphaned inline part
	// should be auto-removed (not error), matching the auto-cleanup behavior.
	err = Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: `<div>no image here</div>`}},
	})
	if err != nil {
		t.Fatalf("Apply(set_body drop CID) error = %v", err)
	}
	for _, part := range flattenParts(snapshot.Body) {
		if part != nil && part.ContentID == "logo" {
			t.Fatal("expected orphaned inline part 'logo' to be auto-removed")
		}
	}
}

func TestAddInlineRejectsCRLFInCID(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello</div>
`)
	for _, bad := range []string{"logo\ninjected", "logo\rinjected", "lo\r\ngo"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "add_inline", Path: "logo.png", CID: bad}},
		})
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		}
	}
}

func TestAddInlineRejectsCRLFInFileName(t *testing.T) {
	chdirTemp(t)
	if err := os.WriteFile("logo.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<div>hello</div>
`)
	for _, bad := range []string{"logo\ninjected.png", "logo\r.png", "lo\r\ngo.png"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "add_inline", Path: "logo.png", CID: "safecid", FileName: bad}},
		})
		if err == nil {
			t.Errorf("expected error for filename %q, got nil", bad)
		}
	}
}

func TestReplaceInlineRejectsInvalidCharactersInCID(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/html_inline_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("updated.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	for _, bad := range []string{"my logo", "cid\there", "lo<go>id", "img(1)"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated.png", CID: bad}},
		})
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		} else if !strings.Contains(err.Error(), "invalid characters") {
			t.Errorf("CID %q: expected 'invalid characters' error, got: %v", bad, err)
		}
	}
}

func TestReplaceInlineRejectsCRLFInCID(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/html_inline_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("updated.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	for _, bad := range []string{"logo\ninjected", "logo\rinjected"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated.png", CID: bad}},
		})
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		}
	}
}

func TestReplaceInlineRejectsInvalidCIDChars(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/html_inline_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("updated.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	for _, bad := range []string{"my logo", "a\tb", "cid<x>", "cid(x)"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated.png", CID: bad}},
		})
		if err == nil {
			t.Errorf("expected error for CID %q, got nil", bad)
		}
	}
}

func TestReplaceInlineRejectsCRLFInFileName(t *testing.T) {
	fixtureData := mustReadFixture(t, "testdata/html_inline_draft.eml")
	chdirTemp(t)
	if err := os.WriteFile("updated.png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	snapshot := mustParseFixtureDraft(t, fixtureData)
	for _, bad := range []string{"logo\ninjected.png", "logo\r.png"} {
		err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
			Ops: []PatchOp{{Op: "replace_inline", Target: AttachmentTarget{PartID: "1.2"}, Path: "updated.png", FileName: bad}},
		})
		if err == nil {
			t.Errorf("expected error for filename %q, got nil", bad)
		}
	}
}

func mustParseFixtureDraft(t *testing.T, raw string) *DraftSnapshot {
	t.Helper()
	snapshot, err := Parse(DraftRaw{DraftID: "d-1", RawEML: encodeFixtureEML(raw)})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return snapshot
}

// ---------------------------------------------------------------------------
// MustJSON — panic on unmarshalable input
// ---------------------------------------------------------------------------

func TestMustJSON_Valid(t *testing.T) {
	got := MustJSON(map[string]string{"key": "value"})
	if got != `{"key":"value"}` {
		t.Errorf("MustJSON = %q", got)
	}
}

func TestMustJSON_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected MustJSON to panic on unmarshalable value")
		}
	}()
	// func values cannot be marshaled to JSON
	MustJSON(func() {})
}
