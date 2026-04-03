// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package emlbuilder provides a Lark-API-compatible RFC 2822 EML message builder.
//
// It is designed for use with the Lark mail drafts API
// (POST /open-apis/mail/v1/user_mailboxes/me/drafts), which requires the
// complete EML to be base64url-encoded and placed in the "raw" request field.
// After creating a draft, send it via POST .../drafts/{draft_id}/send.
//
// Key differences from standard MIME libraries:
//   - Line endings are LF (\n), not CRLF — Lark API requires this.
//   - Content-Type parameters are never folded onto a new line — Lark's MIME
//     parser does not handle header folding correctly.
//   - Non-ASCII body content is encoded as base64 (StdEncoding) — 7bit and 8bit
//     are rejected by Lark for non-ASCII content.
//   - BuildBase64URL() produces the base64url (URLEncoding) output that goes
//     directly into the API's "raw" field.
//
// MIME structure produced by Build():
//
//	multipart/mixed              ← only when attachments exist
//	└─ multipart/related         ← only when CID inline/other parts exist
//	   └─ multipart/alternative  ← only when multiple body types coexist
//	      ├─ text/plain
//	      ├─ text/html
//	      └─ text/calendar
//	   └─ inline/other parts (CID)
//	└─ attachments
//
// Usage:
//
//	raw, err := emlbuilder.New().
//	    From("", "alice@example.com").
//	    To("", "bob@example.com").
//	    Subject("Hello").
//	    TextBody([]byte("Hi Bob")).
//	    HTMLBody([]byte("<p>Hi Bob</p>")).
//	    AddInline(imgBytes, "image/png", "logo.png", "logo").
//	    BuildBase64URL()
package emlbuilder

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math/rand"
	"mime"
	"net/mail"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/mail/filecheck"
)

// MaxEMLSize is the maximum allowed raw EML size in bytes.
const MaxEMLSize = 25 * 1024 * 1024 // 25 MB

// readFile reads the named file and returns its contents.
func readFile(path string) ([]byte, error) {
	safePath, err := validate.SafeInputPath(path)
	if err != nil {
		return nil, fmt.Errorf("attachment %q: %w", path, err)
	}
	return vfs.ReadFile(safePath)
}

// Builder constructs a Lark-compatible RFC 2822 EML message.
// All setter methods return a copy of the Builder (immutable/fluent style),
// so a base builder can be reused across multiple goroutines safely.
type Builder struct {
	from                mail.Address
	to                  []mail.Address
	cc                  []mail.Address
	bcc                 []mail.Address
	replyTo             []mail.Address
	subject             string
	date                time.Time
	messageID           string
	inReplyTo           string // raw value, without angle brackets
	references          string // space-separated list of message IDs, with angle brackets
	lmsReplyToMessageID string // Lark internal message_id of the original message
	textBody            []byte
	htmlBody            []byte
	calendarBody        []byte
	attachments         []attachment
	inlines             []inline
	extraHeaders        [][2]string // ordered list of [name, value] pairs
	allowNoRecipients   bool        // when true, Build() skips the recipient check (for drafts)
	err                 error
}

type attachment struct {
	content     []byte
	contentType string
	fileName    string
}

// inline represents a CID-referenced embedded MIME part (inline image or other resource).
type inline struct {
	content     []byte
	contentType string
	fileName    string
	contentID   string // without angle brackets
	isOtherPart bool   // true = no Content-Disposition (AddOtherPart); false = Content-Disposition: inline
}

// New returns an empty Builder.
func New() Builder {
	return Builder{}
}

// validateHeaderValue rejects strings that contain characters unsafe in MIME
// header values: C0 control chars (except \t for folded headers), DEL (0x7F),
// and dangerous Unicode (Bidi overrides, zero-width chars) that enable
// visual-spoofing attacks.
func validateHeaderValue(v string) error {
	for _, r := range v {
		if r != '\t' && (r < 0x20 || r == 0x7f) {
			return fmt.Errorf("emlbuilder: header value contains control character: %q", v)
		}
		if isHeaderDangerousUnicode(r) {
			return fmt.Errorf("emlbuilder: header value contains dangerous Unicode character: %q", v)
		}
	}
	return nil
}

