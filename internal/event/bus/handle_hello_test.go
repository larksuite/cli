// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"bufio"
	"io"
	"log"
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// HelloAck write failure must unregister the conn from hub and bus before returning.
func TestHandleHello_HelloAckWriteFailureUnregisters(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second),
		shutdownCh: make(chan struct{}, 1),
	}

	server, client := net.Pipe()
	client.Close()
	defer server.Close()

	hello := &protocol.Hello{
		PID:        9999,
		EventKey:   "im.msg",
		EventTypes: []string{"im.message.receive_v1"},
	}

	br := bufio.NewReader(server)

	done := make(chan struct{})
	go func() {
		b.handleHello(server, br, hello)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handleHello did not return within 3s: stuck on write or not handling the error path")
	}

	if got := hub.ConnCount(); got != 0 {
		t.Errorf("hub.ConnCount after failed HelloAck = %d, want 0 (connection must be unregistered)", got)
	}
	if got := hub.EventKeyCount("im.msg"); got != 0 {
		t.Errorf("hub.EventKeyCount(im.msg) after failed HelloAck = %d, want 0", got)
	}
	b.mu.Lock()
	remaining := len(b.conns)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("b.conns after failed HelloAck = %d entries, want 0", remaining)
	}
}
