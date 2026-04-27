// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ics

import (
	"strings"
	"testing"
	"time"
)

func TestBuild_Basic(t *testing.T) {
	event := Event{
		UID:       "test-uid-123",
		Summary:   "Product Review",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Name: "Sender", Email: "sender@example.com"},
		Attendees: []Address{
			{Name: "Alice", Email: "alice@example.com"},
			{Name: "Bob", Email: "bob@example.com"},
		},
	}
	// Unfold before assertion so long property lines (which exceed 75 octets and
	// are folded per RFC 5545) can be matched as a single contiguous string.
	ics := unfoldLines(string(Build(event)))

	checks := []string{
		"BEGIN:VCALENDAR",
		"CALSCALE:GREGORIAN",
		"VERSION:2.0",
		"METHOD:REQUEST",
		"X-LARK-MAIL-DRAFT:TRUE",
		"BEGIN:VEVENT",
		"UID:test-uid-123",
		"DTSTAMP:",
		"CREATED:",
		"LAST-MODIFIED:",
		"DTSTART:20260420T060000Z",
		"DTEND:20260420T070000Z",
		"SUMMARY:Product Review",
		"STATUS:CONFIRMED",
		"TRANSP:OPAQUE",
		"SEQUENCE:0",
		"ORGANIZER;ROLE=CHAIR;CN=Sender:MAILTO:sender@example.com",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CUTYPE=INDIVIDUAL;CN=Alice;PARTSTAT=NEEDS-ACTION:MAILTO:alice@example.com",
		"ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CUTYPE=INDIVIDUAL;CN=Bob;PARTSTAT=NEEDS-ACTION:MAILTO:bob@example.com",
		"END:VEVENT",
		"END:VCALENDAR",
	}
	for _, want := range checks {
		if !strings.Contains(ics, want) {
			t.Errorf("missing %q in ICS:\n%s", want, ics)
		}
	}
}

func TestBuild_OrganizerFallsBackToEmailWhenNoName(t *testing.T) {
	event := Event{
		Summary:   "Meeting",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Email: "o@e.com"},
		Attendees: []Address{{Email: "a@e.com"}},
	}
	ics := unfoldLines(string(Build(event)))
	if !strings.Contains(ics, "ORGANIZER;ROLE=CHAIR;CN=o@e.com:MAILTO:o@e.com") {
		t.Errorf("ORGANIZER without name should fall back to email as CN:\n%s", ics)
	}
	if !strings.Contains(ics, "ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CUTYPE=INDIVIDUAL;CN=a@e.com;PARTSTAT=NEEDS-ACTION:MAILTO:a@e.com") {
		t.Errorf("ATTENDEE without name should fall back to email as CN:\n%s", ics)
	}
}

