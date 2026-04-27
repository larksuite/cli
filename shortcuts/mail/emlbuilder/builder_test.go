// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package emlbuilder

import (
	"encoding/base64"
	"net/mail"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/vfs/localfileio"
)

var (
	fixedDate = time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	testFIO   = &localfileio.LocalFileIO{}
)

// parseEML splits an EML string into a header block and body.
func splitHeaderBody(eml string) (headers, body string) {
	idx := strings.Index(eml, "\n\n")
	if idx == -1 {
		return eml, ""
	}
	return eml[:idx], eml[idx+2:]
}

func headerValue(eml, name string) string {
	for _, line := range strings.Split(eml, "\n") {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(name)+":") {
			return strings.TrimSpace(line[len(name)+1:])
		}
	}
	return ""
}

// ── validation ────────────────────────────────────────────────────────────────

// TestBuild_MissingFrom verifies build missing from.
func TestBuild_MissingFrom(t *testing.T) {
	_, err := New().To("", "bob@example.com").Subject("hi").Build()
	if err == nil || !strings.Contains(err.Error(), "From") {
		t.Fatalf("expected From error, got %v", err)
	}
}

// TestBuild_MissingRecipient verifies build missing recipient.
func TestBuild_MissingRecipient(t *testing.T) {
	_, err := New().From("", "alice@example.com").Subject("hi").Build()
	if err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("expected recipient error, got %v", err)
	}
}

// ── single text/plain ─────────────────────────────────────────────────────────

// TestBuild_SingleTextPlain_ASCII verifies build single text plain ASCII.
func TestBuild_SingleTextPlain_ASCII(t *testing.T) {
	raw, err := New().
		From("Alice", "alice@example.com").
		To("Bob", "bob@example.com").
		Subject("Hello").
		Date(fixedDate).
		MessageID("test-id@lark-cli").
		TextBody([]byte("Hello world")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	// must use LF, not CRLF
	if strings.Contains(eml, "\r\n") {
		t.Error("EML must use LF line endings, not CRLF")
	}

	// required headers
	for _, h := range []string{"Subject: Hello", "From:", "MIME-Version: 1.0", "Message-ID:"} {
		if !strings.Contains(eml, h) {
			t.Errorf("missing header %q in:\n%s", h, eml)
		}
	}

	// content type must not be folded (all params on one line)
	for _, line := range strings.Split(eml, "\n") {
		if strings.Contains(line, "Content-Type:") && strings.Contains(line, "boundary=") {
			if !strings.Contains(line, "boundary=") {
				t.Errorf("Content-Type with boundary must be on a single line: %q", line)
			}
		}
	}

	// 7bit CTE for ASCII
	if !strings.Contains(eml, "Content-Transfer-Encoding: 7bit") {
		t.Errorf("expected 7bit CTE for ASCII body, got:\n%s", eml)
	}
	if !strings.Contains(eml, "Hello world") {
		t.Error("body text missing")
	}
}

// TestBuild_SingleTextPlain_NonASCII verifies build single text plain non ASCII.
func TestBuild_SingleTextPlain_NonASCII(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("你好").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("你好世界")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	// subject must be RFC 2047 encoded
	subj := headerValue(eml, "Subject")
	if subj == "你好" {
		t.Error("non-ASCII subject must be RFC 2047 encoded")
	}
	if !strings.HasPrefix(subj, "=?utf-8?") && !strings.HasPrefix(subj, "=?UTF-8?") {
		t.Errorf("unexpected subject encoding: %q", subj)
	}

	// body must be base64
	if !strings.Contains(eml, "Content-Transfer-Encoding: base64") {
		t.Errorf("expected base64 CTE for non-ASCII body:\n%s", eml)
	}

	// body content must be valid base64 of the original text
	headers, body := splitHeaderBody(eml)
	_ = headers
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body))
	if err != nil {
		t.Fatalf("body is not valid base64: %v\nbody=%q", err, body)
	}
	if string(decoded) != "你好世界" {
		t.Errorf("decoded body mismatch: got %q", decoded)
	}
}

// ── multipart/alternative ─────────────────────────────────────────────────────

// TestBuild_MultipartAlternative verifies build multipart alternative.
func TestBuild_MultipartAlternative(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("plain")).
		HTMLBody([]byte("<p>html</p>")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "multipart/alternative") {
		t.Error("expected multipart/alternative")
	}
	// boundary must be on the same line as Content-Type
	for _, line := range strings.Split(eml, "\n") {
		if strings.HasPrefix(line, "Content-Type: multipart/") {
			if !strings.Contains(line, "boundary=") {
				t.Errorf("Content-Type line missing boundary param: %q", line)
			}
		}
	}
	if !strings.Contains(eml, "text/plain") {
		t.Error("missing text/plain part")
	}
	if !strings.Contains(eml, "text/html") {
		t.Error("missing text/html part")
	}
}

