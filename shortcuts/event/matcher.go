// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

// RawEventMatch captures the exact raw event type extracted from a normalized event.
type RawEventMatch struct {
	EventType string
	Matched   bool
}

// MatchRawEventType returns the exact event type when present on the normalized event.
func MatchRawEventType(evt *Event) (RawEventMatch, bool) {
	if evt == nil || evt.EventType == "" {
		return RawEventMatch{}, false
	}

	return RawEventMatch{
		EventType: evt.EventType,
		Matched:   true,
	}, true
}