func TestBuild_WithLocation(t *testing.T) {
	event := Event{
		Summary:  "Meeting",
		Location: "5F Conference Room",
		Start:    time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	ics := string(Build(event))
	if !strings.Contains(ics, "LOCATION:5F Conference Room") {
		t.Errorf("missing LOCATION in ICS:\n%s", ics)
	}
}

func TestBuild_NoLocation(t *testing.T) {
	event := Event{
		Summary: "Meeting",
		Start:   time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	ics := string(Build(event))
	if strings.Contains(ics, "LOCATION") {
		t.Errorf("should not have LOCATION when empty:\n%s", ics)
	}
}

func TestBuild_AutoUIDIsPureUUID(t *testing.T) {
	event := Event{
		Summary: "Test",
		Start:   time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	ics := string(Build(event))
	if !strings.Contains(ics, "UID:") {
		t.Fatal("missing UID")
	}
	// Extract the UID line to assert on its format.
	var uid string
	for _, line := range strings.Split(ics, "\r\n") {
		if strings.HasPrefix(line, "UID:") {
			uid = strings.TrimPrefix(line, "UID:")
			break
		}
	}
	if strings.Contains(uid, "@") {
		t.Errorf("auto-generated UID should be pure UUID (no @host suffix), got %q", uid)
	}
	// UUID v4 has 36 chars (8-4-4-4-12 plus 4 dashes).
	if len(uid) != 36 {
		t.Errorf("auto-generated UID should be 36-char UUID, got %d chars: %q", len(uid), uid)
	}
}

func TestBuild_EscapesTextValues(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"semicolon", "a;b", `a\;b`},
		{"comma", "a,b", `a\,b`},
		{"backslash", `a\b`, `a\\b`},
		{"newline", "a\nb", `a\nb`},
		{"crlf", "a\r\nb", `a\nb`},
		{"mixed", `a;\,b` + "\n", `a\;\\\,b\n`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapeTextValue(tc.input); got != tc.want {
				t.Errorf("escapeTextValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestBuild_RejectsInjectionViaSummary proves that a malicious SUMMARY
// containing a newline plus a fake property line cannot inject a second
// DTSTART into the rendered ICS — the newline is escaped into a literal
// "\n" sequence inside the SUMMARY value.
func TestBuild_RejectsInjectionViaSummary(t *testing.T) {
	event := Event{
		Summary: "harmless\nDTSTART:19700101T000000Z",
		Start:   time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	ics := unfoldLines(string(Build(event)))

	// Count occurrences of DTSTART at the start of a line (i.e., as an
	// actual property), ignoring the literal "DTSTART:" substring that
	// now appears inside the escaped SUMMARY value.
	dtstartPropertyLines := 0
	for _, line := range strings.Split(ics, "\r\n") {
		if strings.HasPrefix(line, "DTSTART:") {
			dtstartPropertyLines++
		}
	}
	if dtstartPropertyLines != 1 {
		t.Errorf("expected exactly one DTSTART: property line, got %d in:\n%s", dtstartPropertyLines, ics)
	}
	if !strings.Contains(ics, `SUMMARY:harmless\nDTSTART:19700101T000000Z`) {
		t.Errorf("expected escaped SUMMARY to contain literal \\n, got:\n%s", ics)
	}
}

func TestBuild_CNWithSpecialCharsIsQuoted(t *testing.T) {
	event := Event{
		Summary:   "Meeting",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Name: "Smith, Alice", Email: "alice@example.com"},
		Attendees: []Address{
			{Name: "Doe; Bob", Email: "bob@example.com"},
			{Name: "Plain Name", Email: "plain@example.com"},
		},
	}
	ics := unfoldLines(string(Build(event)))
	if !strings.Contains(ics, `CN="Smith, Alice"`) {
		t.Errorf("expected quoted CN for organizer name with comma:\n%s", ics)
	}
	if !strings.Contains(ics, `CN="Doe; Bob"`) {
		t.Errorf("expected quoted CN for attendee name with semicolon:\n%s", ics)
	}
	// Names without special chars must NOT be double-quoted.
	if !strings.Contains(ics, "CN=Plain Name") || strings.Contains(ics, `CN="Plain Name"`) {
		t.Errorf("plain name should be unquoted:\n%s", ics)
	}
}

func TestBuild_EmailAddressSanitized(t *testing.T) {
	// CR/LF inside an email address must not produce injected property lines.
	event := Event{
		Summary:   "Meeting",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Name: "Alice", Email: "alice@example.com\r\nX-INJECTED:bad"},
		Attendees: []Address{{Name: "Bob", Email: "bob@example.com\nY-INJECTED:bad"}},
	}
	output := string(Build(event))
	if strings.Contains(output, "\r\nX-INJECTED") {
		t.Error("organizer email CR/LF injection not sanitized")
	}
	if strings.Contains(output, "\r\nY-INJECTED") {
		t.Error("attendee email CR/LF injection not sanitized")
	}
}

func TestBuild_CNStripsControlChars(t *testing.T) {
	// A display name containing CR, LF, or other control characters must not
	// produce extra ICS property lines (injection via CN parameter).
	event := Event{
		Summary:   "Meeting",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Name: "Alice\r\nDTSTART:99999999", Email: "alice@example.com"},
		Attendees: []Address{
			{Name: "Bob\nX-INJECTED:bad", Email: "bob@example.com"},
		},
	}
	output := string(Build(event))
	// Check that control chars don't produce injected property lines.
	// A standalone ICS property line starts at the beginning of a CRLF-delimited line.
	if strings.Contains(output, "\r\nDTSTART:99999999") {
		t.Error("ICS output contains injected DTSTART property line via organizer CN")
	}
	if strings.Contains(output, "\r\nX-INJECTED") {
		t.Error("ICS output contains injected X-INJECTED property line via attendee CN")
	}
}

func TestBuild_LineFolding(t *testing.T) {
	event := Event{
		Summary: strings.Repeat("A", 100), // long summary triggers folding
		Start:   time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:     time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	ics := string(Build(event))
	// Every physical line (first line and continuation lines alike) must be
	// ≤ 75 octets excluding the CRLF terminator per RFC 5545 §3.1.
	for _, line := range strings.Split(ics, "\r\n") {
		if len(line) > 75 {
			t.Errorf("line exceeds 75 octets: %q (len=%d)", line, len(line))
		}
	}
}

func TestParseEvent_Basic(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"METHOD:REQUEST\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:abc123@larksuite.com\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:Product Review\r\n" +
		"LOCATION:5F Room\r\n" +
		"ORGANIZER;CN=Sender:mailto:sender@example.com\r\n" +
		"ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CN=Alice:mailto:alice@example.com\r\n" +
		"ATTENDEE;ROLE=REQ-PARTICIPANT;RSVP=TRUE;CN=Bob:mailto:bob@example.com\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if event.Method != "REQUEST" {
		t.Errorf("Method = %q, want REQUEST", event.Method)
	}
	if event.IsLarkDraft {
		t.Error("IsLarkDraft = true, want false (no X-LARK-MAIL-DRAFT in input)")
	}
	if event.UID != "abc123@larksuite.com" {
		t.Errorf("UID = %q, want abc123@larksuite.com", event.UID)
	}
	if event.Summary != "Product Review" {
		t.Errorf("Summary = %q, want Product Review", event.Summary)
	}
	if event.Location != "5F Room" {
		t.Errorf("Location = %q, want 5F Room", event.Location)
	}
	if event.Organizer != "sender@example.com" {
		t.Errorf("Organizer = %q, want sender@example.com", event.Organizer)
	}
	if len(event.Attendees) != 2 {
		t.Fatalf("Attendees count = %d, want 2", len(event.Attendees))
	}
	if event.Attendees[0] != "alice@example.com" {
		t.Errorf("Attendees[0] = %q, want alice@example.com", event.Attendees[0])
	}
	if event.Attendees[1] != "bob@example.com" {
		t.Errorf("Attendees[1] = %q, want bob@example.com", event.Attendees[1])
	}
	wantStart := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	if !event.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", event.Start, wantStart)
	}
	wantEnd := time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC)
	if !event.End.Equal(wantEnd) {
		t.Errorf("End = %v, want %v", event.End, wantEnd)
	}
}

func TestParseEvent_IsLarkDraft(t *testing.T) {
	icsWithMarker := "BEGIN:VCALENDAR\r\n" +
		"METHOD:REQUEST\r\n" +
		"X-LARK-MAIL-DRAFT:TRUE\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:draft-test\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:Draft Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	event := ParseEvent(icsWithMarker)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if !event.IsLarkDraft {
		t.Error("IsLarkDraft = false, want true")
	}

	icsWithoutMarker := "BEGIN:VCALENDAR\r\n" +
		"METHOD:REQUEST\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:external-test\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:External Event\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	event2 := ParseEvent(icsWithoutMarker)
	if event2 == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if event2.IsLarkDraft {
		t.Error("IsLarkDraft = true, want false")
	}
}

func TestParseEvent_WithTZID(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:tz-test\r\n" +
		"DTSTART;TZID=Asia/Shanghai:20260420T140000\r\n" +
		"DTEND;TZID=Asia/Shanghai:20260420T150000\r\n" +
		"SUMMARY:TZ Test\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	// 14:00 Asia/Shanghai = 06:00 UTC
	wantStart := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	if !event.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", event.Start, wantStart)
	}
}

