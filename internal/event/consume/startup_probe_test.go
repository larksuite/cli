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

type probeMockTransport struct {
	mu       sync.Mutex
	listener net.Listener
	addr     string

	wg    sync.WaitGroup
	conns []net.Conn
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

func (m *probeMockTransport) trackConn(c net.Conn) {
	m.mu.Lock()
	m.conns = append(m.conns, c)
	m.mu.Unlock()
}

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

func runHealthyBus(t *testing.T, m *probeMockTransport) {
	t.Helper()
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
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

		realConn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.trackConn(realConn)
	}()
}

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
	if elapsed > 3*time.Second {
		t.Errorf("expected ~2s timeout, got %v", elapsed)
	}
}
