// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"net"
	"testing"
	"time"
)

// TestDoHello_ReadDeadline — regression for the Bug 4 finding
// (CodeRabbit PR #615): consumer-side doHello had no read deadline on
// the HelloAck frame. If the bus accepts the connection but never
// replies (wedged, crashed, or network-stalled), the consumer hangs
// indefinitely, breaking bounded-startup contracts.
//
// Strategy: use net.Pipe for a lossless server side that reads Hello
// but deliberately sends nothing back. Without a read deadline the
// call hangs forever; with the fix it returns an error within
// helloAckTimeout.
func TestDoHello_ReadDeadline(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	// Server goroutine: read whatever Hello bytes arrive and discard;
	// never respond. Must drain the bytes so the client's Write does
	// not block forever on net.Pipe's synchronous semantics.
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	start := time.Now()
	done := make(chan error, 1)
	go func() {
		_, _, err := doHello(client, "im.msg", []string{"im.msg"})
		done <- err
	}()

	select {
	case err := <-done:
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("doHello returned nil error when server never replied; must fail with deadline-driven error")
		}
		// Accept any error — the concrete type may be a net.OpError
		// wrapping "i/o timeout" — we assert only on wall-clock elapsed.
		// The deadline is expected to be in the low single-digit seconds;
		// we bound at helloAckTimeout + 1s of slack.
		if elapsed > helloAckTimeout+2*time.Second {
			t.Errorf("doHello returned %v after %v; deadline should fire within ~%v", err, elapsed, helloAckTimeout)
		}
	case <-time.After(helloAckTimeout + 3*time.Second):
		t.Fatal("doHello hung past deadline + 3s slack: read deadline is missing or not being honoured")
	}
}
