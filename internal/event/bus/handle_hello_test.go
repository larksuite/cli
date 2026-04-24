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

// TestHandleHello_HelloAckWriteFailureUnregisters — regression for the
// Bug 6 finding (CodeRabbit PR #615). If writing HelloAck fails (peer
// closed mid-handshake, write timeout), handleHello previously
// swallowed the error and proceeded to bc.Start(), leaving the
// connection registered with the hub while SenderLoop/ReaderLoop raced
// on a broken conn. First/last bookkeeping could end up skewed and
// cleanup lock decisions incorrect.
//
// Expected: on HelloAck write error, handleHello must (1) NOT start the
// conn's goroutines, and (2) undo all hub/bus-level registration so
// counters return to pre-Hello state.
func TestHandleHello_HelloAckWriteFailureUnregisters(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:        hub,
		logger:     logger,
		conns:      make(map[*Conn]struct{}),
		idleTimer:  time.NewTimer(30 * time.Second), // arbitrary, timer API only
		shutdownCh: make(chan struct{}, 1),
	}

	// net.Pipe: synchronous / unbuffered. Close the client side
	// immediately so any write on the server side fails with
	// io.ErrClosedPipe.
	server, client := net.Pipe()
	client.Close()
	defer server.Close()

	hello := &protocol.Hello{
		PID:        9999,
		EventKey:   "im.msg",
		EventTypes: []string{"im.message.receive_v1"},
	}

	// handleHello creates a Conn with NewConn(server, reader, ...), so
	// we provide a reader. The hello has already been decoded by the
	// caller; we just need a reader for the subsequent ReaderLoop (which
	// should NOT start on the failure path).
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

	// Hub must be empty — no lingering subscriber.
	if got := hub.ConnCount(); got != 0 {
		t.Errorf("hub.ConnCount after failed HelloAck = %d, want 0 (connection must be unregistered)", got)
	}
	if got := hub.EventKeyCount("im.msg"); got != 0 {
		t.Errorf("hub.EventKeyCount(im.msg) after failed HelloAck = %d, want 0", got)
	}
	// Bus-level conn map also empty.
	b.mu.Lock()
	remaining := len(b.conns)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("b.conns after failed HelloAck = %d entries, want 0", remaining)
	}
}
