// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ics

import (
	"strings"
	"time"
)

// mailtoScheme is the canonical case for the RFC 5545 ORGANIZER / ATTENDEE
// CAL-ADDRESS URI scheme. Emitted by the builder in upper-case to match
// Feishu client output; matched case-insensitively by the parser.
const mailtoScheme = "MAILTO:"

// ParsedEvent holds key fields extracted from an ICS VCALENDAR.
type ParsedEvent struct {
	Method       string    // VCALENDAR-level METHOD (REQUEST/REPLY/CANCEL)
	IsLarkDraft  bool      // true when VCALENDAR contains X-LARK-MAIL-DRAFT (Feishu private property indicating the event is editable)
	UID          string    // VEVENT UID
	Summary      string    // VEVENT SUMMARY, RFC 5545 TEXT unescaped
	Location     string    // VEVENT LOCATION, RFC 5545 TEXT unescaped
	Start        time.Time // VEVENT DTSTART
	End          time.Time // VEVENT DTEND
	Organizer    string    // ORGANIZER email (from MAILTO: URI or bare email)
	Attendees    []string  // ATTENDEE emails (from MAILTO: URIs or bare emails)
	OriginalTime int64     // RECURRENCE-ID as Unix seconds, 0 if not present. Used together with UID to derive the Feishu calendar event_id = UID + "_" + OriginalTime.
}

// ParseEvent extracts key fields from an ICS VCALENDAR string.
// Returns nil if no VEVENT is found.
func ParseEvent(icsText string) *ParsedEvent {
	// Step 1: line unfolding (RFC 5545 §3.1)
	unfolded := unfoldLines(icsText)

	lines := strings.Split(unfolded, "\n")
	var event ParsedEvent
	inVEvent := false
	foundVEvent := false

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		upper := strings.ToUpper(line)

		// VCALENDAR-level properties
		if !inVEvent && strings.HasPrefix(upper, "METHOD:") {
			event.Method = strings.TrimSpace(line[len("METHOD:"):])
			continue
		}
		if !inVEvent && strings.HasPrefix(upper, "X-LARK-MAIL-DRAFT:") {
			event.IsLarkDraft = true
			continue
		}

		if upper == "BEGIN:VEVENT" {
			inVEvent = true
			continue
		}
		if upper == "END:VEVENT" {
			inVEvent = false
			foundVEvent = true
			continue
		}

		if !inVEvent {
			continue
		}

		// VEVENT properties — RFC 5545 §3.1: property names are
		// case-insensitive and may carry parameters (NAME;PARAM=v:value).
		name, value := splitProperty(line)
		propUpper := strings.ToUpper(name)
		switch {
		case propUpper == "UID" || strings.HasPrefix(propUpper, "UID;"):
			event.UID = value
		case propUpper == "SUMMARY" || strings.HasPrefix(propUpper, "SUMMARY;"):
			event.Summary = unescapeTextValue(value)
		case propUpper == "LOCATION" || strings.HasPrefix(propUpper, "LOCATION;"):
			event.Location = unescapeTextValue(value)
		case propUpper == "DTSTART" || strings.HasPrefix(propUpper, "DTSTART;"):
			event.Start = parseICSTime(value, name)
		case propUpper == "DTEND" || strings.HasPrefix(propUpper, "DTEND;"):
			event.End = parseICSTime(value, name)
		case propUpper == "RECURRENCE-ID" || strings.HasPrefix(propUpper, "RECURRENCE-ID;"):
			if t := parseICSTime(value, name); !t.IsZero() {
				event.OriginalTime = t.Unix()
			}
		case propUpper == "ORGANIZER" || strings.HasPrefix(propUpper, "ORGANIZER;"):
			if email := extractMailto(value); email != "" {
				event.Organizer = email
			}
		case propUpper == "ATTENDEE" || strings.HasPrefix(propUpper, "ATTENDEE;"):
			if email := extractMailto(value); email != "" {
				event.Attendees = append(event.Attendees, email)
			}
		}
	}

	if !foundVEvent {
		return nil
	}
	return &event
}

// unfoldLines reverses RFC 5545 line folding: CRLF (or bare LF) followed by
// a single whitespace character is merged back into the preceding line.
// CRLF forms are handled first so that "\r\n " is consumed as a unit and does
// not leave a stray "\r" for the LF-only pass to mis-process.
func unfoldLines(s string) string {
	s = strings.ReplaceAll(s, "\r\n ", "")
	s = strings.ReplaceAll(s, "\r\n\t", "")
	// LF-only folding — produced by some mail servers that strip \r.
	s = strings.ReplaceAll(s, "\n ", "")
	s = strings.ReplaceAll(s, "\n\t", "")
	return s
}

// splitProperty splits "NAME;PARAMS:VALUE" into (name-with-params, value).
// It scans for the first colon that is not inside a double-quoted parameter
// value (e.g. CN="Doe: Jane"), per RFC 5545 §3.1.
func splitProperty(line string) (string, string) {
	inQuote := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			inQuote = !inQuote
		case ':':
			if !inQuote {
				return line[:i], line[i+1:]
			}
		}
	}
	return line, ""
}

// parseICSTime parses ICS datetime formats:
//   - 20260420T060000Z        (UTC)
//   - TZID=Asia/Shanghai:20260420T140000  (with timezone in property params)
//   - 20260420T140000         (local, treated as UTC)
func parseICSTime(value, propName string) time.Time {
	value = strings.TrimSpace(value)

	// Check for TZID in property params: DTSTART;TZID=Asia/Shanghai
	// Case-insensitive search (RFC 5545 §3.2 param names are case-insensitive).
	// Stop at the next ';' so trailing params like ;VALUE=DATE-TIME are excluded.
	if idx := strings.Index(strings.ToUpper(propName), "TZID="); idx >= 0 {
		tzPart := propName[idx+5:] // skip past "TZID="
		if end := strings.IndexByte(tzPart, ';'); end >= 0 {
			tzPart = tzPart[:end]
		}
		if loc, err := time.LoadLocation(tzPart); err == nil {
			if t, err := time.ParseInLocation("20060102T150405", value, loc); err == nil {
				return t
			}
		}
	}

	// UTC format: YYYYMMDDTHHMMSSZ
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t
	}

	// Date-only: YYYYMMDD (all-day events)
	if t, err := time.Parse("20060102", value); err == nil {
		return t
	}

	// Local time without timezone (treat as UTC)
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t
	}

	return time.Time{}
}

// unescapeTextValue reverses escapeTextValue per RFC 5545 §3.3.11, turning
// the ICS on-wire representation back into a plain Go string. Only applied
// to TEXT-typed properties (SUMMARY, LOCATION, DESCRIPTION, etc.) —
// identifiers, date-times, and URIs are parsed as-is.
func unescapeTextValue(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n', 'N':
				b.WriteByte('\n')
				i++
				continue
			case '\\', ';', ',':
				b.WriteByte(s[i+1])
				i++
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// extractMailto extracts the email address from an ICS ORGANIZER/ATTENDEE value.
// Accepts both "mailto:user@example.com" (RFC 5545 standard, case-insensitive per
// RFC 3986 §3.1) and a bare "user@example.com" value (observed in backend-regenerated
// ICS where the mailto: scheme prefix is dropped).
func extractMailto(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	if idx := strings.Index(lower, strings.ToLower(mailtoScheme)); idx >= 0 {
		return strings.TrimSpace(value[idx+len(mailtoScheme):])
	}
	if strings.Contains(value, "@") && !strings.ContainsAny(value, " \t") {
		return value
	}
	return ""
}
