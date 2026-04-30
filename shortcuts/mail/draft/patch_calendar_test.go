// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

const fixtureCalData = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"

// ---------------------------------------------------------------------------
// set_calendar — validate
// ---------------------------------------------------------------------------

func TestSetCalendar_ValidateRequiresSummary(t *testing.T) {
	err := PatchOp{Op: "set_calendar", EventStart: "2026-04-25T10:00+08:00", EventEnd: "2026-04-25T11:00+08:00"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "event_summary") {
		t.Errorf("expected event_summary error, got %v", err)
	}
}

func TestSetCalendar_ValidateRequiresStartAndEnd(t *testing.T) {
	err := PatchOp{Op: "set_calendar", EventSummary: "Meeting", EventStart: "2026-04-25T10:00+08:00"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "event_start and event_end") {
		t.Errorf("expected start/end error, got %v", err)
	}
}

func TestSetCalendar_ValidateInvalidStartFormat(t *testing.T) {
	err := PatchOp{Op: "set_calendar", EventSummary: "M", EventStart: "not-a-date", EventEnd: "2026-04-25T11:00+08:00"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "event_start") {
		t.Errorf("expected event_start error for bad format, got %v", err)
	}
}

func TestSetCalendar_ValidateInvalidEndFormat(t *testing.T) {
	err := PatchOp{Op: "set_calendar", EventSummary: "M", EventStart: "2026-04-25T10:00+08:00", EventEnd: "not-a-date"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "event_end") {
		t.Errorf("expected event_end error for bad format, got %v", err)
	}
}

func TestSetCalendar_ValidateEndNotAfterStart(t *testing.T) {
	err := PatchOp{Op: "set_calendar", EventSummary: "M", EventStart: "2026-04-25T11:00+08:00", EventEnd: "2026-04-25T10:00+08:00"}.Validate()
	if err == nil || !strings.Contains(err.Error(), "after") {
		t.Errorf("expected end-after-start error, got %v", err)
	}
}

func TestSetCalendar_ValidateOK(t *testing.T) {
	err := PatchOp{
		Op:           "set_calendar",
		EventSummary: "Meeting",
		EventStart:   "2026-04-25T10:00+08:00",
		EventEnd:     "2026-04-25T11:00+08:00",
	}.Validate()
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// set_calendar — Apply adds text/calendar part when none exists
// ---------------------------------------------------------------------------

func TestSetCalendar_AddsCalendarPartToHTMLDraft(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Meeting
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:           "set_calendar",
			EventSummary: "Meeting",
			EventStart:   "2026-04-25T10:00+08:00",
			EventEnd:     "2026-04-25T11:00+08:00",
			CalendarICS:  []byte(fixtureCalData),
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	part := FindPartByMediaType(snapshot.Body, calendarMediaType)
	if part == nil {
		t.Fatal("text/calendar part not added to draft")
	}
	if string(part.Body) != fixtureCalData {
		t.Errorf("calendar part body mismatch: got %q", part.Body)
	}
	if part.MediaParams["method"] != "REQUEST" {
		t.Errorf("calendar part missing method=REQUEST in MediaParams: %v", part.MediaParams)
	}
}

// ---------------------------------------------------------------------------
// set_calendar — Apply replaces existing text/calendar part
// ---------------------------------------------------------------------------

func TestSetCalendar_ReplacesExistingCalendarPart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Meeting
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="b1"

--b1
Content-Type: text/html; charset=UTF-8

<p>Hello</p>
--b1
Content-Type: text/calendar; charset=UTF-8

BEGIN:VCALENDAR
VERSION:2.0
SUMMARY:OLD
END:VCALENDAR
--b1--`)

	newICS := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nSUMMARY:NEW\r\nEND:VCALENDAR\r\n")
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:           "set_calendar",
			EventSummary: "NEW",
			EventStart:   "2026-04-25T10:00+08:00",
			EventEnd:     "2026-04-25T11:00+08:00",
			CalendarICS:  newICS,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	part := FindPartByMediaType(snapshot.Body, calendarMediaType)
	if part == nil {
		t.Fatal("text/calendar part missing")
	}
	if !strings.Contains(string(part.Body), "SUMMARY:NEW") {
		t.Errorf("expected new SUMMARY, got %q", part.Body)
	}
	if strings.Contains(string(part.Body), "SUMMARY:OLD") {
		t.Errorf("old SUMMARY not replaced")
	}
}

// ---------------------------------------------------------------------------
// set_calendar — Apply requires pre-built ICS
// ---------------------------------------------------------------------------

func TestSetCalendar_EmptyICSIsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Meeting
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:           "set_calendar",
			EventSummary: "Meeting",
			EventStart:   "2026-04-25T10:00+08:00",
			EventEnd:     "2026-04-25T11:00+08:00",
			// CalendarICS intentionally nil — simulates missing pre-process.
		}},
	})
	if err == nil {
		t.Fatal("expected error for missing CalendarICS")
	}
	if !strings.Contains(err.Error(), "ICS data is empty") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// remove_calendar
// ---------------------------------------------------------------------------

func TestRemoveCalendar_StripsCalendarPart(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Meeting
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/alternative; boundary="b1"

--b1
Content-Type: text/html; charset=UTF-8

<p>Hello</p>
--b1
Content-Type: text/calendar; charset=UTF-8

BEGIN:VCALENDAR
END:VCALENDAR
--b1--`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_calendar"}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if part := FindPartByMediaType(snapshot.Body, calendarMediaType); part != nil {
		t.Errorf("text/calendar part should be removed, but still found")
	}
}

