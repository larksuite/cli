// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"os"

	"github.com/larksuite/cli/internal/event/protocol"
)

// doHello sends a Hello and waits for HelloAck. Returns a bufio.Reader
// holding any bytes already pulled off conn (events buffered with the
// ack in the same TCP segment) so the caller's consume loop can keep
// reading without a fresh scanner dropping them.
func doHello(conn net.Conn, eventKey string, eventTypes []string) (*protocol.HelloAck, *bufio.Reader, error) {
	hello := protocol.NewHello(os.Getpid(), eventKey, eventTypes, "v1")
	if err := protocol.EncodeWithDeadline(conn, hello, protocol.WriteTimeout); err != nil {
		return nil, nil, err
	}

	br := bufio.NewReader(conn)
	line, err := protocol.ReadFrame(br)
	if err != nil {
		return nil, nil, fmt.Errorf("no hello_ack received: %w", err)
	}
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