// ── multipart/mixed (with attachments) ───────────────────────────────────────

// TestBuild_WithAttachment verifies build with attachment.
func TestBuild_WithAttachment(t *testing.T) {
	attContent := []byte("PDF content here")
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("with attachment").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("see attached")).
		AddAttachment(attContent, "application/pdf", "doc.pdf").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "multipart/mixed") {
		t.Error("expected multipart/mixed for message with attachments")
	}
	if !strings.Contains(eml, `Content-Disposition: attachment; filename="doc.pdf"`) {
		t.Errorf("missing attachment disposition:\n%s", eml)
	}

	// attachment body must be base64 of attContent
	expectedB64 := base64.StdEncoding.EncodeToString(attContent)
	if !strings.Contains(eml, expectedB64) {
		t.Errorf("attachment base64 not found in EML:\n%s", eml)
	}
}

// ── reply threading headers ───────────────────────────────────────────────────

// TestBuild_ReplyHeaders verifies build reply headers.
func TestBuild_ReplyHeaders(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("Re: hello").
		Date(fixedDate).
		MessageID("reply@x").
		InReplyTo("original@smtp").
		References("<original@smtp>").
		TextBody([]byte("my reply")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	inReplyTo := headerValue(eml, "In-Reply-To")
	if inReplyTo != "<original@smtp>" {
		t.Errorf("In-Reply-To: got %q, want <original@smtp>", inReplyTo)
	}
	refs := headerValue(eml, "References")
	if refs != "<original@smtp>" {
		t.Errorf("References: got %q, want <original@smtp>", refs)
	}
}

// TestBuild_LMSReplyToMessageID verifies build LMS reply to message ID.
func TestBuild_LMSReplyToMessageID(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("Re: hello").
		Date(fixedDate).
		InReplyTo("original@smtp").
		LMSReplyToMessageID("740000000000000067").
		TextBody([]byte("my reply")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	got := headerValue(eml, "X-LMS-Reply-To-Message-Id")
	if got != "740000000000000067" {
		t.Errorf("X-LMS-Reply-To-Message-Id: got %q, want 740000000000000067", got)
	}
}

// TestBuild_LMSReplyToMessageID_NotWrittenWithoutInReplyTo verifies build LMS reply to message ID not written without in reply to.
func TestBuild_LMSReplyToMessageID_NotWrittenWithoutInReplyTo(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hello").
		Date(fixedDate).
		LMSReplyToMessageID("740000000000000067").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	got := headerValue(eml, "X-LMS-Reply-To-Message-Id")
	if got != "" {
		t.Errorf("X-LMS-Reply-To-Message-Id should be absent when In-Reply-To is not set, got %q", got)
	}
}

// ── Disposition-Notification-To (read receipt) ───────────────────────────────

// TestBuild_DispositionNotificationTo verifies build disposition notification to.
func TestBuild_DispositionNotificationTo(t *testing.T) {
	raw, err := New().
		From("Alice", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("dnt@x").
		DispositionNotificationTo("Alice", "alice@example.com").
		TextBody([]byte("please ack")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	got := headerValue(string(raw), "Disposition-Notification-To")
	want := `"Alice" <alice@example.com>`
	if got != want {
		t.Errorf("Disposition-Notification-To: got %q, want %q", got, want)
	}
}

// TestBuild_DispositionNotificationTo_MultipleAddresses verifies build disposition notification to multiple addresses.
func TestBuild_DispositionNotificationTo_MultipleAddresses(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("dnt-multi@x").
		DispositionNotificationTo("", "alice@example.com").
		DispositionNotificationTo("", "carol@example.com").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	got := headerValue(string(raw), "Disposition-Notification-To")
	want := "<alice@example.com>, <carol@example.com>"
	if got != want {
		t.Errorf("Disposition-Notification-To: got %q, want %q", got, want)
	}
}

// TestBuild_DispositionNotificationTo_NotWrittenWhenUnset verifies build disposition notification to not written when unset.
func TestBuild_DispositionNotificationTo_NotWrittenWhenUnset(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("no-dnt@x").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := headerValue(string(raw), "Disposition-Notification-To"); got != "" {
		t.Errorf("Disposition-Notification-To should be absent when unset, got %q", got)
	}
}

// TestBuild_DispositionNotificationTo_EmptyAddressIgnored verifies build disposition notification to empty address ignored.
func TestBuild_DispositionNotificationTo_EmptyAddressIgnored(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("empty-dnt@x").
		DispositionNotificationTo("", "").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := headerValue(string(raw), "Disposition-Notification-To"); got != "" {
		t.Errorf("empty address should be ignored; got header %q", got)
	}
}

// TestBuild_DispositionNotificationTo_CRLFRejected verifies build disposition notification to CR LF rejected.
func TestBuild_DispositionNotificationTo_CRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		DispositionNotificationTo("Alice\r\nBcc: evil@evil.com", "alice@example.com").
		TextBody([]byte("body")).
		Build()
	if err == nil || !strings.Contains(err.Error(), "display name") {
		t.Fatalf("expected display-name CRLF error, got %v", err)
	}
}

// TestBuild_DispositionNotificationTo_AddrCRLFRejected verifies build disposition notification to addr CR LF rejected.
func TestBuild_DispositionNotificationTo_AddrCRLFRejected(t *testing.T) {
	// Injection via the address (not just the display name) must be blocked.
	// A plain mail.Address.String() would emit "<alice@x.com\r\nX-Injected: 1>"
	// unchanged, allowing the attacker to inject new headers.
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		DispositionNotificationTo("Alice", "alice@example.com\r\nX-Injected: pwned").
		TextBody([]byte("body")).
		Build()
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected addr CRLF error, got %v", err)
	}
}

// TestBuild_DispositionNotificationTo_AddrBidiRejected verifies build disposition notification to addr bidi rejected.
func TestBuild_DispositionNotificationTo_AddrBidiRejected(t *testing.T) {
	// Bidi overrides (U+202E RLO) enable visual spoofing (e.g. "gmail" + RLO + "com.evil.com"
	// renders as gmail.com at the tail); they must be blocked in the addr
	// too, not only in header names / display names.
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		DispositionNotificationTo("Alice", "alice@gma\u202eil.com").
		TextBody([]byte("body")).
		Build()
	if err == nil || !strings.Contains(err.Error(), "dangerous Unicode") {
		t.Fatalf("expected addr dangerous-Unicode error, got %v", err)
	}
}

// ── X-Lark-Read-Receipt-Mail (read receipt response marker) ──────────────────

// TestBuild_IsReadReceiptMail_True verifies build is read receipt mail true.
func TestBuild_IsReadReceiptMail_True(t *testing.T) {
	raw, err := New().
		From("", "bob@example.com").
		To("", "alice@example.com").
		Subject("已读回执：hi").
		Date(fixedDate).
		MessageID("irrm@x").
		IsReadReceiptMail(true).
		TextBody([]byte("read")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	got := headerValue(string(raw), "X-Lark-Read-Receipt-Mail")
	if got != "1" {
		t.Errorf("X-Lark-Read-Receipt-Mail: got %q, want 1", got)
	}
}

// TestBuild_IsReadReceiptMail_DefaultAbsent verifies build is read receipt mail default absent.
func TestBuild_IsReadReceiptMail_DefaultAbsent(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("no-irrm@x").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := headerValue(string(raw), "X-Lark-Read-Receipt-Mail"); got != "" {
		t.Errorf("X-Lark-Read-Receipt-Mail should be absent by default, got %q", got)
	}
}

// TestBuild_IsReadReceiptMail_ExplicitFalse verifies build is read receipt mail explicit false.
func TestBuild_IsReadReceiptMail_ExplicitFalse(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("hi").
		Date(fixedDate).
		MessageID("irrm-false@x").
		IsReadReceiptMail(false).
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if got := headerValue(string(raw), "X-Lark-Read-Receipt-Mail"); got != "" {
		t.Errorf("X-Lark-Read-Receipt-Mail should be absent when set false, got %q", got)
	}
}

// TestBuild_DispositionNotificationTo_PreservesPriorError verifies that once
// the Builder carries an error from a prior setter, DispositionNotificationTo
// short-circuits and does NOT clobber the existing error with a nil.
func TestBuild_DispositionNotificationTo_PreservesPriorError(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("bad\r\nheader"). // injects err
		DispositionNotificationTo("Alice", "alice@example.com").
		Date(fixedDate).
		TextBody([]byte("body")).
		Build()
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected original Subject CRLF error to survive DispositionNotificationTo, got %v", err)
	}
}

// TestBuild_IsReadReceiptMail_PreservesPriorError verifies that once the
// Builder carries an error from a prior setter, IsReadReceiptMail short-
// circuits and does NOT clobber the existing error with a nil.
func TestBuild_IsReadReceiptMail_PreservesPriorError(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("bad\r\nheader"). // injects err
		IsReadReceiptMail(true).
		Date(fixedDate).
		TextBody([]byte("body")).
		Build()
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected original Subject CRLF error to survive IsReadReceiptMail, got %v", err)
	}
}

// ── CC / BCC ──────────────────────────────────────────────────────────────────

// TestBuild_CCBCC verifies build c c b c c.
func TestBuild_CCBCC(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		CC("", "charlie@example.com").
		BCC("", "dave@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("body")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "charlie@example.com") {
		t.Errorf("missing Cc address:\n%s", eml)
	}
	if !strings.Contains(eml, "Cc:") {
		t.Errorf("missing Cc header:\n%s", eml)
	}
	if !strings.Contains(eml, "Bcc:") {
		t.Errorf("missing Bcc header:\n%s", eml)
	}
	if !strings.Contains(eml, "dave@example.com") {
		t.Errorf("missing Bcc address:\n%s", eml)
	}
}

// TestAllRecipients verifies all recipients.
func TestAllRecipients(t *testing.T) {
	b := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		CC("", "charlie@example.com").
		BCC("", "dave@example.com")
	recips := b.AllRecipients()
	if len(recips) != 3 {
		t.Fatalf("expected 3 recipients, got %d: %v", len(recips), recips)
	}
}

// ── BuildBase64URL ────────────────────────────────────────────────────────────

// TestBuildBase64URL verifies build base64 URL.
func TestBuildBase64URL(t *testing.T) {
	encoded, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("base64url test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hello")).
		BuildBase64URL()
	if err != nil {
		t.Fatal(err)
	}

	// must be valid base64url
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("BuildBase64URL produced invalid base64url: %v", err)
	}

	// decoded must be valid EML
	if !strings.Contains(string(decoded), "Subject: base64url test") {
		t.Errorf("decoded EML missing expected content:\n%s", decoded)
	}

	// must NOT contain standard base64 chars that differ from base64url
	// ('+' → '-', '/' → '_')
	if strings.ContainsAny(encoded, "+/") {
		t.Error("BuildBase64URL must use base64url encoding (- and _ instead of + and /)")
	}
}

