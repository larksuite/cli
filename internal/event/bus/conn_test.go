// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event/protocol"
)

func TestConn_SenderWritesEvents(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	bc := NewConn(server, nil, "im.msg", []string{"im.message.receive_v1"}, 12345)
	go bc.SenderLoop()

	bc.SendCh() <- &protocol.Event{
		Type:      protocol.MsgTypeEvent,
		EventType: "im.message.receive_v1",
	}

	scanner := bufio.NewScanner(client)
	client.SetReadDeadline(time.Now().Add(time.Second))
	if !scanner.Scan() {
		t.Fatalf("expected to read a line: %v", scanner.Err())
	}
	line := scanner.Bytes()
	if !bytes.Contains(line, []byte(`"event"`)) {
		t.Errorf("unexpected line: %s", line)
	}
}

type serializingDetector struct {
	net.Conn
	inFlight atomic.Int32
	violated atomic.Bool
}

func (s *serializingDetector) Write(b []byte) (int, error) {
	if s.inFlight.Add(1) > 1 {
		s.violated.Store(true)
	}
	time.Sleep(500 * time.Microsecond)
	defer s.inFlight.Add(-1)
	return s.Conn.Write(b)
}

// Two goroutines writing frames (event + ack) must not overlap on the underlying net.Conn.
func TestConn_ConcurrentWritesSerialised(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	det := &serializingDetector{Conn: server}
	bc := NewConn(det, nil, "im.msg", []string{"im.msg"}, 12345)

	go func() { _, _ = io.Copy(io.Discard, client) }()

	go bc.SenderLoop()

	var wg sync.WaitGroup
	const workers = 8
	const perWorker = 20
	deadline := time.Now().Add(2 * time.Second)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker && time.Now().Before(deadline); j++ {
				bc.SendCh() <- &protocol.Event{Type: protocol.MsgTypeEvent, EventType: "im.msg"}
			}
		}()
	}

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker && time.Now().Before(deadline); j++ {
				bc.handleControlMessage(&protocol.PreShutdownCheck{EventKey: "im.msg"})
			}
		}()
	}

	wg.Wait()
	bc.Close()

	if det.violated.Load() {
		t.Error("concurrent Write on net.Conn detected: SenderLoop and handleControlMessage " +
			"overlapped without serialisation (framing / deadline race)")
	}
}

func TestConn_TrySend_NonEvicting(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	bc := NewConn(server, nil, "im.msg", []string{"im.msg"}, 12345)

	for i := 0; i < sendChCap; i++ {
		if !bc.TrySend(i) {
			t.Fatalf("TrySend returned false at iteration %d; expected all sendChCap (%d) to fit", i, sendChCap)
		}
	}
	if bc.TrySend("overflow") {
		t.Fatal("TrySend on full channel returned true: TrySend must be non-evicting")
	}
	first := <-bc.SendCh()
	if first != 0 {
		t.Errorf("first drained item = %v, want 0", first)
	}
}

func TestConn_ReaderDetectsEOF(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	bc := NewConn(server, nil, "im.msg", []string{"im.msg"}, 12345)

	done := make(chan struct{})
	go func() {
		bc.ReaderLoop()
		close(done)
	}()

	client.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ReaderLoop did not exit on EOF")
	}
}
