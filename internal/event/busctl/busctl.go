// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package busctl is the wire-level control client for the event bus daemon.
package busctl

import (
	"bufio"
	"bytes"
	"fmt"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
	"github.com/larksuite/cli/internal/event/transport"
)

const readTimeout = 5 * time.Second // matches protocol.WriteTimeout

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

// SendShutdown sends a Shutdown command; caller polls Dial to confirm exit.
func SendShutdown(tr transport.IPC, appID string) error {
	conn, err := tr.Dial(tr.Address(appID))
	if err != nil {
		return err
	}
	defer conn.Close()
	return protocol.EncodeWithDeadline(conn, protocol.NewShutdown(), protocol.WriteTimeout)
}