// ── immutability ──────────────────────────────────────────────────────────────

// TestBuilder_Immutability verifies builder immutability.
func TestBuilder_Immutability(t *testing.T) {
	base := New().From("", "alice@example.com").Subject("base")
	b1 := base.To("", "bob@example.com")
	b2 := base.To("", "charlie@example.com")

	if len(b1.to) != 1 || b1.to[0].Address != "bob@example.com" {
		t.Errorf("b1 unexpected to: %v", b1.to)
	}
	if len(b2.to) != 1 || b2.to[0].Address != "charlie@example.com" {
		t.Errorf("b2 unexpected to: %v", b2.to)
	}
	// base should have no To
	if len(base.to) != 0 {
		t.Errorf("base was mutated: to=%v", base.to)
	}
}

// ── ToAddrs / CCAddrs ─────────────────────────────────────────────────────────

// TestBuild_ToAddrs verifies build to addrs.
func TestBuild_ToAddrs(t *testing.T) {
	addrs := []mail.Address{
		{Name: "Bob", Address: "bob@example.com"},
		{Name: "Carol", Address: "carol@example.com"},
	}
	raw, err := New().
		From("", "alice@example.com").
		ToAddrs(addrs).
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)
	if !strings.Contains(eml, "bob@example.com") || !strings.Contains(eml, "carol@example.com") {
		t.Errorf("expected both recipients in EML:\n%s", eml)
	}
}