// isHeaderDangerousUnicode identifies Unicode code points used for visual
// spoofing: Bidi overrides that reverse display order, and zero-width characters
// that hide content.  These must not appear in email header values.
func isHeaderDangerousUnicode(r rune) bool {
	switch {
	case r >= 0x200B && r <= 0x200D: // zero-width space/non-joiner/joiner
		return true
	case r == 0xFEFF: // BOM / zero-width no-break space
		return true
	case r >= 0x202A && r <= 0x202E: // Bidi: LRE/RLE/PDF/LRO/RLO
		return true
	case r >= 0x2028 && r <= 0x2029: // line/paragraph separator
		return true
	case r >= 0x2066 && r <= 0x2069: // Bidi isolates: LRI/RLI/FSI/PDI
		return true
	}
	return false
}

// validateHeaderName rejects any string that contains ':', CR (\r), LF (\n),
// or non-printable ASCII characters, as required by RFC 5322 field-name syntax.
func validateHeaderName(n string) error {
	if strings.ContainsAny(n, ":\r\n") {
		return fmt.Errorf("emlbuilder: header name contains ':', CR, or LF: %q", n)
	}
	for _, r := range n {
		if r < 0x21 || r > 0x7e {
			return fmt.Errorf("emlbuilder: header name contains non-printable character: %q", n)
		}
	}
	return nil
}

// validateDisplayName rejects display names containing CR or LF, which could
// escape the quoted-string encoding used by mail.Address.String() and inject headers.
func validateDisplayName(name string) error {
	if strings.ContainsAny(name, "\r\n") {
		return fmt.Errorf("emlbuilder: display name contains CR or LF: %q", name)
	}
	return nil
}

// validateCID rejects content IDs containing ASCII control characters (0x00–0x1F, 0x7F).
// RFC 2045 Content-ID has the same syntax as Message-ID; control characters are never valid.
func validateCID(cid string) error {
	for _, r := range cid {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("emlbuilder: content ID contains control character: %q", cid)
		}
	}
	return nil
}

// From sets the From header. name may be empty.
func (b Builder) From(name, addr string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateDisplayName(name); err != nil {
		b.err = err
		return b
	}
	b.from = mail.Address{Name: name, Address: addr}
	return b
}

// To appends an address to the To header. name may be empty.
func (b Builder) To(name, addr string) Builder {
	if addr == "" {
		return b
	}
	if b.err != nil {
		return b
	}
	if err := validateDisplayName(name); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.to = append(cp.to, mail.Address{Name: name, Address: addr})
	return cp
}

// ToAddrs sets the To header to the given address list.
func (b Builder) ToAddrs(addrs []mail.Address) Builder {
	b.to = addrs
	return b
}

// CC appends an address to the Cc header. name may be empty.
func (b Builder) CC(name, addr string) Builder {
	if addr == "" {
		return b
	}
	if b.err != nil {
		return b
	}
	if err := validateDisplayName(name); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.cc = append(cp.cc, mail.Address{Name: name, Address: addr})
	return cp
}

// CCAddrs sets the Cc header to the given address list.
func (b Builder) CCAddrs(addrs []mail.Address) Builder {
	b.cc = addrs
	return b
}

// BCC appends an address to the Bcc list.
// Bcc addresses are included in AllRecipients() but not written to the EML headers.
func (b Builder) BCC(name, addr string) Builder {
	if addr == "" {
		return b
	}
	if b.err != nil {
		return b
	}
	if err := validateDisplayName(name); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.bcc = append(cp.bcc, mail.Address{Name: name, Address: addr})
	return cp
}

// BCCAddrs sets the Bcc list to the given address list.
func (b Builder) BCCAddrs(addrs []mail.Address) Builder {
	b.bcc = addrs
	return b
}

// ReplyTo appends an address to the Reply-To header. name may be empty.
func (b Builder) ReplyTo(name, addr string) Builder {
	if addr == "" {
		return b
	}
	if b.err != nil {
		return b
	}
	if err := validateDisplayName(name); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.replyTo = append(cp.replyTo, mail.Address{Name: name, Address: addr})
	return cp
}

