// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package websocket

import (
	"testing"

	"github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	"github.com/larksuite/cli/internal/event/bus"
)

// The adapter reports source states with its own constants so it never
// imports the IPC package; this test pins all three vocabularies (adapter,
// bus port, IPC frame) to the same wire values.
func TestSourceStates_MatchTheWireVocabulary(t *testing.T) {
	pins := []struct{ adapter, port, wire string }{
		{sourceStateConnecting, bus.SourceStateConnecting, protocol.SourceStateConnecting},
		{sourceStateConnected, bus.SourceStateConnected, protocol.SourceStateConnected},
		{sourceStateDisconnected, bus.SourceStateDisconnected, protocol.SourceStateDisconnected},
		{sourceStateReconnecting, bus.SourceStateReconnecting, protocol.SourceStateReconnecting},
	}
	for _, pin := range pins {
		if pin.adapter != pin.port || pin.port != pin.wire {
			t.Errorf("state vocabulary drifted: adapter=%q port=%q wire=%q", pin.adapter, pin.port, pin.wire)
		}
	}
}