func TestParseEvent_FoldedLines(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:fold-test\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:This is a very long summary that should be unfolded correctly by th\r\n" +
		" e parser when processing\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	want := "This is a very long summary that should be unfolded correctly by the parser when processing"
	if event.Summary != want {
		t.Errorf("Summary = %q, want %q", event.Summary, want)
	}
}

func TestParseEvent_FoldedLines_LFOnly(t *testing.T) {
	// Some mail servers strip \r before storage, producing LF-only ICS.
	ics := "BEGIN:VCALENDAR\n" +
		"BEGIN:VEVENT\n" +
		"UID:lf-fold-test\n" +
		"DTSTART:20260420T060000Z\n" +
		"DTEND:20260420T070000Z\n" +
		"SUMMARY:This is a very long summary that should be unfolded correctly by th\n" +
		" e parser when LF-only folding is used\n" +
		"END:VEVENT\n" +
		"END:VCALENDAR\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil for LF-only ICS")
	}
	want := "This is a very long summary that should be unfolded correctly by the parser when LF-only folding is used"
	if event.Summary != want {
		t.Errorf("Summary = %q, want %q", event.Summary, want)
	}
}

func TestParseEvent_NoVEvent(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"
	event := ParseEvent(ics)
	if event != nil {
		t.Error("expected nil for ICS without VEVENT")
	}
}