// Subject sets the Subject header.
// Non-ASCII characters are automatically RFC 2047 B-encoded.
// Returns an error builder if subject contains CR or LF.
func (b Builder) Subject(subject string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(subject); err != nil {
		b.err = err
		return b
	}
	b.subject = subject
	return b
}

// Date sets the Date header. If not set, Build() uses time.Now().
func (b Builder) Date(date time.Time) Builder {
	b.date = date
	return b
}

// MessageID sets the Message-ID header value (without angle brackets).
// If not set, Build() generates a unique ID.
// Returns an error builder if id contains CR or LF.
func (b Builder) MessageID(id string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(id); err != nil {
		b.err = err
		return b
	}
	b.messageID = id
	return b
}

// InReplyTo sets the In-Reply-To header (the smtp_message_id of the original mail,
// without angle brackets). Used for reply threading.
// Returns an error builder if id contains CR or LF.
func (b Builder) InReplyTo(id string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(id); err != nil {
		b.err = err
		return b
	}
	b.inReplyTo = id
	return b
}

// LMSReplyToMessageID sets the Lark internal message_id of the original message.
// Written as X-LMS-Reply-To-Message-Id when In-Reply-To is also set.
// Returns an error builder if id contains CR or LF.
func (b Builder) LMSReplyToMessageID(id string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(id); err != nil {
		b.err = err
		return b
	}
	b.lmsReplyToMessageID = id
	return b
}

// References sets the References header value verbatim.
// Typically a space-separated list of message IDs including angle brackets,
// e.g. "<id1@host> <id2@host>".
// Returns an error builder if refs contains CR or LF.
func (b Builder) References(refs string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(refs); err != nil {
		b.err = err
		return b
	}
	b.references = refs
	return b
}

// TextBody sets the text/plain body.
func (b Builder) TextBody(body []byte) Builder {
	b.textBody = body
	return b
}

// HTMLBody sets the text/html body.
func (b Builder) HTMLBody(body []byte) Builder {
	b.htmlBody = body
	return b
}

// CalendarBody sets the text/calendar body (e.g. for meeting invitations).
// May be combined with TextBody and/or HTMLBody; the resulting parts are wrapped
// in multipart/alternative.
func (b Builder) CalendarBody(body []byte) Builder {
	b.calendarBody = body
	return b
}

