// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"encoding/json"
	"io"
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// TestCheckLastForKey_IgnoresNonAckFrames — regression for Bug 5
// (CodeRabbit PR #615). `checkLastForKey` previously scanned exactly
// one frame after sending PreShutdownCheck. If an event frame or
// source-status frame was already queued on the bus side (buffered
// with or before the ack in the same TCP window), the scanner read
// that first, the type-assert to *PreShutdownAck failed, and the
// function fell through to its default `return true`. That either
// fires an extra cleanup (spurious unsubscribe) or hides the actual
// LastForKey=false answer the bus reserved.
//
// Fix expectation: loop over frames until we see PreShutdownAck or the
// deadline fires, discarding any other frame type.
func TestCheckLastForKey_IgnoresNonAckFrames(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Server goroutine: read one PreShutdownCheck from the client, then
	// deliberately reply with an Event frame (non-ack) first, followed by
	// PreShutdownAck{LastForKey: false}. If the fix is correct,
	// checkLastForKey sees the Event, discards it, reads the next frame,
	// sees the ack, and returns false.
	errs := make(chan error, 2)
	go func() {
		// Drain the PreShutdownCheck the client sends.
		buf := make([]byte, 4096)
		if _, err := server.Read(buf); err != nil && err != io.EOF {
			errs <- err
			return
		}
		// Now write an Event frame first (should be discarded by the client).
		evt := protocol.NewEvent("im.msg", "evt_1", "", 1, json.RawMessage(`{}`))
		if err := protocol.Encode(server, evt); err != nil {
			errs <- err
			return
		}
		// Then the real ack.
		ack := protocol.NewPreShutdownAck(false)
		if err := protocol.Encode(server, ack); err != nil {
			errs <- err
			return
		}
	}()

	got := checkLastForKey(client, "im.msg")
	if got != false {
		t.Errorf("checkLastForKey = %v, want false (bus replied with LastForKey=false; "+
			"an Event frame before the ack must not be mistaken for the ack)", got)
	}

	select {
	case err := <-errs:
		t.Fatalf("server goroutine error: %v", err)
	default:
	}
}

// TestCheckLastForKey_ReturnsAckValue — sanity test that the straightforward
// "one frame = ack" path still works.
func TestCheckLastForKey_ReturnsAckValue(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		_, _ = server.Read(buf)
		ack := protocol.NewPreShutdownAck(true)
		_ = protocol.Encode(server, ack)
	}()

	got := checkLastForKey(client, "im.msg")
	if got != true {
		t.Errorf("checkLastForKey = %v, want true", got)
	}
}

// TestCheckLastForKey_DefaultsToTrueOnTimeout — when the bus never
// replies, we default to `true` (safer to run cleanup than to leave
// server-side state stranded). Bounded by preShutdownAckTimeout.
func TestCheckLastForKey_DefaultsToTrueOnTimeout(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	start := time.Now()
	got := checkLastForKey(client, "im.msg")
	elapsed := time.Since(start)

	if got != true {
		t.Errorf("checkLastForKey = %v, want true (default on timeout)", got)
	}
	if elapsed > preShutdownAckTimeout+2*time.Second {
		t.Errorf("elapsed = %v, expected ~%v (timeout-bounded)", elapsed, preShutdownAckTimeout)
	}
}
