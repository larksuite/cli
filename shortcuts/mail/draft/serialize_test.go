// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

func TestSerializeRoundTripKeepsAttachmentsAndHTML(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Bcc: Hidden <hidden@example.com>
In-Reply-To: <orig-1@example.com>
References: <root@example.com> <orig-1@example.com>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary=mix

--mix
Content-Type: multipart/alternative; boundary=alt

--alt
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 7bit

hello
--alt
Content-Type: text/html; charset=UTF-8
Content-Transfer-Encoding: 7bit

<p>hello</p>
--alt--
--mix
Content-Type: application/pdf; name=report.pdf
Content-Disposition: attachment; filename=report.pdf
Content-Transfer-Encoding: base64

aGVsbG8=
--mix--
`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{
			{Op: "set_subject", Value: "Updated"},
			{Op: "set_body", Value: "<div>updated <strong>body</strong></div>"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-1", RawEML: serialized})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if roundTrip.Subject != "Updated" {
		t.Fatalf("Subject = %q", roundTrip.Subject)
	}
	if roundTrip.InReplyTo != "<orig-1@example.com>" {
		t.Fatalf("InReplyTo = %q", roundTrip.InReplyTo)
	}
	if roundTrip.References != "<root@example.com> <orig-1@example.com>" {
		t.Fatalf("References = %q", roundTrip.References)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryHTMLPartID).Body); got != "<div>updated <strong>body</strong></div>" {
		t.Fatalf("HTML body = %q", got)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryTextPartID).Body); got != "updated body" {
		t.Fatalf("text body = %q", got)
	}
	if attachment := findPart(roundTrip.Body, "1.2"); attachment == nil || attachment.FileName() != "report.pdf" {
		t.Fatalf("attachment not preserved: %#v", attachment)
	}
	if got := headerValue(roundTrip.Headers, "Bcc"); got == "" {
		t.Fatalf("Bcc header unexpectedly dropped")
	}
}

func TestSerializeSubjectOnlyPreservesOriginalBodyEntity(t *testing.T) {
	original := `Subject: Original
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Message-ID: <draft-1@example.com>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary=mix

--mix
Content-Type: text/plain; charset=UTF-8
Content-Transfer-Encoding: 7bit

hello
--mix
Content-Type: image/png; name=logo.png
Content-Transfer-Encoding: base64
Content-Disposition: inline; filename=logo.png
Content-ID: <logo>

aGVsbG8=
--mix--
`
	snapshot := mustParseFixtureDraft(t, original)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{Ops: []PatchOp{{Op: "set_subject", Value: "Updated"}}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	decoded, err := decodeRawEML(serialized)
	if err != nil {
		t.Fatalf("decodeRawEML() error = %v", err)
	}
	got := string(decoded)
	wantIdx := strings.Index(original, "Content-Type: multipart/mixed; boundary=mix")
	if wantIdx < 0 {
		t.Fatal("expected Content-Type multipart/mixed not found in original")
	}
	gotIdx := strings.Index(got, "Content-Type: multipart/mixed; boundary=mix")
	if gotIdx < 0 {
		t.Fatal("expected Content-Type multipart/mixed not found in serialized output")
	}
	wantBodyEntity := original[wantIdx:]
	gotBodyEntity := got[gotIdx:]
	if gotBodyEntity != wantBodyEntity {
		t.Fatalf("body entity changed unexpectedly\nwant:\n%s\n\ngot:\n%s", wantBodyEntity, gotBodyEntity)
	}
}

func TestSerializeEditedQuotedPrintableTextPreservesReadableTextSemantics(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Encoded body
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=ISO-8859-1
Content-Transfer-Encoding: quoted-printable

caf=E9
`)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: " déjà"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	decoded, err := decodeRawEML(serialized)
	if err != nil {
		t.Fatalf("decodeRawEML() error = %v", err)
	}
	raw := string(decoded)
	if !strings.Contains(strings.ToLower(raw), "content-transfer-encoding: quoted-printable") {
		t.Fatalf("serialized raw missing quoted-printable:\n%s", raw)
	}
	if !strings.Contains(strings.ToLower(raw), "charset=iso-8859-1") {
		t.Fatalf("serialized raw missing original charset:\n%s", raw)
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-qp", RawEML: serialized})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryTextPartID).Body); got != "café\n déjà\n" {
		t.Fatalf("round-trip text body = %q", got)
	}
}

func TestSerializeSubjectOnlyPreservesEmbeddedMessageAttachment(t *testing.T) {
	original := mustReadFixture(t, "testdata/message_rfc822_draft.eml")
	snapshot := mustParseFixtureDraft(t, original)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{Ops: []PatchOp{{Op: "set_subject", Value: "Updated forward"}}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	decoded, err := decodeRawEML(serialized)
	if err != nil {
		t.Fatalf("decodeRawEML() error = %v", err)
	}
	got := string(decoded)
	if !strings.Contains(got, "Content-Type: message/rfc822; name=forwarded.eml") {
		t.Fatalf("embedded message attachment missing:\n%s", got)
	}
	if !strings.Contains(got, "Subject: Inner message") {
		t.Fatalf("embedded message payload changed unexpectedly:\n%s", got)
	}
}

func TestSerializeSubjectOnlyPreservesSignedBodyEntity(t *testing.T) {
	original := mustReadFixture(t, "testdata/multipart_signed_draft.eml")
	snapshot := mustParseFixtureDraft(t, original)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{Ops: []PatchOp{{Op: "set_subject", Value: "Updated signed"}}}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	decoded, err := decodeRawEML(serialized)
	if err != nil {
		t.Fatalf("decodeRawEML() error = %v", err)
	}
	got := string(decoded)
	wantIdx := strings.Index(original, "Content-Type: multipart/signed")
	if wantIdx < 0 {
		t.Fatal("expected Content-Type multipart/signed not found in original")
	}
	gotIdx := strings.Index(got, "Content-Type: multipart/signed")
	if gotIdx < 0 {
		t.Fatal("expected Content-Type multipart/signed not found in serialized output")
	}
	wantBodyEntity := original[wantIdx:]
	gotBodyEntity := got[gotIdx:]
	if gotBodyEntity != wantBodyEntity {
		t.Fatalf("signed body entity changed unexpectedly\nwant:\n%s\n\ngot:\n%s", wantBodyEntity, gotBodyEntity)
	}
}

func TestSerializeDirtyMultipartKeepsPreambleAndEpilogue(t *testing.T) {
	original := mustReadFixture(t, "testdata/dirty_multipart_preamble.eml")
	snapshot := mustParseFixtureDraft(t, original)
	if err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "append_body", BodyKind: "text/plain", Selector: "primary", Value: "\nworld"}},
	}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	serialized, err := Serialize(snapshot)
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	decoded, err := decodeRawEML(serialized)
	if err != nil {
		t.Fatalf("decodeRawEML() error = %v", err)
	}
	got := string(decoded)
	for _, want := range []string{
		"This is a preamble line.\nStill preamble.\n",
		"--mix--\nThis is an epilogue line.\nTrailing dirty text.\n",
		"cOnTeNt-TyPe: multipart/mixed; boundary=mix",
		"Subject: Dirty multipart",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("serialized multipart missing %q\n%s", want, got)
		}
	}
	roundTrip, err := Parse(DraftRaw{DraftID: "d-dirty", RawEML: serialized})
	if err != nil {
		t.Fatalf("Parse(roundTrip) error = %v", err)
	}
	if got := string(findPart(roundTrip.Body, roundTrip.PrimaryTextPartID).Body); got != "hello\nworld" {
		t.Fatalf("round-trip text body = %q", got)
	}
}