// ── CalendarBody ──────────────────────────────────────────────────────────────

// TestBuild_CalendarBody_Single verifies build calendar body single.
func TestBuild_CalendarBody_Single(t *testing.T) {
	calData := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR")
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("Meeting").
		Date(fixedDate).
		MessageID("test@x").
		CalendarBody(calData).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "text/calendar") {
		t.Errorf("expected text/calendar in EML:\n%s", eml)
	}
	if strings.Contains(eml, "multipart") {
		t.Errorf("single calendar body should not produce multipart:\n%s", eml)
	}
}

// TestBuild_CalendarWithText verifies build calendar with text.
func TestBuild_CalendarWithText(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("Meeting").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("You are invited.")).
		CalendarBody([]byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR")).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	// text/calendar lives inside multipart/alternative as an alternative
	// representation of the body, matching Feishu client behavior.
	if !strings.Contains(eml, "multipart/alternative") {
		t.Errorf("expected multipart/alternative for text+calendar:\n%s", eml)
	}
	if !strings.Contains(eml, "text/plain") {
		t.Errorf("missing text/plain part:\n%s", eml)
	}
	if !strings.Contains(eml, "text/calendar") {
		t.Errorf("missing text/calendar part:\n%s", eml)
	}
}

// ── AddInline / multipart/related ────────────────────────────────────────────

