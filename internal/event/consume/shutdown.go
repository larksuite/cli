// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"net"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// preShutdownAckTimeout bounds how long checkLastForKey will wait on
// the bus's ack. A bus that is wedged or already dead must not strand
// the consumer in this call — user Ctrl-C has already fired and we are
// past the ctx-Done selection point here.
const preShutdownAckTimeout = 2 * time.Second

// checkLastForKey asks the bus to atomically reserve a cleanup lock for
// this consumer's EventKey. Returns true iff the caller acquired the
// reservation (so the caller MUST run cleanup — disconnect will release
// the lock on the bus side), or false if another subscriber holds the key
// and cleanup should NOT run. On any error (including timeout) the caller
// defaults to true: running cleanup for a still-busy key is safer than
// leaking server-side state; the other consumer can re-subscribe by
// re-running consume.
//
// The "reserve first, then ack" design closes a TOCTOU race: without the
// reservation, a new subscriber could register for the same key between
// our observation of "alone" and the actual cleanup call — silently
// black-holing them.
//
// The read loop discards non-ack frames. Event and source-status frames
// may already be in flight from the bus side when we send our
// PreShutdownCheck; reading just the first frame could see an event and
// fall through, masking the real ack (or conversely, falsely reporting
// LastForKey=true on a non-ack frame we couldn't decode). Loop until an
// ack arrives, we see decoded-but-unrelated frames, or the deadline
// fires.
//
// Note: we create a fresh bufio.Reader on the raw conn here. The main
// consume loop's bufio.Reader (from doHello) may hold buffered bytes at
// shutdown, but those bytes predate our PreShutdownCheck and therefore
// cannot contain the ack we're waiting for — so any bytes it abandons
// are event/source-status frames we don't care about at shutdown time.
func checkLastForKey(conn net.Conn, eventKey string) bool {
	msg := protocol.NewPreShutdownCheck(eventKey)
	if err := protocol.EncodeWithDeadline(conn, msg, protocol.WriteTimeout); err != nil {
		return true
	}

	if err := conn.SetReadDeadline(time.Now().Add(preShutdownAckTimeout)); err != nil {
		return true
	}
	br := bufio.NewReader(conn)
	for {
		line, err := protocol.ReadFrame(br)
		if err != nil {
			// Deadline, EOF, or protocol error: default to cleanup (safer
			// than leaving server-side state stranded).
			return true
		}
		resp, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			// Malformed frame on a presumed-healthy connection. Skip and
			// keep reading — the ack may follow.
			continue
		}
		if ack, ok := resp.(*protocol.PreShutdownAck); ok {
			return ack.LastForKey
		}
		// Any other decoded frame type (Event, SourceStatus, …) was
		// queued before or alongside our ack. Discard and continue.
	}
}
