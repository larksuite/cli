// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package ics provides RFC 5545 iCalendar generation and parsing for mail calendar invitations.
package ics

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Event holds the data needed to generate an ICS VCALENDAR invitation.
type Event struct {
	UID       string    // auto-generated if empty
	Summary   string    // SUMMARY (required)
	Location  string    // LOCATION (optional)
	Start     time.Time // DTSTART (required)
	End       time.Time // DTEND (required)
	Organizer Address   // ORGANIZER
	Attendees []Address // ATTENDEE list (To + Cc, excluding Bcc)
}

// Address represents a name + email pair for ORGANIZER / ATTENDEE.
type Address struct {
	Name  string
	Email string
}

// Build generates a RFC 5545 VCALENDAR byte slice with METHOD:REQUEST.
// The output is suitable for use as a text/calendar MIME part.
func Build(event Event) []byte {
	uid := event.UID
	if uid == "" {
		uid = uuid.New().String()
	}

	now := time.Now().UTC()
	nowICS := formatICSTime(now)
	var b strings.Builder

	b.WriteString("BEGIN:VCALENDAR\r\n")
	b.WriteString("CALSCALE:GREGORIAN\r\n")
	b.WriteString("VERSION:2.0\r\n")
	b.WriteString("PRODID:-//Lark CLI//EN\r\n")
	b.WriteString("METHOD:REQUEST\r\n")
	b.WriteString("X-LARK-MAIL-DRAFT:TRUE\r\n")
	b.WriteString("BEGIN:VEVENT\r\n")
	writeFolded(&b, "UID", uid)
	writeFolded(&b, "DTSTAMP", nowICS)
	writeFolded(&b, "CREATED", nowICS)
	writeFolded(&b, "LAST-MODIFIED", nowICS)
	writeFolded(&b, "DTSTART", formatICSTime(event.Start.UTC()))
	writeFolded(&b, "DTEND", formatICSTime(event.End.UTC()))
	writeFolded(&b, "SUMMARY", escapeTextValue(event.Summary))
	if event.Location != "" {
		writeFolded(&b, "LOCATION", escapeTextValue(event.Location))
	}
	b.WriteString("STATUS:CONFIRMED\r\n")
	b.WriteString("TRANSP:OPAQUE\r\n")
	b.WriteString("SEQUENCE:0\r\n")
	if event.Organizer.Email != "" {
		organizer := "ORGANIZER;ROLE=CHAIR"
		if event.Organizer.Name != "" {
			organizer += ";CN=" + quoteCNParam(event.Organizer.Name)
		} else {
			organizer += ";CN=" + quoteCNParam(event.Organizer.Email)
		}
		writeFolded(&b, organizer, mailtoScheme+sanitizeMailtoAddress(event.Organizer.Email))
	}
	for _, a := range event.Attendees {
		attendee := "ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CUTYPE=INDIVIDUAL"
		if a.Name != "" {
			attendee += ";CN=" + quoteCNParam(a.Name)
		} else {
			attendee += ";CN=" + quoteCNParam(a.Email)
		}
		attendee += ";PARTSTAT=NEEDS-ACTION"
		writeFolded(&b, attendee, mailtoScheme+sanitizeMailtoAddress(a.Email))
	}
	b.WriteString("END:VEVENT\r\n")
	b.WriteString("END:VCALENDAR\r\n")

	return []byte(b.String())
}

// formatICSTime formats a time.Time as ICS UTC: YYYYMMDDTHHMMSSZ.
func formatICSTime(t time.Time) string {
	return t.Format("20060102T150405Z")
}

// escapeTextValue escapes a string for use as an ICS TEXT value per RFC 5545
// §3.3.11: backslash, newline, semicolon, and comma carry structural meaning
// and must be escaped. Applied to SUMMARY, LOCATION, DESCRIPTION etc. — not
// to identifiers (UID), date-times (DTSTART/DTEND), or URIs.
//
// Without this, a user-supplied summary containing a newline or colon would
// let the payload inject a fake property line, e.g.
//
//	--event-summary "foo\nDTSTART:20000101T000000Z"
//
// would turn into a second DTSTART line after folding.
func escapeTextValue(s string) string {
	// Normalise CR / CRLF so downstream only sees LF.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// Order matters: escape backslash first so its own replacement is not
	// picked up by later rules.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, ";", `\;`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return s
}

// quoteCNParam wraps a CN parameter value in double-quotes per RFC 5545 §3.2
// when the value contains characters that are not allowed in an unquoted
// paramtext (,  ; :). Characters that are illegal inside a quoted-string are
// stripped: DQUOTE (%x22) is excluded by QSAFE-CHAR, and control characters
// (%x00–%x08, %x0A–%x1F, %x7F) would break the property line structure.
func quoteCNParam(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '"' || r < 0x09 || (r >= 0x0A && r <= 0x1F) || r == 0x7F {
			return -1
		}
		return r
	}, s)
	if strings.ContainsAny(s, ",:;") {
		return `"` + s + `"`
	}
	return s
}

// writeFolded writes a property line with RFC 5545 line folding (75-octet limit).
// Long lines are folded by inserting CRLF + space at UTF-8 character boundaries.
// Continuation lines begin with a single SPACE (1 octet), so their content is
// limited to 74 octets to keep the total physical line at ≤ 75 octets.
func writeFolded(b *strings.Builder, name, value string) {
	line := fmt.Sprintf("%s:%s", name, value)
	const maxLineOctets = 75 // RFC 5545 §3.1: lines SHOULD NOT be longer than 75 octets
	limit := maxLineOctets
	for len(line) > limit {
		// Find the last complete UTF-8 character that fits within the limit.
		cut := 0
		for i := 0; i < len(line); {
			_, size := utf8.DecodeRuneInString(line[i:])
			if i+size > limit {
				break
			}
			i += size
			cut = i
		}
		if cut == 0 {
			// Single character exceeds limit (shouldn't happen in practice).
			cut = limit
		}
		b.WriteString(line[:cut])
		b.WriteString("\r\n ")
		line = line[cut:]
		limit = maxLineOctets - 1 // continuation lines: 1-octet SPACE + 74 content = 75
	}
	b.WriteString(line)
	b.WriteString("\r\n")
}

// sanitizeMailtoAddress strips control characters (CR, LF, and other chars
// below 0x20 or equal to 0x7F) from an email address before embedding it in a
// MAILTO: URI value. Prevents property-injection attacks analogous to the CN
// parameter protection in quoteCNParam.
func sanitizeMailtoAddress(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}