func TestRemoveCalendar_NoOpWhenAbsent(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Plain
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/html; charset=UTF-8

<p>Hello</p>`)

	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_calendar"}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Body remains intact.
	if snapshot.Body == nil {
		t.Fatal("body unexpectedly nil")
	}
}

// ---------------------------------------------------------------------------
// Internal MIME helpers (coverage)
// ---------------------------------------------------------------------------

func TestFindPartByMediaType_CaseInsensitive(t *testing.T) {
	root := &Part{
		MediaType: "multipart/mixed",
		Children: []*Part{
			{MediaType: "TEXT/Calendar"},
		},
	}
	got := FindPartByMediaType(root, "text/calendar")
	if got == nil {
		t.Fatal("expected to find part despite case mismatch")
	}
}

func TestRemovePartByMediaType_MarksParentDirty(t *testing.T) {
	root := &Part{
		MediaType: "multipart/mixed",
		Children: []*Part{
			{MediaType: "text/calendar"},
			{MediaType: "text/html"},
		},
	}
	removePartByMediaType(root, "text/calendar")
	if len(root.Children) != 1 {
		t.Fatalf("expected 1 remaining child, got %d", len(root.Children))
	}
	if !root.Dirty {
		t.Error("parent not marked dirty after removal")
	}
}

func TestSetCalendar_CollapsesToOneInsideAlternative(t *testing.T) {
	// Feishu client creates two text/calendar copies: one inside
	// multipart/alternative and one as an inline attachment in
	// multipart/mixed. set_calendar must collapse them to a single
	// copy inside multipart/alternative.
	snapshot := mustParseFixtureDraft(t, `Subject: Meeting
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/alternative; boundary="inner"

--inner
Content-Type: text/html; charset=UTF-8

<p>Hello</p>
--inner
Content-Type: text/calendar; charset=UTF-8

BEGIN:VCALENDAR
SUMMARY:OLD
END:VCALENDAR
--inner--
--outer
Content-Type: text/calendar; charset=UTF-8; name="invite.ics"
Content-Id: <invite.ics>

BEGIN:VCALENDAR
SUMMARY:OLD
END:VCALENDAR
--outer--`)

	newICS := []byte("BEGIN:VCALENDAR\r\nSUMMARY:NEW\r\nEND:VCALENDAR\r\n")
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:           "set_calendar",
			EventSummary: "NEW",
			EventStart:   "2026-04-25T10:00+08:00",
			EventEnd:     "2026-04-25T11:00+08:00",
			CalendarICS:  newICS,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Exactly one text/calendar part should remain, inside alternative.
	parts := findAllPartsByMediaType(snapshot.Body, calendarMediaType)
	if len(parts) != 1 {
		t.Fatalf("expected 1 text/calendar part, got %d", len(parts))
	}
	if !strings.Contains(string(parts[0].Body), "SUMMARY:NEW") {
		t.Errorf("expected SUMMARY:NEW, got %q", parts[0].Body)
	}

	// The calendar part must be a child of multipart/alternative.
	alt := FindPartByMediaType(snapshot.Body, "multipart/alternative")
	if alt == nil {
		t.Fatal("multipart/alternative not found")
	}
	found := false
	for _, child := range alt.Children {
		if strings.EqualFold(child.MediaType, calendarMediaType) {
			found = true
		}
	}
	if !found {
		t.Error("text/calendar part not inside multipart/alternative")
	}
}

func TestRemoveCalendar_RootLevelCalendarBody(t *testing.T) {
	// When the snapshot body is itself a text/calendar leaf (no multipart
	// wrapper), removeCalendarPart must nil out snapshot.Body rather than
	// trying to remove it from a parent's children slice.
	snapshot := &DraftSnapshot{
		Body: &Part{
			MediaType: "text/calendar",
			Body:      []byte(fixtureCalData),
		},
	}
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_calendar"}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if snapshot.Body != nil {
		t.Errorf("snapshot.Body should be nil after removing root-level text/calendar, got %+v", snapshot.Body)
	}
}

func TestSetCalendarPart_OnNilBodyCreatesLeaf(t *testing.T) {
	snapshot := &DraftSnapshot{}
	setCalendarPart(snapshot, []byte(fixtureCalData))
	if snapshot.Body == nil {
		t.Fatal("body should be created")
	}
	if !strings.EqualFold(snapshot.Body.MediaType, calendarMediaType) {
		t.Errorf("expected %s leaf, got %s", calendarMediaType, snapshot.Body.MediaType)
	}
}

func TestSetCalendarPart_MixedWithoutAlternativeWrapsTextChild(t *testing.T) {
	// multipart/mixed with a text/html child but no alternative sub-part.
	// setCalendarPart should wrap the text/html in a new alternative.
	snapshot := &DraftSnapshot{
		Body: &Part{
			MediaType: "multipart/mixed",
			Children: []*Part{
				{MediaType: "text/html", Body: []byte("<p>Hi</p>")},
				{MediaType: "application/pdf", Body: []byte("pdf-data")},
			},
		},
	}
	setCalendarPart(snapshot, []byte(fixtureCalData))

	if snapshot.Body.MediaType != "multipart/mixed" {
		t.Fatalf("root should stay multipart/mixed, got %s", snapshot.Body.MediaType)
	}
	alt := FindPartByMediaType(snapshot.Body, "multipart/alternative")
	if alt == nil {
		t.Fatal("expected a multipart/alternative child to be created")
	}
	if len(alt.Children) != 2 {
		t.Fatalf("alternative should have 2 children, got %d", len(alt.Children))
	}
	if !strings.EqualFold(alt.Children[0].MediaType, "text/html") {
		t.Errorf("first alternative child should be text/html, got %s", alt.Children[0].MediaType)
	}
	if !strings.EqualFold(alt.Children[1].MediaType, calendarMediaType) {
		t.Errorf("second alternative child should be text/calendar, got %s", alt.Children[1].MediaType)
	}
}

func TestSetCalendarPart_FallbackAppendsToMultipart(t *testing.T) {
	// multipart/mixed with only non-text children (no text/* to wrap).
	snapshot := &DraftSnapshot{
		Body: &Part{
			MediaType: "multipart/mixed",
			Children: []*Part{
				{MediaType: "application/pdf", Body: []byte("pdf-data")},
			},
		},
	}
	setCalendarPart(snapshot, []byte(fixtureCalData))

	found := false
	for _, child := range snapshot.Body.Children {
		if strings.EqualFold(child.MediaType, calendarMediaType) {
			found = true
		}
	}
	if !found {
		t.Error("text/calendar should be appended as fallback child")
	}
}
