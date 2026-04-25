// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package source

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/event/protocol"
)

// Samples preserve the real SDK shape ("<verb> to <url>[conn_id=...]" — no space before bracket).
func TestSDKLogPatternsMatchKnownSDKOutput(t *testing.T) {
	cases := []struct {
		name          string
		sdkLogSample  string
		expectedState string
	}{
		{
			name:          "reconnect with attempt number",
			sdkLogSample:  "trying to reconnect: 2[conn_id=abc123]",
			expectedState: protocol.SourceStateReconnecting,
		},
		{
			name:          "reconnect high attempt",
			sdkLogSample:  "trying to reconnect: 12",
			expectedState: protocol.SourceStateReconnecting,
		},
		{
			name:          "connected success with conn_id",
			sdkLogSample:  "connected to wss://open.feishu.cn/gateway[conn_id=abc123]",
			expectedState: protocol.SourceStateConnected,
		},
		{
			name:          "connected to custom gateway",
			sdkLogSample:  "connected to wss://internal.example.com/gw",
			expectedState: protocol.SourceStateConnected,
		},
		{
			name:          "disconnected does not alias connected",
			sdkLogSample:  "disconnected to wss://open.feishu.cn/gateway[conn_id=abc123]",
			expectedState: protocol.SourceStateDisconnected,
		},
		{
			name:          "connected uppercase",
			sdkLogSample:  "CONNECTED TO WSS://OPEN.FEISHU.CN/GATEWAY",
			expectedState: protocol.SourceStateConnected,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var gotState string
			called := false
			notify := func(state, detail string) {
				mu.Lock()
				gotState = state
				called = true
				mu.Unlock()
			}
			logger := &sdkLogger{notify: notify}
			logger.Info(context.Background(), tc.sdkLogSample)

			mu.Lock()
			defer mu.Unlock()
			if !called {
				t.Fatalf("SDK log sample %q did not trigger notify — SDK log format may have changed", tc.sdkLogSample)
			}
			if gotState != tc.expectedState {
				t.Errorf("SDK log sample %q classified as %q, want %q", tc.sdkLogSample, gotState, tc.expectedState)
			}
		})
	}
}

func TestSDKLogPatternsConstantsContainExpectedSubstrings(t *testing.T) {
	if !strings.Contains(sdkLogReconnecting, "reconnect") {
		t.Errorf("sdkLogReconnecting should contain 'reconnect', got %q", sdkLogReconnecting)
	}
	if !strings.Contains(sdkLogConnected, "connected") {
		t.Errorf("sdkLogConnected should contain 'connected', got %q", sdkLogConnected)
	}
	if !strings.Contains(sdkLogDisconnected, "disconnected") {
		t.Errorf("sdkLogDisconnected should contain 'disconnected', got %q", sdkLogDisconnected)
	}
	if sdkLogReconnecting != strings.ToLower(sdkLogReconnecting) {
		t.Errorf("sdkLogReconnecting must be lowercase, got %q", sdkLogReconnecting)
	}
	if sdkLogConnected != strings.ToLower(sdkLogConnected) {
		t.Errorf("sdkLogConnected must be lowercase, got %q", sdkLogConnected)
	}
	if sdkLogDisconnected != strings.ToLower(sdkLogDisconnected) {
		t.Errorf("sdkLogDisconnected must be lowercase, got %q", sdkLogDisconnected)
	}
	if strings.HasPrefix(sdkLogDisconnected, sdkLogConnected) {
		t.Errorf("sdkLogConnected %q is a prefix of sdkLogDisconnected %q — restore the trailing space.",
			sdkLogConnected, sdkLogDisconnected)
	}
	if !strings.HasSuffix(sdkLogConnected, " ") {
		t.Error("sdkLogConnected must keep its trailing space")
	}
	if !strings.HasSuffix(sdkLogDisconnected, " ") {
		t.Error("sdkLogDisconnected must keep its trailing space")
	}
}
