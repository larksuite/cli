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

const helloAckTimeout = 5 * time.Second // symmetric with bus-side hello read deadline

// doHello returns a bufio.Reader holding any bytes already pulled off conn so events
// buffered with the ack in one TCP segment aren't dropped.
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
	// best-effort clear; if the conn is already broken, the loop's first read will surface it
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