// TestParseEvent_OrganizerWithoutMailto covers the case where the backend
// re-serializes our ICS and drops the "MAILTO:" scheme prefix. Observed in
// practice on drafts returned by user_mailboxes/me/drafts.get.
func TestParseEvent_OrganizerWithoutMailto(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:no-mailto-test\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:Test\r\n" +
		"ORGANIZER;CN=org@example.com:org@example.com\r\n" +
		"ATTENDEE;PARTSTAT=NEEDS-ACTION;CN=att@example.com:att@example.com\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if event.Organizer != "org@example.com" {
		t.Errorf("Organizer = %q, want org@example.com (parser must accept bare email when mailto: is absent)", event.Organizer)
	}
	if len(event.Attendees) != 1 || event.Attendees[0] != "att@example.com" {
		t.Errorf("Attendees = %v, want [att@example.com]", event.Attendees)
	}
}

func TestParseEvent_MailtoCaseInsensitive(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:case-test\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:Test\r\n" +
		"ORGANIZER;CN=Sender:MAILTO:sender@example.com\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if event.Organizer != "sender@example.com" {
		t.Errorf("Organizer = %q, want sender@example.com (uppercase MAILTO: should be accepted)", event.Organizer)
	}
}

func TestParseEvent_RecurrenceIDPopulatesOriginalTime(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:recurring-exception\r\n" +
		"DTSTART:20260501T020000Z\r\n" +
		"DTEND:20260501T030000Z\r\n" +
		"RECURRENCE-ID:20260501T020000Z\r\n" +
		"SUMMARY:Exception instance\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	// 2026-05-01 02:00:00 UTC = 1777600800
	if event.OriginalTime != 1777600800 {
		t.Errorf("OriginalTime = %d, want 1777600800", event.OriginalTime)
	}
}

func TestParseEvent_NoRecurrenceIDYieldsZero(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:single-event\r\n" +
		"DTSTART:20260420T060000Z\r\n" +
		"DTEND:20260420T070000Z\r\n" +
		"SUMMARY:Single\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
	event := ParseEvent(ics)
	if event == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if event.OriginalTime != 0 {
		t.Errorf("OriginalTime = %d, want 0 for non-recurring event", event.OriginalTime)
	}
}

