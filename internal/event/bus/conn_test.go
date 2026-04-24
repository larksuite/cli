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

// serializingDetector wraps a net.Conn and records whether any two
// Write calls were ever in-flight simultaneously. Used to verify that
// Conn's write paths (SenderLoop event encode + handleControlMessage
// ack encode) don't race each other on the same underlying connection.
type serializingDetector struct {
	net.Conn
	inFlight atomic.Int32
	violated atomic.Bool
}

func (s *serializingDetector) Write(b []byte) (int, error) {
	if s.inFlight.Add(1) > 1 {
		s.violated.Store(true)
	}
	// Brief yield inside the critical section so concurrent attempts have
	// a real chance to overlap; without it Go's goroutine scheduling may
	// serialise writes coincidentally even when the code is racy.
	time.Sleep(500 * time.Microsecond)
	defer s.inFlight.Add(-1)
	return s.Conn.Write(b)
}

// TestConn_ConcurrentWritesSerialised — regression test for the
// "SenderLoop + handleControlMessage race on net.Conn" finding.
// protocol.Encode plus SetWriteDeadline is a 2-call sequence; without
// an internal mutex, two goroutines writing frames simultaneously can
// corrupt framing or race the shared deadline.
func TestConn_ConcurrentWritesSerialised(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	det := &serializingDetector{Conn: server}
	bc := NewConn(det, nil, "im.msg", []string{"im.msg"}, 12345)

	// Drain whatever the pipe receives so Writes on the server side
	// unblock; content doesn't matter for this test, we only care that
	// the server-side Write path never sees concurrent calls.
	go func() { _, _ = io.Copy(io.Discard, client) }()

	go bc.SenderLoop()

	var wg sync.WaitGroup
	const workers = 8
	const perWorker = 20
	deadline := time.Now().Add(2 * time.Second)

	// Path 1: events via SendCh → SenderLoop → writeFrame.
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker && time.Now().Before(deadline); j++ {
				bc.SendCh() <- &protocol.Event{Type: protocol.MsgTypeEvent, EventType: "im.msg"}
			}
		}()
	}

	// Path 2: ack writes via handleControlMessage on a separate goroutine,
	// simulating the ReaderLoop calling it on a PreShutdownCheck arrival.
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

// TestConn_TrySend_NonEvicting verifies TrySend succeeds while the
// channel has room and returns false when full — it must never drop
// existing messages to make room (broadcast paths use this for
// best-effort source-status fan-out).
func TestConn_TrySend_NonEvicting(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	bc := NewConn(server, nil, "im.msg", []string{"im.msg"}, 12345)

	// Fill the channel to capacity without starting SenderLoop so nothing
	// drains.
	for i := 0; i < sendChCap; i++ {
		if !bc.TrySend(i) {
			t.Fatalf("TrySend returned false at iteration %d; expected all sendChCap (%d) to fit", i, sendChCap)
		}
	}
	// One more: channel is full; must fail (not evict).
	if bc.TrySend("overflow") {
		t.Fatal("TrySend on full channel returned true: TrySend must be non-evicting")
	}
	// Drain one and verify the order preserved — if TrySend had
	// evicted, the first item would be missing and we'd see 1 not 0.
	first := <-bc.SendCh()
	if first != 0 {
		t.Errorf("first drained item = %v, want 0 (head of queue preserved)", first)
	}
}

// Note: the sendMu-sharing invariant for TrySend (Bug 3 regression) is
// proven by TestPublishRaceBookkeepingAccurate in hub_publish_race_test.go
// — that test's `returnedFalse > 0` assertion fires directly when a
// TrySend bypasses sendMu and steals the slot between another
// goroutine's drop and its retry push. Verified by manually reverting
// raceSubscriber.TrySend's lock: the assertion caught it in 3
// iterations. We therefore do not duplicate that coverage here.

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
		// ReaderLoop exited
	case <-time.After(time.Second):
		t.Fatal("ReaderLoop did not exit on EOF")
	}
}
