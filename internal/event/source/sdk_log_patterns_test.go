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

// TestSDKLogPatternsMatchKnownSDKOutput verifies that the lifecycle-pattern
// substrings still appear in real SDK log strings.
//
// Truth source for the format: larksuite/oapi-sdk-go/v3@v3.5.3 ws/client.go
// fmtLog — it constructs lines via fmt.Sprint which does NOT insert a
// separator between the message and the [conn_id=...] bracket. So the real
// SDK output is "connected to wss://.../gw[conn_id=abc]" (no space before
// the bracket), not "connected to wss://.../gw [conn_id=abc]". The samples
// below must preserve that exact shape; softening them with a spurious
// space would be fake provenance (see CLAUDE.md §4 — unit tests are not the
// truth source, the SDK is).
//
// If the SDK changes its log format, this test will fail — prompting us to
// update both the constants in sdk_log_patterns.go AND the samples here.
// Do NOT silence the failure by softening the samples; the whole point is to
// catch upstream drift at CI time, not in production.
//
// Note: tryNotify uses HasPrefix (not Contains) after we discovered that
// "disconnected to wss..." literally contains "connected to ws" as a
// substring. So samples here must begin with the expected prefix, matching
// the real SDK line shape "<verb> to <url>[conn_id=...]".
func TestSDKLogPatternsMatchKnownSDKOutput(t *testing.T) {
	cases := []struct {
		name          string
		sdkLogSample  string
		expectedState string
	}{
		// Sample strings matching oapi-sdk-go/v3 ws/client.go fmtLog output.
		// fmtLog uses fmt.Sprint — no separator before the [conn_id=...]
		// bracket.
		// Shape: "trying to reconnect: N[conn_id=...]"
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
		// Shape: "connected to wss://<host>/<path>[conn_id=...]"
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
		// Shape: "disconnected to wss://<host>/<path>[conn_id=...]"
		//
		// CRITICAL regression sample: this line literally contains
		// "connected to ws" as a substring. A Contains-based matcher would
		// misclassify it as Connected. HasPrefix is what keeps it honest.
		{
			name:          "disconnected does not alias connected",
			sdkLogSample:  "disconnected to wss://open.feishu.cn/gateway[conn_id=abc123]",
			expectedState: protocol.SourceStateDisconnected,
		},
		// Case-insensitivity: Info/Warn callers may emit upper or mixed
		// case in future SDK versions; tryNotify lowercases first.
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
			// Route through Info — the classifier should fire regardless
			// of level. (Warn/Error exercise the same tryNotify path.)
			logger.Info(context.Background(), tc.sdkLogSample)

			mu.Lock()
			defer mu.Unlock()
			if !called {
				t.Fatalf("SDK log sample %q did not trigger notify — the SDK may have changed its log format; update sdk_log_patterns.go accordingly",
					tc.sdkLogSample)
			}
			if gotState != tc.expectedState {
				t.Errorf("SDK log sample %q classified as %q, want %q — the SDK may have changed its log format; update sdk_log_patterns.go accordingly",
					tc.sdkLogSample, gotState, tc.expectedState)
			}
		})
	}
}

// TestSDKLogPatternsConstantsContainExpectedSubstrings is a paranoid check
// that someone hasn't modified the constants themselves to subtly break
// matching. If this fails, the constants were changed incorrectly.
func TestSDKLogPatternsConstantsContainExpectedSubstrings(t *testing.T) {
	// These assertions pin the semantic intent of each constant.
	if !strings.Contains(sdkLogReconnecting, "reconnect") {
		t.Errorf("sdkLogReconnecting should contain 'reconnect', got %q", sdkLogReconnecting)
	}
	if !strings.Contains(sdkLogConnected, "connected") {
		t.Errorf("sdkLogConnected should contain 'connected', got %q", sdkLogConnected)
	}
	if !strings.Contains(sdkLogDisconnected, "disconnected") {
		t.Errorf("sdkLogDisconnected should contain 'disconnected', got %q", sdkLogDisconnected)
	}
	// Constants must be lowercase: tryNotify calls strings.ToLower before
	// matching, so any uppercase content here would never match.
	if sdkLogReconnecting != strings.ToLower(sdkLogReconnecting) {
		t.Errorf("sdkLogReconnecting must be lowercase, got %q", sdkLogReconnecting)
	}
	if sdkLogConnected != strings.ToLower(sdkLogConnected) {
		t.Errorf("sdkLogConnected must be lowercase, got %q", sdkLogConnected)
	}
	if sdkLogDisconnected != strings.ToLower(sdkLogDisconnected) {
		t.Errorf("sdkLogDisconnected must be lowercase, got %q", sdkLogDisconnected)
	}
	// Ambiguity guard: sdkLogConnected must not be a prefix of
	// sdkLogDisconnected (or vice versa). The trailing space on both is
	// what enforces this; if someone drops the space, HasPrefix matching
	// breaks and every disconnect becomes a connect.
	if strings.HasPrefix(sdkLogDisconnected, sdkLogConnected) {
		t.Errorf("sdkLogConnected %q is a prefix of sdkLogDisconnected %q — HasPrefix matching will misclassify every disconnect as a connect. Restore the trailing space on sdkLogConnected.",
			sdkLogConnected, sdkLogDisconnected)
	}
	// Explicit trailing-space pins. The HasPrefix guard above catches the
	// case where the space on sdkLogConnected is dropped (because then
	// "connected to" becomes a prefix of "disconnected to "), but it does
	// NOT catch the symmetric case where the space is dropped from
	// sdkLogDisconnected — that would still produce an unambiguous constant
	// pair yet silently break matching against noise lines like
	// "disconnected: io timeout" which happen to start with "disconnected".
	// Pin both explicitly so any trailing-space regression is loud.
	if !strings.HasSuffix(sdkLogConnected, " ") {
		t.Error("sdkLogConnected must keep its trailing space — it disambiguates from 'disconnected' under HasPrefix")
	}
	if !strings.HasSuffix(sdkLogDisconnected, " ") {
		t.Error("sdkLogDisconnected must keep its trailing space — without it, 'disconnected to ws...' has no unique prefix vs noise lines starting with 'disconnected:'")
	}
}