// TestBuild_WithInline verifies build with inline.
func TestBuild_WithInline(t *testing.T) {
	imgBytes := []byte("\x89PNG\r\n\x1a\n") // minimal PNG magic bytes
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("inline image").
		Date(fixedDate).
		MessageID("test@x").
		HTMLBody([]byte(`<img src="cid:logo"/>`)).
		AddInline(imgBytes, "image/png", "logo.png", "logo").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "multipart/related") {
		t.Errorf("expected multipart/related when inlines present:\n%s", eml)
	}
	if !strings.Contains(eml, "Content-Id: <logo>") {
		t.Errorf("missing Content-Id header:\n%s", eml)
	}
	if !strings.Contains(eml, "Content-Disposition: inline") {
		t.Errorf("missing Content-Disposition: inline:\n%s", eml)
	}
	if !strings.Contains(eml, `Content-Disposition: inline; filename="logo.png"`) {
		t.Errorf("missing quoted inline filename:\n%s", eml)
	}
	if !strings.Contains(eml, "X-Attachment-Id: logo") {
		t.Errorf("missing X-Attachment-Id:\n%s", eml)
	}
	if !strings.Contains(eml, "X-Image-Id: logo") {
		t.Errorf("missing X-Image-Id:\n%s", eml)
	}
	if !strings.Contains(eml, "image/png") {
		t.Errorf("missing image/png Content-Type:\n%s", eml)
	}
}

// TestBuild_WithOtherPart verifies build with other part.
func TestBuild_WithOtherPart(t *testing.T) {
	calData := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR")
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("other part").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("see embedded")).
		AddOtherPart(calData, "text/calendar", "invite.ics", "cal001").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "multipart/related") {
		t.Errorf("expected multipart/related for other parts:\n%s", eml)
	}
	if !strings.Contains(eml, "Content-Id: <cal001>") {
		t.Errorf("missing Content-ID:\n%s", eml)
	}
	// AddOtherPart must NOT write Content-Disposition
	if strings.Contains(eml, "Content-Disposition") {
		t.Errorf("AddOtherPart must not include Content-Disposition:\n%s", eml)
	}
}

// TestBuild_FoldBodyLines_Base64 verifies build fold body lines base64.
func TestBuild_FoldBodyLines_Base64(t *testing.T) {
	body := strings.Repeat("你", 120)
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("fold html").
		Date(fixedDate).
		MessageID("test@x").
		HTMLBody([]byte(body)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	headers, bodyPart := splitHeaderBody(eml)
	_ = headers
	lines := strings.Split(strings.TrimSpace(bodyPart), "\n")
	for i, line := range lines {
		if len(line) > 76 {
			t.Fatalf("base64 line %d too long: %d", i, len(line))
		}
	}
}

// TestBuild_FoldBodyLines_7bit verifies build fold body lines 7bit.
func TestBuild_FoldBodyLines_7bit(t *testing.T) {
	body := strings.Repeat("A", 200)
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("fold text").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte(body)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	headers, bodyPart := splitHeaderBody(eml)
	_ = headers
	lines := strings.Split(strings.TrimSpace(bodyPart), "\n")
	for i, line := range lines {
		// RFC 5322: 7bit lines MUST NOT exceed 998 characters.
		if len(line) > 998 {
			t.Fatalf("7bit line %d too long: %d (RFC 5322 limit is 998)", i, len(line))
		}
	}
}

// TestBuild_InlineAndAttachment verifies build inline and attachment.
func TestBuild_InlineAndAttachment(t *testing.T) {
	imgBytes := []byte("fake-png")
	pdfBytes := []byte("fake-pdf")
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("inline+attachment").
		Date(fixedDate).
		MessageID("test@x").
		HTMLBody([]byte(`<img src="cid:img1"/>`)).
		AddInline(imgBytes, "image/png", "img.png", "img1").
		AddAttachment(pdfBytes, "application/pdf", "doc.pdf").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	eml := string(raw)

	if !strings.Contains(eml, "multipart/mixed") {
		t.Errorf("expected multipart/mixed (attachment present):\n%s", eml)
	}
	if !strings.Contains(eml, "multipart/related") {
		t.Errorf("expected multipart/related (inline present):\n%s", eml)
	}
	if !strings.Contains(eml, "Content-Disposition: attachment") {
		t.Errorf("missing attachment disposition:\n%s", eml)
	}
	if !strings.Contains(eml, "Content-Id: <img1>") {
		t.Errorf("missing inline Content-ID:\n%s", eml)
	}
}

// ContentID without angle brackets is normalised to <id> form.
func TestBuild_InlineContentIDNormalisation(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("cid test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("body")).
		AddInline([]byte("data"), "image/gif", "a.gif", "already-no-brackets").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Content-Id: <already-no-brackets>") {
		t.Errorf("Content-ID should be wrapped in angle brackets:\n%s", raw)
	}
}

// ── extra Header ─────────────────────────────────────────────────────────────

// TestBuild_ExtraHeader verifies build extra header.
func TestBuild_ExtraHeader(t *testing.T) {
	raw, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Header("X-Custom", "my-value").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "X-Custom: my-value") {
		t.Errorf("extra header missing:\n%s", raw)
	}
}

