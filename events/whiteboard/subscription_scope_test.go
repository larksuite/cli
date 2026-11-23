// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

import (
	"testing"

	event "github.com/larksuite/cli/internal/event"
)

// The whiteboard subscription is registered per whiteboard on the server, so
// the whiteboard id must take part in the consumer's subscription identity.
// Without it, two consumers of different whiteboards share one scope: the
// second consumer's setup never runs (its whiteboard is never subscribed) and
// whichever exits last unsubscribes the other one's still-active board.
func TestWhiteboardID_IsPartOfSubscriptionIdentity(t *testing.T) {
	defs := Keys()
	if len(defs) != 1 {
		t.Fatalf("expected exactly one whiteboard key, got %d", len(defs))
	}
	def := defs[0]

	var found *event.ParamDef
	for i := range def.Params {
		if def.Params[i].Name == "whiteboard_id" {
			found = &def.Params[i]
		}
	}
	if found == nil {
		t.Fatal("whiteboard_id param is missing")
	}
	if !found.SubscriptionKey {
		t.Error("whiteboard_id must be a subscription key: the server-side subscription is per-whiteboard")
	}
}
