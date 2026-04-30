// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"io"
	"log"
	"net"
	"testing"
	"time"
)

// Reproduces Run × onClose re-entrant deadlock if b.mu is held across Close.
func TestRunShutdownWithMultipleConns(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	hub := NewHub()
	b := &Bus{
		hub:    hub,
		logger: logger,
		conns:  make(map[*Conn]struct{}),
	}

	const N = 3
	pipes := make([]net.Conn, 0, N*2)
	t.Cleanup(func() {
		for _, p := range pipes {
			p.Close()
		}
	})

	for i := 0; i < N; i++ {
		server, client := net.Pipe()
		pipes = append(pipes, server, client)

		bc := NewConn(server, nil, "im.msg", []string{"im.message.receive_v1"}, 1000+i)
		bc.SetLogger(logger)
		hub.RegisterAndIsFirst(bc)

		bc.SetOnClose(func(c *Conn) {
			b.hub.UnregisterAndIsLast(c)
			b.mu.Lock()
			delete(b.conns, c)
			b.mu.Unlock()
		})

		b.mu.Lock()
		b.conns[bc] = struct{}{}
		b.mu.Unlock()
	}

	done := make(chan struct{})
	go func() {
		shutdownConns(b)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdownConns deadlocked: did not complete within 2s")
	}

	if got := hub.ConnCount(); got != 0 {
		t.Errorf("expected 0 subscribers in hub after shutdown, got %d", got)
	}
	b.mu.Lock()
	remaining := len(b.conns)
	b.mu.Unlock()
	if remaining != 0 {
		t.Errorf("expected 0 conns in Bus after shutdown, got %d", remaining)
	}
}

// shutdownCh must be buffered so a signal sent before Run's select loop is still delivered.
func TestShutdownSignalNotDroppedBeforeRunSelects(t *testing.T) {
	b := NewBus("test-app", "test-secret", "", nil, log.New(io.Discard, "", 0))

	select {
	case b.shutdownCh <- struct{}{}:
	default:
		t.Fatal("handleShutdown's send took default branch — signal would be lost")
	}

	select {
	case <-b.shutdownCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("shutdown signal was not latched")
	}
}
