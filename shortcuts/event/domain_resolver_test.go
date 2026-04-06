// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import "testing"

func TestMatchRawEventTypeReturnsExactEventType(t *testing.T) {
	evt := &Event{EventType: "im.message.receive_v1"}

	match, ok := MatchRawEventType(evt)
	if !ok {
		t.Fatal("MatchRawEventType returned ok=false, want true")
	}
	if !match.Matched {
		t.Fatal("Matched = false, want true")
	}
	if match.EventType != "im.message.receive_v1" {
		t.Fatalf("EventType = %q, want im.message.receive_v1", match.EventType)
	}
}

func TestMatchRawEventTypeReturnsFalseForNilOrEmptyEventType(t *testing.T) {
	tests := []struct {
		name string
		evt  *Event
	}{
		{name: "nil event", evt: nil},
		{name: "empty event type", evt: &Event{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, ok := MatchRawEventType(tt.evt)
			if ok {
				t.Fatal("MatchRawEventType returned ok=true, want false")
			}
			if match != (RawEventMatch{}) {
				t.Fatalf("match = %+v, want zero value", match)
			}
		})
	}
}

func TestResolveDomainKnownMappings(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      string
	}{
		{name: "im", eventType: "im.message.receive_v1", want: "im"},
		{name: "base", eventType: "base.record.created_v1", want: "base"},
		{name: "bitable aliases to base", eventType: "bitable.record.updated_v1", want: "base"},
		{name: "docs", eventType: "docs.document.created_v1", want: "docs"},
		{name: "docx aliases to docs", eventType: "docx.document.updated_v1", want: "docs"},
		{name: "drive aliases to docs", eventType: "drive.file.created_v1", want: "docs"},
		{name: "calendar", eventType: "calendar.event.created_v4", want: "calendar"},
		{name: "task", eventType: "task.task.updated_v1", want: "task"},
		{name: "contact", eventType: "contact.user.created_v3", want: "contact"},
		{name: "vc", eventType: "vc.meeting.started_v1", want: "vc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDomain(&Event{EventType: tt.eventType})
			if got != tt.want {
				t.Fatalf("ResolveDomain(%q) = %q, want %q", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestResolveDomainReturnsUnknownForNilOrUnknownEventType(t *testing.T) {
	tests := []struct {
		name string
		evt  *Event
	}{
		{name: "nil event", evt: nil},
		{name: "empty event type", evt: &Event{}},
		{name: "unknown prefix", evt: &Event{EventType: "approval.instance.created_v1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveDomain(tt.evt); got != DomainUnknown {
				t.Fatalf("ResolveDomain() = %q, want %q", got, DomainUnknown)
			}
		})
	}
}
