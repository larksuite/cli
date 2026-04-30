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

const preShutdownAckTimeout = 2 * time.Second

// checkLastForKey atomically reserves a cleanup lock; on any error defaults to true
// (cleanup-on-error is safer than leaking server state). Discards non-ack frames in flight.
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
			return true
		}
		resp, err := protocol.Decode(bytes.TrimRight(line, "\n"))
		if err != nil {
			continue
		}
		if ack, ok := resp.(*protocol.PreShutdownAck); ok {
			return ack.LastForKey
		}
	}
}