// ── CRLF / header-injection guards ───────────────────────────────────────────

// TestSubjectCRLFRejected verifies subject CR LF rejected.
func TestSubjectCRLFRejected(t *testing.T) {
	for _, inj := range []string{"legit\r\nBcc: evil@evil.com", "legit\nBcc: evil@evil.com", "legit\rBcc: evil@evil.com"} {
		_, err := New().
			From("", "alice@example.com").
			To("", "bob@example.com").
			Subject(inj).
			Date(fixedDate).
			MessageID("test@x").
			TextBody([]byte("hi")).
			Build()
		if err == nil {
			t.Errorf("Subject(%q): expected error, got nil", inj)
		}
	}
}

// TestMessageIDCRLFRejected verifies message ID CR LF rejected.
func TestMessageIDCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("bad\r\nX-Injected: hdr").
		TextBody([]byte("hi")).
		Build()
	if err == nil {
		t.Error("MessageID with CRLF: expected error, got nil")
	}
}

// TestInReplyToCRLFRejected verifies in reply to CR LF rejected.
func TestInReplyToCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		InReplyTo("legit\r\nBcc: evil@evil.com").
		TextBody([]byte("hi")).
		Build()
	if err == nil {
		t.Error("InReplyTo with CRLF: expected error, got nil")
	}
}

// TestReferencesCRLFRejected verifies references CR LF rejected.
func TestReferencesCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		References("legit@x\r\nBcc: evil@evil.com").
		TextBody([]byte("hi")).
		Build()
	if err == nil {
		t.Error("References with CRLF: expected error, got nil")
	}
}

// TestHeaderNameColonRejected verifies header name colon rejected.
func TestHeaderNameColonRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Header("X-Bad:Name", "value").
		Build()
	if err == nil {
		t.Error("Header with colon in name: expected error, got nil")
	}
}

// TestHeaderNameCRLFRejected verifies header name CR LF rejected.
func TestHeaderNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Header("X-Bad\r\nBcc", "evil@evil.com").
		Build()
	if err == nil {
		t.Error("Header with CRLF in name: expected error, got nil")
	}
}

// TestHeaderValueCRLFRejected verifies header value CR LF rejected.
func TestHeaderValueCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Header("X-Custom", "legit\r\nBcc: evil@evil.com").
		Build()
	if err == nil {
		t.Error("Header with CRLF in value: expected error, got nil")
	}
}

// TestFromDisplayNameCRLFRejected verifies from display name CR LF rejected.
func TestFromDisplayNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("Alice\r\nBcc: evil@evil.com", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Build()
	if err == nil {
		t.Error("From with CRLF in display name: expected error, got nil")
	}
}

// TestToDisplayNameCRLFRejected verifies to display name CR LF rejected.
func TestToDisplayNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("Bob\r\nBcc: evil@evil.com", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Build()
	if err == nil {
		t.Error("To with CRLF in display name: expected error, got nil")
	}
}

