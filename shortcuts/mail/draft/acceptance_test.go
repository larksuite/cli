// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import "testing"

func TestAcceptanceReplyDraftSubjectOnly(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/reply_draft_with_inline_attachment.eml"))
	originalBodyEntity := string(snapshot.Body.RawEntity)
	originalInline := findPart(snapshot.Body, "1.2")
	originalAttachment := findPart(snapshot.Body, "1.3")

	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Reply updated"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-reply", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}

	if roundTrip.InReplyTo != "<orig-1@example.com>" {
		t.Fatalf("InReplyTo = %q", roundTrip.InReplyTo)
	}
	if roundTrip.References != "<root@example.com> <orig-1@example.com>" {
		t.Fatalf("References = %q", roundTrip.References)
	}
	if got := string(roundTrip.Body.RawEntity); got != originalBodyEntity {
		t.Fatalf("body entity changed unexpectedly\nwant:\n%s\n\ngot:\n%s", originalBodyEntity, got)
	}
	inline := findPart(roundTrip.Body, "1.2")
	if inline == nil || inline.ContentID != originalInline.ContentID {
		t.Fatalf("inline part mismatch: %#v", inline)
	}
	attachment := findPart(roundTrip.Body, "1.3")
	if attachment == nil || attachment.FileName() != originalAttachment.FileName() {
		t.Fatalf("attachment mismatch: %#v", attachment)
	}
}

func TestAcceptanceHTMLInlineReplaceHTML(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/html_inline_draft.eml"))
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "replace_body", BodyKind: "text/html", Selector: "primary", Value: `<div>updated<img src="cid:logo"></div>`}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-inline", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryHTMLPartID).Body); got != `<div>updated<img src="cid:logo"></div>` {
		t.Fatalf("HTML body = %q", got)
	}
	inline := findPart(roundTrip.Body, "1.2")
	if inline == nil || inline.ContentID != "logo" {
		t.Fatalf("inline part mismatch: %#v", inline)
	}
}

func TestAcceptanceAlternativeSetBodyUpdatesHTMLAndSummary(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/alternative_draft.eml"))
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_body", Value: "<div>updated <strong>body</strong></div>"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-alt", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryTextPartID).Body); got != "updated body" {
		t.Fatalf("text body = %q", got)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryHTMLPartID).Body); got != "<div>updated <strong>body</strong></div>" {
		t.Fatalf("HTML body = %q", got)
	}
}

func TestAcceptanceCalendarDraftAppendPlainPreservesCalendar(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/calendar_draft.eml"))
	originalCalendar := findPart(snapshot.Body, "1.2")
	if originalCalendar == nil {
		t.Fatalf("calendar part missing")
	}
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: "\nagenda"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-cal", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryTextPartID).Body); got != "Team sync invite\nagenda" {
		t.Fatalf("text body = %q", got)
	}
	calendar := findPart(roundTrip.Body, "1.2")
	if calendar == nil || string(calendar.Body) != string(originalCalendar.Body) {
		t.Fatalf("calendar part changed unexpectedly: %#v", calendar)
	}
}

func TestAcceptanceSignedDraftSubjectOnlyPreservesSignedEntity(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/multipart_signed_draft.eml"))
	originalBodyEntity := string(snapshot.Body.RawEntity)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Signed updated"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-signed", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(roundTrip.Body.RawEntity); got != originalBodyEntity {
		t.Fatalf("signed body entity changed unexpectedly\nwant:\n%s\n\ngot:\n%s", originalBodyEntity, got)
	}
}

func TestAcceptanceDirtyMultipartAppendPlainPreservesOuterNoise(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, mustReadFixture(t, "testdata/dirty_multipart_preamble.eml"))
	originalPreamble := string(snapshot.Body.Preamble)
	originalEpilogue := string(snapshot.Body.Epilogue)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: "\nworld"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	raw, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-dirty", RawEML: raw})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(roundTrip.Body.Preamble); got != originalPreamble {
		t.Fatalf("preamble changed unexpectedly\nwant:\n%s\n\ngot:\n%s", originalPreamble, got)
	}
	if got := string(roundTrip.Body.Epilogue); got != originalEpilogue {
		t.Fatalf("epilogue changed unexpectedly\nwant:\n%s\n\ngot:\n%s", originalEpilogue, got)
	}
}
