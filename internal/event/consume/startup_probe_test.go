// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"bufio"
	"bytes"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

// probeMockTransport is a minimal mock for probeAndDialBus tests. It
// owns a listener plus a WaitGroup so `stop()` is a hard barrier: when
// stop returns, no accept/read goroutine remains running. Earlier
// versions leaked goroutines via accept loops that never woke up on
// listener close and per-conn readers that held the conn past test end.
type probeMockTransport struct {
	mu       sync.Mutex
	listener net.Listener
	addr     string

	wg    sync.WaitGroup // accept + per-conn reader goroutines
	conns []net.Conn     // every accepted conn, closed in stop()
}

func newProbeMockTransport(t *testing.T) *probeMockTransport {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &probeMockTransport{listener: ln, addr: ln.Addr().String()}
}

func (m *probeMockTransport) Listen(addr string) (net.Listener, error) {
	return m.listener, nil
}

func (m *probeMockTransport) Dial(addr string) (net.Conn, error) {
	return net.Dial("tcp", m.addr)
}

func (m *probeMockTransport) Address(appID string) string { return m.addr }
func (m *probeMockTransport) Cleanup(addr string)         {}

// trackConn records conn so stop() can close it, ensuring readers don't
// block on a dangling peer past test cleanup.
func (m *probeMockTransport) trackConn(c net.Conn) {
	m.mu.Lock()
	m.conns = append(m.conns, c)
	m.mu.Unlock()
}

// stop closes the listener and every tracked conn, then waits for all
// spawned goroutines (accept loop, per-conn readers) to exit.
func (m *probeMockTransport) stop() {
	m.mu.Lock()
	_ = m.listener.Close()
	conns := append([]net.Conn(nil), m.conns...)
	m.conns = nil
	m.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
	m.wg.Wait()
}

// runHealthyBus spawns a goroutine that accepts one probe + one real
// Hello conn. The real conn is tracked so stop() closes it; the
// goroutine exits when the listener errors or the test ends.
func runHealthyBus(t *testing.T, m *probeMockTransport) {
	t.Helper()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		// Accept probe
		probeConn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.trackConn(probeConn)
		br := bufio.NewReader(probeConn)
		line, _ := br.ReadBytes('\n')
		msg, _ := protocol.Decode(bytes.TrimRight(line, "\n"))
		if _, ok := msg.(*protocol.StatusQuery); ok {
			_ = protocol.Encode(probeConn, protocol.NewStatusResponse(12345, 10, 0, nil))
		}
		_ = probeConn.Close()

		// Accept the "real" conn the caller Dials after probe succeeds.
		realConn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.trackConn(realConn)
		// Leave it open; caller will use this for Hello. stop() will close it.
	}()
}

// runDeadBus accepts conns but never responds to StatusQuery — simulates
// a mid-shutdown bus where the listener is still up but the handler
// layer isn't responsive. Tracked conns ensure readers unblock on stop.
func runDeadBus(t *testing.T, m *probeMockTransport) {
	t.Helper()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		for {
			conn, err := m.listener.Accept()
			if err != nil {
				return
			}
			m.trackConn(conn)
			m.wg.Add(1)
			go func(c net.Conn) {
				defer m.wg.Done()
				buf := make([]byte, 4096)
				for {
					if _, err := c.Read(buf); err != nil {
						return
					}
				}
			}(conn)
		}
	}()
}

func TestProbeAndDialBusHealthy(t *testing.T) {
	m := newProbeMockTransport(t)
	t.Cleanup(m.stop)
	runHealthyBus(t, m)

	conn, err := probeAndDialBus(m, m.addr)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil conn")
	}
	conn.Close()
}

func TestProbeAndDialBusUnresponsive(t *testing.T) {
	m := newProbeMockTransport(t)
	t.Cleanup(m.stop)
	runDeadBus(t, m)

	start := time.Now()
	conn, err := probeAndDialBus(m, m.addr)
	elapsed := time.Since(start)

	if err == nil {
		conn.Close()
		t.Fatal("expected error on unresponsive bus")
	}
	// Should fail within ~2s deadline plus a bit of scheduling slack.
	if elapsed > 3*time.Second {
		t.Errorf("expected ~2s timeout, got %v", elapsed)
	}
}