// TestAddAttachmentContentTypeCRLFRejected verifies add attachment content type CR LF rejected.
func TestAddAttachmentContentTypeCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddAttachment([]byte("data"), "application/pdf\r\nBcc: evil@evil.com", "file.pdf").
		Build()
	if err == nil {
		t.Error("AddAttachment with CRLF in contentType: expected error, got nil")
	}
}

// TestAddAttachmentFileNameCRLFRejected verifies add attachment file name CR LF rejected.
func TestAddAttachmentFileNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddAttachment([]byte("data"), "application/pdf", "file.pdf\r\nBcc: evil@evil.com").
		Build()
	if err == nil {
		t.Error("AddAttachment with CRLF in fileName: expected error, got nil")
	}
}

// TestAddInlineContentTypeCRLFRejected verifies add inline content type CR LF rejected.
func TestAddInlineContentTypeCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddInline([]byte("data"), "image/png\r\nBcc: evil@evil.com", "img.png", "cid1").
		Build()
	if err == nil {
		t.Error("AddInline with CRLF in contentType: expected error, got nil")
	}
}

// TestAddInlineContentIDCRLFRejected verifies add inline content ID CR LF rejected.
func TestAddInlineContentIDCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddInline([]byte("data"), "image/png", "img.png", "cid1\r\nBcc: evil@evil.com").
		Build()
	if err == nil {
		t.Error("AddInline with CRLF in contentID: expected error, got nil")
	}
}

// TestAddInlineFileNameCRLFRejected verifies add inline file name CR LF rejected.
func TestAddInlineFileNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddInline([]byte("data"), "image/png", "img.png\r\nBcc: evil@evil.com", "cid1").
		Build()
	if err == nil {
		t.Error("AddInline with CRLF in fileName: expected error, got nil")
	}
}

// TestAddOtherPartFileNameCRLFRejected verifies add other part file name CR LF rejected.
func TestAddOtherPartFileNameCRLFRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddOtherPart([]byte("data"), "application/octet-stream", "file.bin\r\nBcc: evil@evil.com", "").
		Build()
	if err == nil {
		t.Error("AddOtherPart with CRLF in fileName: expected error, got nil")
	}
}

// TestAddInlineContentIDControlCharRejected verifies add inline content ID control char rejected.
func TestAddInlineContentIDControlCharRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddInline([]byte("data"), "image/png", "img.png", "cid1\x01evil").
		Build()
	if err == nil {
		t.Error("AddInline with control char (0x01) in contentID: expected error, got nil")
	}
}

// TestAddOtherPartContentIDControlCharRejected verifies add other part content ID control char rejected.
func TestAddOtherPartContentIDControlCharRejected(t *testing.T) {
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		AddOtherPart([]byte("data"), "application/octet-stream", "file.bin", "cid1\x09evil").
		Build()
	if err == nil {
		t.Error("AddOtherPart with control char (tab/0x09) in contentID: expected error, got nil")
	}
}

// TestHeaderValueControlCharRejected verifies header value control char rejected.
func TestHeaderValueControlCharRejected(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"null byte", "hello\x00world"},
		{"ESC", "hello\x1bworld"},
		{"DEL", "hello\x7fworld"},
		{"CR", "hello\rworld"},
		{"LF", "hello\nworld"},
		{"CRLF", "hello\r\nworld"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New().
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				TextBody([]byte("hi")).
				Header("X-Custom", tc.value).
				Build()
			if err == nil {
				t.Errorf("Header with %s in value: expected error, got nil", tc.name)
			}
		})
	}
}

// TestHeaderValueDangerousUnicodeRejected verifies header value dangerous unicode rejected.
func TestHeaderValueDangerousUnicodeRejected(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"Bidi RLO (U+202E)", "file\u202Etxt.exe"},
		{"zero-width space (U+200B)", "hello\u200Bworld"},
		{"BOM (U+FEFF)", "hello\uFEFFworld"},
		{"line separator (U+2028)", "hello\u2028world"},
		{"Bidi isolate LRI (U+2066)", "hello\u2066world"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New().
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				TextBody([]byte("hi")).
				Header("X-Custom", tc.value).
				Build()
			if err == nil {
				t.Errorf("Header with %s in value: expected error, got nil", tc.name)
			}
		})
	}
}

// ── blocked extension via AddFileAttachment ───────────────────────────────────

