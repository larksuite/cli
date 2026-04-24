// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// helloAckTimeout bounds the wait for HelloAck after sending Hello. A
// bus that accepted the connection but wedged before replying would
// otherwise hang the consumer indefinitely. Symmetric with the 5-second
// read deadline the bus applies to its own Hello read (bus.go
// handleConn). Cleared (SetReadDeadline zero) once the ack arrives so
// the subsequent event-loop reads are unbounded.
const helloAckTimeout = 5 * time.Second

// doHello sends a Hello and waits for HelloAck. Returns a bufio.Reader
// holding any bytes already pulled off conn (events buffered with the
// ack in the same TCP segment) so the caller's consume loop can keep
// reading without a fresh scanner dropping them.
func doHello(conn net.Conn, eventKey string, eventTypes []string) (*protocol.HelloAck, *bufio.Reader, error) {
	hello := protocol.NewHello(os.Getpid(), eventKey, eventTypes, "v1")
	if err := protocol.EncodeWithDeadline(conn, hello, protocol.WriteTimeout); err != nil {
		return nil, nil, err
	}

	if err := conn.SetReadDeadline(time.Now().Add(helloAckTimeout)); err != nil {
		return nil, nil, fmt.Errorf("set hello_ack deadline: %w", err)
	}
	br := bufio.NewReader(conn)
	line, err := protocol.ReadFrame(br)
	if err != nil {
		return nil, nil, fmt.Errorf("no hello_ack received: %w", err)
	}
	// Clear the deadline: subsequent event-loop reads are unbounded.
	// Best-effort (matches bus.go's handleConn symmetry): a failure here
	// means the connection already broke after ack was received, and
	// the loop will surface the real error immediately on its first
	// read. Aborting handshake with a misleading "clear deadline" error
	// would mask that.
	_ = conn.SetReadDeadline(time.Time{})
	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return nil, nil, err
	}
	ack, ok := msg.(*protocol.HelloAck)
	if !ok {
		return nil, nil, fmt.Errorf("expected hello_ack, got %T", msg)
	}
	return ack, br, nil
}