func TestUnescapeTextValue(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"semicolon", `a\;b`, "a;b"},
		{"comma", `a\,b`, "a,b"},
		{"backslash", `a\\b`, `a\b`},
		{"newline_lower", `a\nb`, "a\nb"},
		{"newline_upper", `a\Nb`, "a\nb"},
		{"mixed", `a\;\\\,b\n`, "a;\\,b\n"},
		{"dangling_backslash_kept", `ends\`, `ends\`},
		{"unknown_escape_kept", `\x`, `\x`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := unescapeTextValue(tc.input); got != tc.want {
				t.Errorf("unescapeTextValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRoundTrip_SpecialCharsInSummaryAndLocation(t *testing.T) {
	event := Event{
		UID:      "rt-special",
		Summary:  `Review;with,special\chars` + "\n" + `and newline`,
		Location: `B1,Room 3;floor 2`,
		Start:    time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
	}
	parsed := ParseEvent(string(Build(event)))
	if parsed == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if !parsed.IsLarkDraft {
		t.Error("IsLarkDraft = false after roundtrip, want true (Build should write X-LARK-MAIL-DRAFT)")
	}
	if parsed.Summary != event.Summary {
		t.Errorf("Summary roundtrip: got %q, want %q", parsed.Summary, event.Summary)
	}
	if parsed.Location != event.Location {
		t.Errorf("Location roundtrip: got %q, want %q", parsed.Location, event.Location)
	}
}

func TestBuild_WriteFolded_SingleCharExceeds75Bytes(t *testing.T) {
	// A single multibyte rune that is > 75 bytes is not reachable in practice,
	// but we exercise the cut==0 fallback by constructing a fake line via a
	// 75-octet name followed by a multi-octet rune that crosses the boundary.
	// The simplest way: a name of exactly 74 chars + ':' = 75, then a multi-byte
	// rune — the first iteration has cut==0, triggering the fallback.
	var b strings.Builder
	longName := strings.Repeat("A", 74)
	// value starts with a 3-byte UTF-8 rune (€ = 0xE2 0x82 0xAC)
	writeFolded(&b, longName, "€remainder")
	result := b.String()
	if !strings.Contains(result, "\r\n ") {
		t.Errorf("expected line folding CRLF+SP in output:\n%q", result)
	}
}

func TestSplitProperty_NoColon(t *testing.T) {
	name, value := splitProperty("NOCOLON")
	if name != "NOCOLON" || value != "" {
		t.Errorf("splitProperty(no colon): got name=%q value=%q, want NOCOLON/\"\"", name, value)
	}
}

func TestSplitProperty_QuotedColon(t *testing.T) {
	// A colon inside a quoted CN param must not be treated as the separator.
	name, value := splitProperty(`ORGANIZER;CN="Doe: Jane":mailto:alice@example.com`)
	if name != `ORGANIZER;CN="Doe: Jane"` {
		t.Errorf("name = %q, want ORGANIZER;CN=\"Doe: Jane\"", name)
	}
	if value != "mailto:alice@example.com" {
		t.Errorf("value = %q, want mailto:alice@example.com", value)
	}
}

func TestParseICSTime_TZIDCaseInsensitive(t *testing.T) {
	// TZID parameter name is case-insensitive per RFC 5545 §3.2.
	result := parseICSTime("20260420T140000", "DTSTART;tzid=Asia/Shanghai")
	want := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	if !result.Equal(want) {
		t.Errorf("parseICSTime with lowercase tzid= = %v, want %v", result, want)
	}
}

func TestParseICSTime_TZIDWithTrailingParam(t *testing.T) {
	// Trailing parameters after TZID (e.g. ;VALUE=DATE-TIME) must not be
	// included in the timezone name passed to time.LoadLocation.
	result := parseICSTime("20260420T140000", "DTSTART;TZID=Asia/Shanghai;VALUE=DATE-TIME")
	want := time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC)
	if !result.Equal(want) {
		t.Errorf("parseICSTime with trailing ;VALUE= = %v, want %v", result, want)
	}
}

func TestParseICSTime_DateOnly(t *testing.T) {
	// All-day event: YYYYMMDD format
	result := parseICSTime("20260420", "DTSTART;VALUE=DATE")
	want := time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	if !result.Equal(want) {
		t.Errorf("parseICSTime date-only = %v, want %v", result, want)
	}
}

func TestParseICSTime_LocalWithoutTZ(t *testing.T) {
	// Local time without timezone suffix (no Z, no TZID) — treated as UTC
	result := parseICSTime("20260420T140000", "DTSTART")
	want := time.Date(2026, 4, 20, 14, 0, 0, 0, time.UTC)
	if !result.Equal(want) {
		t.Errorf("parseICSTime local = %v, want %v", result, want)
	}
}

func TestParseICSTime_InvalidReturnsZero(t *testing.T) {
	result := parseICSTime("not-a-date", "DTSTART")
	if !result.IsZero() {
		t.Errorf("parseICSTime invalid = %v, want zero", result)
	}
}

func TestExtractMailto_NoAt(t *testing.T) {
	result := extractMailto("notanemail")
	if result != "" {
		t.Errorf("extractMailto(no @) = %q, want empty", result)
	}
}

func TestRoundTrip(t *testing.T) {
	original := Event{
		UID:       "roundtrip-test",
		Summary:   "Roundtrip Meeting",
		Location:  "Room 301",
		Start:     time.Date(2026, 4, 20, 6, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 4, 20, 7, 0, 0, 0, time.UTC),
		Organizer: Address{Name: "Sender", Email: "sender@example.com"},
		Attendees: []Address{
			{Name: "Alice", Email: "alice@example.com"},
		},
	}
	icsBytes := Build(original)
	parsed := ParseEvent(string(icsBytes))
	if parsed == nil {
		t.Fatal("ParseEvent returned nil on Build output")
	}
	if parsed.UID != original.UID {
		t.Errorf("UID roundtrip: %q != %q", parsed.UID, original.UID)
	}
	if parsed.Summary != original.Summary {
		t.Errorf("Summary roundtrip: %q != %q", parsed.Summary, original.Summary)
	}
	if parsed.Location != original.Location {
		t.Errorf("Location roundtrip: %q != %q", parsed.Location, original.Location)
	}
	if !parsed.Start.Equal(original.Start) {
		t.Errorf("Start roundtrip: %v != %v", parsed.Start, original.Start)
	}
	if parsed.Organizer != original.Organizer.Email {
		t.Errorf("Organizer roundtrip: %q != %q", parsed.Organizer, original.Organizer.Email)
	}
	if len(parsed.Attendees) != 1 || parsed.Attendees[0] != "alice@example.com" {
		t.Errorf("Attendees roundtrip: %v", parsed.Attendees)
	}
}

func TestParseEvent_LowercaseAndParameterizedProps(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" +
		"uid:lowercased-uid-value\r\n" +
		"SUMMARY;LANGUAGE=en-US:Team Sync\r\n" +
		"location;ALTREP=\"cid:part1\":Room 301\r\n" +
		"DTSTART:20260501T100000Z\r\n" +
		"DTEND:20260501T110000Z\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	ev := ParseEvent(ics)
	if ev == nil {
		t.Fatal("ParseEvent returned nil")
	}
	if ev.UID != "lowercased-uid-value" {
		t.Errorf("UID: got %q", ev.UID)
	}
	if ev.Summary != "Team Sync" {
		t.Errorf("Summary: got %q", ev.Summary)
	}
	if ev.Location != "Room 301" {
		t.Errorf("Location: got %q", ev.Location)
	}
}

func TestParseEvent_StartEndUTCInOutput(t *testing.T) {
	// Verify that times with TZID are parsed with correct offset
	// (UTC normalization in output is done by the helpers layer; parser
	// returns time.Time which callers can call .UTC() on).
	ics := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\n" +
		"DTSTART;TZID=Asia/Shanghai:20260501T180000\r\n" +
		"DTEND;TZID=Asia/Shanghai:20260501T190000\r\n" +
		"END:VEVENT\r\nEND:VCALENDAR\r\n"
	ev := ParseEvent(ics)
	if ev == nil {
		t.Fatal("ParseEvent returned nil")
	}
	wantStart := "2026-05-01T10:00:00Z"
	if got := ev.Start.UTC().Format(time.RFC3339); got != wantStart {
		t.Errorf("Start UTC: got %q, want %q", got, wantStart)
	}
}