// TestAddFileAttachmentBlockedExtension verifies add file attachment blocked extension.
func TestAddFileAttachmentBlockedExtension(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	blocked := []string{"malware.exe", "script.BAT", "payload.js", "hack.ps1", "app.msi"}
	for _, name := range blocked {
		os.WriteFile(name, []byte("content"), 0o644)
	}
	for _, name := range blocked {
		t.Run(name, func(t *testing.T) {
			_, err := New().WithFileIO(testFIO).
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				TextBody([]byte("hi")).
				AddFileAttachment(name).
				Build()
			if err == nil {
				t.Fatalf("expected blocked extension error for %q", name)
			}
			if !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("error = %v, want 'not allowed' message", err)
			}
		})
	}
}

// TestAddFileInlineBlockedFormat verifies add file inline blocked format.
func TestAddFileInlineBlockedFormat(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	// PNG magic bytes but .svg extension → rejected (bad extension)
	pngContent := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	os.WriteFile("icon.svg", pngContent, 0o644)
	// .png extension but EXE content → rejected (bad content)
	os.WriteFile("evil.png", []byte("MZ"), 0o644)

	for _, name := range []string{"icon.svg", "evil.png"} {
		t.Run(name, func(t *testing.T) {
			_, err := New().WithFileIO(testFIO).
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				HTMLBody([]byte(`<img src="cid:img1">`)).
				AddFileInline(name, "img1").
				Build()
			if err == nil {
				t.Fatalf("expected inline format error for %q", name)
			}
		})
	}
}

// TestAddFileInlineAllowedFormat verifies add file inline allowed format.
func TestAddFileInlineAllowedFormat(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	pngContent := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	jpegContent := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	os.WriteFile("logo.png", pngContent, 0o644)
	os.WriteFile("photo.jpg", jpegContent, 0o644)

	for _, name := range []string{"logo.png", "photo.jpg"} {
		t.Run(name, func(t *testing.T) {
			_, err := New().WithFileIO(testFIO).
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				HTMLBody([]byte(`<img src="cid:img1">`)).
				AddFileInline(name, "img1").
				Build()
			if err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", name, err)
			}
		})
	}
}

// TestAddFileAttachmentAllowedExtension verifies add file attachment allowed extension.
func TestAddFileAttachmentAllowedExtension(t *testing.T) {
	dir := t.TempDir()
	orig, _ := os.Getwd()
	os.Chdir(dir)
	t.Cleanup(func() { os.Chdir(orig) })

	allowed := []string{"report.pdf", "photo.jpg", "data.csv", "page.html"}
	for _, name := range allowed {
		os.WriteFile(name, []byte("content"), 0o644)
	}
	for _, name := range allowed {
		t.Run(name, func(t *testing.T) {
			_, err := New().WithFileIO(testFIO).
				From("", "alice@example.com").
				To("", "bob@example.com").
				Subject("test").
				Date(fixedDate).
				MessageID("test@x").
				TextBody([]byte("hi")).
				AddFileAttachment(name).
				Build()
			if err != nil {
				t.Fatalf("expected %q to be allowed, got: %v", name, err)
			}
		})
	}
}

// TestHeaderValueTabAllowed verifies header value tab allowed.
func TestHeaderValueTabAllowed(t *testing.T) {
	// Tab (\t) is valid in folded header values per RFC 5322
	_, err := New().
		From("", "alice@example.com").
		To("", "bob@example.com").
		Subject("test").
		Date(fixedDate).
		MessageID("test@x").
		TextBody([]byte("hi")).
		Header("X-Custom", "hello\tworld").
		Build()
	if err != nil {
		t.Errorf("Header with tab in value: expected no error, got %v", err)
	}
}

func TestWriteCalendarPart_MethodFromBody(t *testing.T) {
	cases := []struct {
		name   string
		ics    string
		wantCT string
	}{
		{"request", "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nEND:VCALENDAR\r\n", "method=REQUEST"},
		{"cancel", "BEGIN:VCALENDAR\r\nMETHOD:CANCEL\r\nEND:VCALENDAR\r\n", "method=CANCEL"},
		{"reply", "BEGIN:VCALENDAR\r\nMETHOD:REPLY\r\nEND:VCALENDAR\r\n", "method=REPLY"},
		{"no method defaults to REQUEST", "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n", "method=REQUEST"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eml, err := New().
				From("", "sender@example.com").
				To("", "recipient@example.com").
				Subject("Test").
				Date(fixedDate).
				MessageID("test-method@x").
				HTMLBody([]byte("<p>hi</p>")).
				CalendarBody([]byte(tc.ics)).
				Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if !strings.Contains(string(eml), tc.wantCT) {
				t.Errorf("expected Content-Type to contain %q\n%s", tc.wantCT, eml)
			}
		})
	}
}
