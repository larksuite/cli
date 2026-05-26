// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events

import (
	"testing"

	"github.com/larksuite/cli/internal/event"
)

// TestMailKeysWiredUp proves that events/register.go correctly wires up the
// mail domain Keys() into the global registry. This acts as a regression guard
// preventing accidental deletion of the mail.Keys() call in init().
func TestMailKeysWiredUp(t *testing.T) {
	def, ok := event.Lookup("mail.user_mailbox.event.message_received_v1")
	if !ok || def == nil {
		t.Fatal("mail EventKey 'mail.user_mailbox.event.message_received_v1' not registered; " +
			"check events/register.go for mail.Keys() wire-up")
	}
	if def.EventType != def.Key {
		t.Errorf("EventType %q != Key %q", def.EventType, def.Key)
	}
}
