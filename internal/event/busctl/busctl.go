// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package busctl is the wire-level control client for the event bus
// daemon: sends StatusQuery / Shutdown over IPC and returns the parsed
// response. Keeping this in internal/event/ lets cmd/event/ talk only
// to one abstraction layer instead of directly importing transport +
// protocol for control-plane operations.
package busctl

import (
	"bufio"
	"bytes"
	"fmt"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
)

// readTimeout bounds the status-response read. Matches protocol.WriteTimeout
// so a wedged bus can't hang the caller longer on one side than the other.
const readTimeout = 5 * time.Second

// QueryStatus dials the bus for appID, sends a StatusQuery, and reads
// back the StatusResponse. Uses bufio-framed reads so a multi-segment
// response (common once consumer count grows) reassembles correctly.
func QueryStatus(tr transport.IPC, appID string) (*protocol.StatusResponse, error) {
	conn, err := tr.Dial(tr.Address(appID))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := protocol.EncodeWithDeadline(conn, protocol.NewStatusQuery(), protocol.WriteTimeout); err != nil {
		return nil, err
	}

	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return nil, err
	}
	line, err := protocol.ReadFrame(bufio.NewReader(conn))
	if err != nil {
		return nil, err
	}

	msg, err := protocol.Decode(bytes.TrimRight(line, "\n"))
	if err != nil {
		return nil, err
	}
	resp, ok := msg.(*protocol.StatusResponse)
	if !ok {
		return nil, fmt.Errorf("unexpected response type from bus: %T", msg)
	}
	return resp, nil
}

// SendShutdown dials the bus for appID and sends a Shutdown command.
// Does not wait for the bus process to exit — the caller is responsible
// for polling Dial to confirm shutdown actually happened.
func SendShutdown(tr transport.IPC, appID string) error {
	conn, err := tr.Dial(tr.Address(appID))
	if err != nil {
		return err
	}
	defer conn.Close()
	return protocol.EncodeWithDeadline(conn, protocol.NewShutdown(), protocol.WriteTimeout)
}
