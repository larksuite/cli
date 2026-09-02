// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"encoding/json"
	"testing"

	event "github.com/larksuite/cli/internal/event"
)

func whiteboardEvent(payload string) *event.RawEvent {
	return &event.RawEvent{
		EventID:   "evt-1",
		EventType: eventTypeWhiteboardUpdated,
		Payload:   json.RawMessage(payload),
	}
}

func envelopeFor(whiteboardID string) string {
	return `{"schema":"2.0","header":{"event_type":"` + eventTypeWhiteboardUpdated +
		`"},"event":{"whiteboard_id":"` + whiteboardID + `","operator_ids":[]}}`
}

// The filter must exist on the key, not just as a function: the server-side
// subscription is per whiteboard but the local bus fans out by event type, so
// a key without this filter hands every subscribed whiteboard's events to
// every consumer.
func TestWhiteboardKey_DeclaresTheBoardFilter(t *testing.T) {
	defs := Keys()
	if len(defs) != 1 {
		t.Fatalf("expected exactly one whiteboard key, got %d", len(defs))
	}
	if defs[0].Match == nil {
		t.Fatal("the whiteboard key must declare Match; without it consumers of different boards see each other's events")
	}
}

// The pollution this closes: two consumers of different boards on one bus.
func TestWhiteboardMatch_KeepsEachConsumerToItsOwnBoard(t *testing.T) {
	params := map[string]string{"whiteboard_id": "board-A"}

	if !whiteboardIDMatch(whiteboardEvent(envelopeFor("board-A")), params) {
		t.Error("an event for the requested board must be delivered")
	}
	if whiteboardIDMatch(whiteboardEvent(envelopeFor("board-B")), params) {
		t.Error("an event for another board must be dropped; delivering it is the cross-board pollution")
	}
}

// An event whose board cannot be read is dropped rather than delivered: a
// consumer that asked for one board must not be handed an event that cannot
// be attributed to it.
func TestWhiteboardMatch_DropsUnattributableEvents(t *testing.T) {
	params := map[string]string{"whiteboard_id": "board-A"}
	cases := map[string]string{
		"absent id":          `{"schema":"2.0","event":{"operator_ids":[]}}`,
		"id is not string":   `{"schema":"2.0","event":{"whiteboard_id":42}}`,
		"event not object":   `{"schema":"2.0","event":"board-A"}`,
		"payload not json":   `definitely not json {{{`,
		"payload not object": `["board-A"]`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if whiteboardIDMatch(whiteboardEvent(payload), params) {
				t.Error("an event whose board cannot be read must be dropped")
			}
		})
	}
}

// Defensive: the parameter is declared required and validated upstream, but an
// empty request must not degrade into "deliver everything".
func TestWhiteboardMatch_DropsWhenNoBoardWasRequested(t *testing.T) {
	if whiteboardIDMatch(whiteboardEvent(envelopeFor("board-A")), map[string]string{}) {
		t.Error("without a requested board there is nothing to attribute an event to; it must be dropped")
	}
}
