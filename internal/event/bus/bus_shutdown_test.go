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

// TestRunShutdownWithMultipleConns reproduces the Run() × onClose re-entrant
// deadlock. With the buggy code that holds b.mu across c.Close(), the FIRST
// conn's onClose callback re-acquires b.mu and deadlocks forever.
//
// The test installs onClose callbacks that mirror handleHello's real callback:
// they acquire b.mu.Lock() and delete(b.conns, c). Under the fix, shutdownConns
// releases b.mu before calling Close(), so each onClose can re-acquire it.
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

		// Mirror handleHello's onClose: takes b.mu, mutates b.conns, releases.
		// If shutdownConns holds b.mu during c.Close(), this will deadlock.
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
		// Completed without deadlock.
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

// TestShutdownSignalNotDroppedBeforeRunSelects verifies that sending on
// shutdownCh before Run enters its select still delivers. The bug was that an
// unbuffered channel drops the signal (via select/default) if nobody is ready
// to receive; the fix is a buffered (cap=1) channel so the signal is latched.
func TestShutdownSignalNotDroppedBeforeRunSelects(t *testing.T) {
	b := NewBus("test-app", "test-secret", "", nil, log.New(io.Discard, "", 0))

	// Fire the same pattern used by handleShutdown — the producer is NOT
	// blocked on a receiver, so it must rely on the buffer to latch.
	select {
	case b.shutdownCh <- struct{}{}:
	default:
		t.Fatal("handleShutdown's send took default branch — signal would be lost")
	}

	// A later receiver must see the signal.
	select {
	case <-b.shutdownCh:
		// Passed
	case <-time.After(200 * time.Millisecond):
		t.Fatal("shutdown signal was not latched")
	}
}
