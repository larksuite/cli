// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package source

import (
	"testing"

	"github.com/larksuite/cli/internal/event/protocol"
)

// TestTryNotify_Classify covers the four log shapes we care about plus
// the regression case: SDK's "disconnected to <wss-url>" literally
// contains the substring "connected to ws", so a Contains-based switch
// misreported every disconnect as a reconnect. HasPrefix matching makes
// "connected to " and "disconnected to " mutually exclusive.
func TestTryNotify_Classify(t *testing.T) {
	cases := []struct {
		name       string
		msg        string
		errDetail  string
		wantState  string
		wantDetail string
		wantCalled bool
	}{
		{
			name:       "connected (SDK connect success)",
			msg:        "connected to wss://example.com/gw [conn_id=abc]",
			wantState:  protocol.SourceStateConnected,
			wantCalled: true,
		},
		{
			// REGRESSION: prior Contains-based switch matched "connected to ws"
			// inside "disconnected to wss...", misclassifying every disconnect
			// as a Connected notification. Must now land in Disconnected.
			name:       "disconnected must not be misclassified as connected",
			msg:        "disconnected to wss://example.com/gw [conn_id=abc]",
			wantState:  protocol.SourceStateDisconnected,
			wantCalled: true,
		},
		{
			name:       "disconnected carries errDetail through",
			msg:        "disconnected to wss://example.com/gw [conn_id=abc]",
			errDetail:  "read tcp: broken pipe",
			wantState:  protocol.SourceStateDisconnected,
			wantDetail: "read tcp: broken pipe",
			wantCalled: true,
		},
		{
			name:       "reconnecting with attempt 1",
			msg:        "trying to reconnect: 1 [conn_id=abc]",
			wantState:  protocol.SourceStateReconnecting,
			wantDetail: "attempt 1",
			wantCalled: true,
		},
		{
			name:       "reconnecting with attempt 12",
			msg:        "trying to reconnect: 12",
			wantState:  protocol.SourceStateReconnecting,
			wantDetail: "attempt 12",
			wantCalled: true,
		},
		{
			name:       "case-insensitive connected",
			msg:        "CONNECTED TO WSS://example.com",
			wantState:  protocol.SourceStateConnected,
			wantCalled: true,
		},
		{
			// Generic SDK errors shouldn't mutate source state; they stay in
			// bus.log but don't reach the user as a lifecycle event.
			name:       "ignore generic connect-failed error",
			msg:        "connect failed, err: dial tcp: i/o timeout",
			errDetail:  "connect failed, err: dial tcp: i/o timeout",
			wantCalled: false,
		},
		{
			name:       "ignore read-loop failure",
			msg:        "receive message failed, err: websocket: close 1006",
			errDetail:  "receive message failed, err: websocket: close 1006",
			wantCalled: false,
		},
		{
			name:       "ignore heartbeat noise",
			msg:        "receive pong",
			wantCalled: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotState, gotDetail string
			called := false
			lg := &sdkLogger{notify: func(state, detail string) {
				called = true
				gotState = state
				gotDetail = detail
			}}
			lg.tryNotify(tc.msg, tc.errDetail)

			if called != tc.wantCalled {
				t.Fatalf("called=%v, want %v (msg=%q)", called, tc.wantCalled, tc.msg)
			}
			if !called {
				return
			}
			if gotState != tc.wantState {
				t.Errorf("state = %q, want %q", gotState, tc.wantState)
			}
			if gotDetail != tc.wantDetail {
				t.Errorf("detail = %q, want %q", gotDetail, tc.wantDetail)
			}
		})
	}
}

// TestTryNotify_NilNotifySafe protects the early-return guard: a logger
// without a notify callback (the l-only path used before bus wiring) must
// not panic when classifying lines.
func TestTryNotify_NilNotifySafe(t *testing.T) {
	lg := &sdkLogger{notify: nil}
	lg.tryNotify("disconnected to wss://example.com", "")
	lg.tryNotify("connected to wss://example.com", "")
	lg.tryNotify("trying to reconnect: 1", "")
}