// AddAttachment appends a file attachment.
// contentType should be a valid MIME type (e.g. "application/pdf").
// If contentType is empty, "application/octet-stream" is used.
// Returns an error builder if contentType or fileName contains CR or LF.
func (b Builder) AddAttachment(content []byte, contentType, fileName string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderValue(fileName); err != nil {
		b.err = err
		return b
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := validateHeaderValue(contentType); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.attachments = append(cp.attachments, attachment{
		content:     content,
		contentType: contentType,
		fileName:    fileName,
	})
	return cp
}

// AddFileAttachment reads a file from disk and appends it as an attachment.
// The backend canonicalizes regular attachments to application/octet-stream on
// save/readback, so the builder aligns with that behavior instead of inferring
// a richer MIME type from the local file extension. If reading the file fails,
// the error is stored and returned by Build().
func (b Builder) AddFileAttachment(path string) Builder {
	if b.err != nil {
		return b
	}
	if err := filecheck.CheckBlockedExtension(filepath.Base(path)); err != nil {
		b.err = err
		return b
	}
	content, err := readFile(path)
	if err != nil {
		b.err = err
		return b
	}
	name := filepath.Base(path)
	return b.AddAttachment(content, "application/octet-stream", name)
}

// AddInline appends a CID-referenced inline part (e.g. an embedded image).
// The part is written with Content-Disposition: inline, causing most mail clients
// to render it inline rather than as a download.
// contentID is a unique identifier without angle brackets; it matches the "cid:"
// reference in the HTML body (e.g. contentID="logo.png" matches src="cid:logo.png").
// When inline parts are present, the message body is automatically wrapped in
// multipart/related.
// Returns an error builder if contentType or fileName contains CR or LF, or if
// contentID contains any ASCII control character.
func (b Builder) AddInline(content []byte, contentType, fileName, contentID string) Builder {
	if b.err != nil {
		return b
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := validateHeaderValue(contentType); err != nil {
		b.err = err
		return b
	}
	if err := validateHeaderValue(fileName); err != nil {
		b.err = err
		return b
	}
	if err := validateCID(contentID); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.inlines = append(cp.inlines, inline{
		content:     content,
		contentType: contentType,
		fileName:    fileName,
		contentID:   contentID,
		isOtherPart: false,
	})
	return cp
}

// AddFileInline reads a file from disk and appends it as a CID inline part.
// The content type is inferred from the file extension.
// If reading the file fails, the error is stored and returned by Build().
func (b Builder) AddFileInline(path, contentID string) Builder {
	if b.err != nil {
		return b
	}
	content, err := readFile(path)
	if err != nil {
		b.err = err
		return b
	}
	name := filepath.Base(path)
	ct, err := filecheck.CheckInlineImageFormat(name, content)
	if err != nil {
		b.err = err
		return b
	}
	return b.AddInline(content, ct, name, contentID)
}

// AddOtherPart appends a CID-referenced embedded part without Content-Disposition.
// Unlike AddInline, this part carries no Content-Disposition header, which is
// appropriate for resources referenced via "cid:" that should not appear as inline
// attachments in the client UI (e.g. calendar objects or custom data blobs).
// When other parts are present, the message body is automatically wrapped in
// multipart/related.
// Returns an error builder if contentType or fileName contains CR or LF, or if
// contentID contains any ASCII control character.
func (b Builder) AddOtherPart(content []byte, contentType, fileName, contentID string) Builder {
	if b.err != nil {
		return b
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if err := validateHeaderValue(contentType); err != nil {
		b.err = err
		return b
	}
	if err := validateHeaderValue(fileName); err != nil {
		b.err = err
		return b
	}
	if err := validateCID(contentID); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.inlines = append(cp.inlines, inline{
		content:     content,
		contentType: contentType,
		fileName:    fileName,
		contentID:   contentID,
		isOtherPart: true,
	})
	return cp
}

// AddFileOtherPart reads a file from disk and appends it as a CID other-part
// (no Content-Disposition header). See AddOtherPart for details.
// If reading the file fails, the error is stored and returned by Build().
func (b Builder) AddFileOtherPart(path, contentID string) Builder {
	if b.err != nil {
		return b
	}
	content, err := readFile(path)
	if err != nil {
		b.err = err
		return b
	}
	name := filepath.Base(path)
	ct := mime.TypeByExtension(filepath.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return b.AddOtherPart(content, ct, name, contentID)
}

// AllowNoRecipients tells Build() to skip the recipient-required check.
// Use this for draft creation, where saving without recipients is valid.
func (b Builder) AllowNoRecipients() Builder {
	b.allowNoRecipients = true
	return b
}

// Header appends an extra header to the message.
// Multiple calls with the same name result in multiple header lines.
// Returns an error builder if name or value contains CR, LF, or (for names) ':'.
func (b Builder) Header(name, value string) Builder {
	if b.err != nil {
		return b
	}
	if err := validateHeaderName(name); err != nil {
		b.err = err
		return b
	}
	if err := validateHeaderValue(value); err != nil {
		b.err = err
		return b
	}
	cp := b.copySlices()
	cp.extraHeaders = append(cp.extraHeaders, [2]string{name, value})
	return cp
}

// Error returns any stored error (e.g. from AddFileAttachment), or nil.
func (b Builder) Error() error {
	return b.err
}

// AllRecipients returns all recipient addresses (To + CC + BCC).
// Useful for SMTP envelope construction.
func (b Builder) AllRecipients() []string {
	out := make([]string, 0, len(b.to)+len(b.cc)+len(b.bcc))
	for _, a := range b.to {
		out = append(out, a.Address)
	}
	for _, a := range b.cc {
		out = append(out, a.Address)
	}
	for _, a := range b.bcc {
		out = append(out, a.Address)
	}
	return out
}

// Build validates the builder and returns the raw EML bytes.
//
// Constraints (Lark API requirements):
//   - From is mandatory.
//   - At least one of To/CC/BCC must be set.
//   - Line endings are LF (\n), not CRLF.
//   - Content-Type parameters are written on a single line (no header folding).
//   - Non-ASCII body content is base64 (StdEncoding) encoded.
func (b Builder) Build() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.from.Address == "" {
		return nil, fmt.Errorf("emlbuilder: From address is required")
	}
	if !b.allowNoRecipients && len(b.to)+len(b.cc)+len(b.bcc) == 0 {
		return nil, fmt.Errorf("emlbuilder: at least one recipient (To/CC/BCC) is required")
	}

	date := b.date
	if date.IsZero() {
		date = time.Now()
	}

	msgID := b.messageID
	if msgID == "" {
		msgID = fmt.Sprintf("%d.%d@larksuite-cli", date.UnixNano(), rand.Int63())
	}

	var buf bytes.Buffer

	// ── Top-level headers ──────────────────────────────────────────────────────
	// Order follows common convention; Lark API does not require a specific order.
	writeHeader(&buf, "Subject", encodeHeaderValue(b.subject))
	writeHeader(&buf, "From", b.from.String())
	writeHeader(&buf, "MIME-Version", "1.0")
	writeHeader(&buf, "Date", date.Format(time.RFC1123Z))
	writeHeader(&buf, "Message-ID", "<"+msgID+">")

	if len(b.to) > 0 {
		writeHeader(&buf, "To", joinAddresses(b.to))
	}
	if len(b.cc) > 0 {
		writeHeader(&buf, "Cc", joinAddresses(b.cc))
	}
	if len(b.bcc) > 0 {
		writeHeader(&buf, "Bcc", joinAddresses(b.bcc))
	}
	if len(b.replyTo) > 0 {
		writeHeader(&buf, "Reply-To", joinAddresses(b.replyTo))
	}
	if b.inReplyTo != "" {
		writeHeader(&buf, "In-Reply-To", "<"+b.inReplyTo+">")
		if b.lmsReplyToMessageID != "" {
			writeHeader(&buf, "X-LMS-Reply-To-Message-Id", b.lmsReplyToMessageID)
		}
	}
	if b.references != "" {
		writeHeader(&buf, "References", b.references)
	}
	for _, kv := range b.extraHeaders {
		writeHeader(&buf, kv[0], kv[1])
	}

	// ── Body ───────────────────────────────────────────────────────────────────
	// Full MIME hierarchy (outer layers only present when needed):
	//   multipart/mixed → multipart/related → multipart/alternative → body parts
	if len(b.attachments) > 0 {
		outerB := newBoundary()
		writeHeader(&buf, "Content-Type", "multipart/mixed; boundary="+outerB)
		buf.WriteByte('\n')

		fmt.Fprintf(&buf, "--%s\n", outerB)
		writePrimaryBody(&buf, b)

		for _, att := range b.attachments {
			fmt.Fprintf(&buf, "--%s\n", outerB)
			writeAttachmentPart(&buf, att)
		}
		fmt.Fprintf(&buf, "--%s--\n", outerB)
	} else {
		writePrimaryBody(&buf, b)
	}

	raw := buf.Bytes()
	if len(raw) > MaxEMLSize {
		return nil, fmt.Errorf("emlbuilder: EML size %.1f MB exceeds the %.0f MB limit",
			float64(len(raw))/1024/1024, float64(MaxEMLSize)/1024/1024)
	}
	return raw, nil
}

// BuildBase64URL returns the EML encoded as base64url (RFC 4648).
// This is the value to place in the Lark API "raw" field.
func (b Builder) BuildBase64URL() (string, error) {
	raw, err := b.Build()
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(raw), nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// copySlices returns a shallow copy of b with independent slice headers,
// so append operations in setter methods do not mutate the original.
func (b Builder) copySlices() Builder {
	cp := b
	cp.to = append([]mail.Address{}, b.to...)
	cp.cc = append([]mail.Address{}, b.cc...)
	cp.bcc = append([]mail.Address{}, b.bcc...)
	cp.replyTo = append([]mail.Address{}, b.replyTo...)
	cp.attachments = append([]attachment{}, b.attachments...)
	cp.inlines = append([]inline{}, b.inlines...)
	cp.extraHeaders = append([][2]string{}, b.extraHeaders...)
	return cp
}

// writePrimaryBody writes the body block of the message (text + inline parts,
// but not attachments). If inline/other parts are present, the body is wrapped
// in multipart/related.
//
// This function writes starting from a Content-Type header, which is either a
// top-level message header (when no attachments) or a sub-part header (inside
// multipart/mixed after a boundary line).
func writePrimaryBody(buf *bytes.Buffer, b Builder) {
	if len(b.inlines) > 0 {
		relatedB := newBoundary()
		writeHeader(buf, "Content-Type", "multipart/related; boundary="+relatedB)
		buf.WriteByte('\n')

		fmt.Fprintf(buf, "--%s\n", relatedB)
		writeAlternativeOrSingleBody(buf, b)

		for _, il := range b.inlines {
			fmt.Fprintf(buf, "--%s\n", relatedB)
			writeInlinePart(buf, il)
		}
		fmt.Fprintf(buf, "--%s--\n", relatedB)
	} else {
		writeAlternativeOrSingleBody(buf, b)
	}
}

// writeAlternativeOrSingleBody writes the text body block.
// If multiple body types (text/plain, text/html, text/calendar) are present,
// they are wrapped in multipart/alternative. Otherwise a single part is written.
func writeAlternativeOrSingleBody(buf *bytes.Buffer, b Builder) {
	hasText := len(b.textBody) > 0
	hasHTML := len(b.htmlBody) > 0
	hasCal := len(b.calendarBody) > 0

	bodyCount := 0
	if hasText {
		bodyCount++
	}
	if hasHTML {
		bodyCount++
	}
	if hasCal {
		bodyCount++
	}

	switch {
	case bodyCount > 1:
		boundary := newBoundary()
		writeHeader(buf, "Content-Type", "multipart/alternative; boundary="+boundary)
		buf.WriteByte('\n')
		if hasText {
			writeBodyPart(buf, boundary, "text/plain", b.textBody)
		}
		if hasHTML {
			writeBodyPart(buf, boundary, "text/html", b.htmlBody)
		}
		if hasCal {
			writeBodyPart(buf, boundary, "text/calendar", b.calendarBody)
		}
		fmt.Fprintf(buf, "--%s--\n", boundary)
	case hasHTML:
		writeSingleBodyPartHeaders(buf, "text/html", b.htmlBody)
	case hasCal:
		writeSingleBodyPartHeaders(buf, "text/calendar", b.calendarBody)
	default:
		// text/plain (also handles empty body)
		writeSingleBodyPartHeaders(buf, "text/plain", b.textBody)
	}
}

// writeInlinePart writes a CID-referenced inline or other-part MIME part.
// The part body is always base64 (StdEncoding), written in 76-character lines.
func writeInlinePart(buf *bytes.Buffer, il inline) {
	rawCID := strings.TrimSpace(strings.TrimPrefix(strings.TrimSuffix(il.contentID, ">"), "<"))
	cid := rawCID
	if rawCID != "" {
		cid = "<" + rawCID + ">"
	}
	encodedName := encodeHeaderValue(il.fileName)
	fmt.Fprintf(buf, "Content-Type: %s; name=%q\n", il.contentType, encodedName)
	writeHeader(buf, "Content-Id", cid)
	writeHeader(buf, "Content-Transfer-Encoding", "base64")
	if !il.isOtherPart {
		fmt.Fprintf(buf, "Content-Disposition: inline; filename=%q\n", encodedName)
		if rawCID != "" {
			writeHeader(buf, "X-Attachment-Id", rawCID)
			writeHeader(buf, "X-Image-Id", rawCID)
		}
	}
	buf.WriteByte('\n')

	encoded := base64.StdEncoding.EncodeToString(il.content)
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteByte('\n')
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		buf.WriteString(encoded)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
}

// writeHeader writes "Name: value\n".
// NOTE: no folding — Lark's MIME parser does not handle folded headers.
// CR and LF are stripped as a last-resort defence against header injection;
// callers (validateHeaderValue, validateCID) already reject them explicitly.
func writeHeader(buf *bytes.Buffer, name, value string) {
	name = strings.NewReplacer("\r", "", "\n", "").Replace(name)
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	fmt.Fprintf(buf, "%s: %s\n", name, value)
}

// encodeHeaderValue RFC 2047 B-encodes s if it contains non-ASCII characters.
func encodeHeaderValue(s string) string {
	for _, r := range s {
		if r > 127 {
			return mime.BEncoding.Encode("utf-8", s)
		}
	}
	return s
}

// hasNonASCII returns true if b contains any byte > 127.
func hasNonASCII(b []byte) bool {
	for _, c := range b {
		if c > 127 {
			return true
		}
	}
	return false
}

// selectCTE chooses the Content-Transfer-Encoding for a body:
//   - "7bit"   — pure ASCII content
//   - "base64" — contains non-ASCII bytes (required by Lark API)
func selectCTE(body []byte) string {
	if hasNonASCII(body) {
		return "base64"
	}
	return "7bit"
}

// encodeBodyContent encodes body according to the chosen CTE.
// For base64, it uses StdEncoding (MIME internal standard).
func encodeBodyContent(body []byte, cte string) string {
	if cte == "base64" {
		return base64.StdEncoding.EncodeToString(body)
	}
	return string(body)
}

// writeFoldedBody writes the encoded part body with fixed-width line wrapping.
// RFC 2045 recommends 76 characters per encoded line; we apply the same width
// to all body parts for consistent MIME output.
func writeFoldedBody(buf *bytes.Buffer, encoded string, width int) {
	if width <= 0 {
		width = 76
	}
	for _, line := range strings.Split(encoded, "\n") {
		for len(line) > width {
			buf.WriteString(line[:width])
			buf.WriteByte('\n')
			line = line[width:]
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
}

// writeBodyPart writes a MIME part within a multipart boundary:
//
//	--<boundary>
//	Content-Type: <ct>; charset=UTF-8
//	Content-Transfer-Encoding: <cte>
//	<blank line>
//	<body>
//	<blank line>
func writeBodyPart(buf *bytes.Buffer, boundary, ct string, body []byte) {
	fmt.Fprintf(buf, "--%s\n", boundary)
	cte := selectCTE(body)
	fmt.Fprintf(buf, "Content-Type: %s; charset=UTF-8\n", ct)
	fmt.Fprintf(buf, "Content-Transfer-Encoding: %s\n\n", cte)
	writeFoldedBody(buf, encodeBodyContent(body, cte), 76)
}

// writeSingleBodyPartHeaders writes the Content-Type / CTE headers and body
// for a single-part (non-multipart) message.
// The blank line separating headers from body is included.
func writeSingleBodyPartHeaders(buf *bytes.Buffer, ct string, body []byte) {
	cte := selectCTE(body)
	fmt.Fprintf(buf, "Content-Type: %s; charset=UTF-8\n", ct)
	fmt.Fprintf(buf, "Content-Transfer-Encoding: %s\n\n", cte)
	writeFoldedBody(buf, encodeBodyContent(body, cte), 76)
}

// writeAttachmentPart writes a MIME attachment part.
// Body is always base64 (StdEncoding), written in 76-character lines per RFC 2045.
func writeAttachmentPart(buf *bytes.Buffer, att attachment) {
	encodedName := encodeHeaderValue(att.fileName)
	fmt.Fprintf(buf, "Content-Type: %s; name=%q\n", att.contentType, encodedName)
	fmt.Fprintf(buf, "Content-Disposition: attachment; filename=%q\n", encodedName)
	fmt.Fprintf(buf, "Content-Transfer-Encoding: base64\n\n")

	encoded := base64.StdEncoding.EncodeToString(att.content)
	for len(encoded) > 76 {
		buf.WriteString(encoded[:76])
		buf.WriteByte('\n')
		encoded = encoded[76:]
	}
	if len(encoded) > 0 {
		buf.WriteString(encoded)
		buf.WriteByte('\n')
	}
	buf.WriteByte('\n')
}

// newBoundary generates a random MIME boundary string.
func newBoundary() string {
	return fmt.Sprintf("lark-%016x", rand.Int63())
}

// joinAddresses formats a list of mail.Address as a comma-separated string.
func joinAddresses(addrs []mail.Address) string {
	parts := make([]string, len(addrs))
	for i, a := range addrs {
		parts[i] = a.String()
	}
	return strings.Join(parts, ", ")
}
