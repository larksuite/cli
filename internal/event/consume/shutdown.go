// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
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
func checkLastForKey(conn net.Conn, eventKey string) bool {
	msg := protocol.NewPreShutdownCheck(eventKey)
	if err := protocol.EncodeWithDeadline(conn, msg, protocol.WriteTimeout); err != nil {
		return true
	}

	if err := conn.SetReadDeadline(time.Now().Add(preShutdownAckTimeout)); err != nil {
		return true
	}
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		resp, err := protocol.Decode(scanner.Bytes())
		if err == nil {
			if ack, ok := resp.(*protocol.PreShutdownAck); ok {
				return ack.LastForKey
			}
		}
	}
	return true
}
