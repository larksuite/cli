// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"encoding/json"

	event "github.com/larksuite/cli/internal/event"
)

// whiteboardIDMatch keeps a consumer's stream to the whiteboard it asked for.
//
// The server-side subscription is registered per whiteboard, but the local bus
// fans out by event type: once two whiteboards are subscribed, every consumer
// of this key receives both boards' events off the wire. Filtering here is
// what makes "subscription is per-whiteboard" true for the consumer's stdout
// as well; routing per whiteboard on the bus would additionally save the IPC
// hop, at the cost of teaching the bus about payload contents.
//
// An event whose whiteboard cannot be read — absent, not a string, or an
// undecodable payload — is dropped rather than delivered: a consumer that
// asked for one whiteboard must not be handed an event that cannot be
// attributed to it. Match has no diagnostic channel, so these drops are
// silent.
func whiteboardIDMatch(raw *event.RawEvent, params map[string]string) bool {
	want := params["whiteboard_id"]
	if want == "" {
		return false
	}
	var envelope struct {
		Event struct {
			WhiteboardID string `json:"whiteboard_id"`
		} `json:"event"`
	}
	if err := json.Unmarshal(raw.Payload, &envelope); err != nil {
		return false
	}
	return envelope.Event.WhiteboardID == want
}
