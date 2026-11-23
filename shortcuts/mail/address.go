// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"mime"
	"strings"
)

// Mailbox is a parsed RFC 2822 address: an optional display name plus an
// email address.  The zero value represents a bare address with no name.
type Mailbox struct {
	Name  string // display name; empty if not present
	Email string
}

// ParseMailbox parses a single address in any of the following forms:
//
//	alice@example.com
//	Alice Smith <alice@example.com>
//	"Alice Smith" <alice@example.com>
//
// The function is intentionally total (never returns an error): syntactic
// validation of the email address is left to the Lark API.  Control
// characters are stripped as a defense against header injection.
func ParseMailbox(raw string) Mailbox {
	raw = strings.TrimSpace(raw)
	if lt := strings.LastIndex(raw, "<"); lt >= 0 {
		if gt := strings.Index(raw[lt:], ">"); gt >= 0 {
			email := sanitizeControlChars(strings.TrimSpace(raw[lt+1 : lt+gt]))
			namePart := strings.TrimSpace(raw[:lt])
			// Strip surrounding quotes: "Alice" → Alice
			namePart = strings.TrimPrefix(namePart, `"`)
			namePart = strings.TrimSuffix(namePart, `"`)
			return Mailbox{Name: sanitizeControlChars(namePart), Email: email}
		}
	}
	return Mailbox{Email: sanitizeControlChars(raw)}
}

// ParseMailboxList splits a comma-separated address list and parses each
// entry.  Entries with an empty email address are silently dropped.
func ParseMailboxList(raw string) []Mailbox {
	var out []Mailbox
	for _, part := range splitAddressList(raw) {
		m := ParseMailbox(part)
		if m.Email != "" {
			out = append(out, m)
		}
	}
	return out
}

// String formats the mailbox for an RFC 2822 header value.
// Non-ASCII display names are encoded using RFC 2047.
func (m Mailbox) String() string {
	if m.Name == "" {
		return m.Email
	}
	return formatDisplayName(m.Name) + " <" + m.Email + ">"
}

// rawString formats a mailbox for intermediate CLI normalization. It preserves
// the original display-name text; RFC 2047 encoding belongs to final header
// rendering in String().
func (m Mailbox) rawString() string {
	if m.Name == "" {
		return m.Email
	}
	return quoteDisplayNameIfNeeded(m.Name) + " <" + m.Email + ">"
}

func formatDisplayName(name string) string {
	encoded := encodeHeader(name)
	if encoded != name {
		return encoded
	}
	return quoteDisplayNameIfNeeded(name)
}

func quoteDisplayNameIfNeeded(name string) string {
	if !strings.ContainsAny(name, "\",;<>@()[]:\\") {
		return name
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name)
	return `"` + escaped + `"`
}

// sanitizeControlChars strips ASCII control characters (0x00–0x1F, 0x7F)
// from a string.  This is applied at the address-parse boundary as a
// defence-in-depth measure against CRLF injection: an attacker who controls
// a display name or email value cannot smuggle extra header lines.
func sanitizeControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0x20 && r != 0x7F {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// encodeHeader encodes a header value that contains non-ASCII characters
// using RFC 2047 base64 ("B") encoding.  ASCII-only values are returned
// unchanged.
func encodeHeader(val string) string {
	for _, r := range val {
		if r > 127 {
			return mime.BEncoding.Encode("UTF-8", val)
		}
	}
	return val
}

// splitAddressList splits a raw comma-separated address list while respecting
// quoted strings (so a display name like `"Doe, Jane" <j@x>` is not split on
// the comma inside the quotes).
func splitAddressList(raw string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, r := range raw {
		switch {
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		parts = append(parts, s)
	}
	return parts
}
