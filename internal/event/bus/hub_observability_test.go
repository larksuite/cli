package bus

import (
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// TestHubDroppedCountIncrements verifies Publish increments a per-conn
// Dropped counter when the drop-oldest path fires.
func TestHubDroppedCountIncrements(t *testing.T) {
	h := NewHub()
	// Use a real Conn for its IncrementDropped path.
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 1) // tiny buffer to force drops
	h.RegisterAndIsFirst(c)

	// First publish fills the channel.
	h.Publish(&event.RawEvent{EventType: "t"})
	// Second publish triggers drop-oldest.
	h.Publish(&event.RawEvent{EventType: "t"})
	// Third publish also triggers drop-oldest.
	h.Publish(&event.RawEvent{EventType: "t"})

	if got := c.DroppedCount(); got != 2 {
		t.Errorf("expected 2 drops, got %d", got)
	}
}

// TestPublishAssignsIncrementalSeq verifies each event sent to a consumer
// has a monotonically increasing Seq starting from 1.
func TestPublishAssignsIncrementalSeq(t *testing.T) {
	h := NewHub()
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 10)
	h.RegisterAndIsFirst(c)

	for i := 0; i < 5; i++ {
		h.Publish(&event.RawEvent{EventType: "t"})
	}

	for i := uint64(1); i <= 5; i++ {
		msg := <-c.SendCh()
		ev, ok := msg.(*protocol.Event)
		if !ok {
			t.Fatalf("iter %d: expected *protocol.Event, got %T", i, msg)
		}
		if ev.Seq != i {
			t.Errorf("iter %d: expected seq %d, got %d", i, i, ev.Seq)
		}
	}
}

// TestPublishPopulatesEventIDAndSourceTime verifies the protocol.Event
// carries EventID and SourceTime derived from the RawEvent.
func TestPublishPopulatesEventIDAndSourceTime(t *testing.T) {
	h := NewHub()
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 1)
	h.RegisterAndIsFirst(c)

	const eid = "test-event-id-123"
	h.Publish(&event.RawEvent{
		EventID:   eid,
		EventType: "t",
		Timestamp: time.UnixMilli(1234567890123),
	})

	msg := <-c.SendCh()
	ev := msg.(*protocol.Event)
	if ev.EventID != eid {
		t.Errorf("expected EventID %q, got %q", eid, ev.EventID)
	}
	if ev.SourceTime != "1234567890123" {
		t.Errorf("expected SourceTime \"1234567890123\", got %q", ev.SourceTime)
	}
}

// TestPublishSourceTimeTakesPrecedence verifies that when RawEvent carries
// an explicit SourceTime (from the upstream header.create_time), it wins
// over Timestamp — Timestamp is a local observability field; SourceTime
// is the upstream publisher's intent and should flow through unchanged.
func TestPublishSourceTimeTakesPrecedence(t *testing.T) {
	h := NewHub()
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 1)
	h.RegisterAndIsFirst(c)

	const upstreamTs = "1700000000000"
	h.Publish(&event.RawEvent{
		EventID:    "evt-1",
		EventType:  "t",
		SourceTime: upstreamTs,
		// Local arrival time — different from upstream publish time.
		Timestamp: time.UnixMilli(1999999999999),
	})

	msg := <-c.SendCh()
	ev := msg.(*protocol.Event)
	if ev.SourceTime != upstreamTs {
		t.Errorf("SourceTime: got %q, want %q (explicit SourceTime must beat derived Timestamp)", ev.SourceTime, upstreamTs)
	}
}

// TestPublishSourceTimeFallback verifies that when SourceTime is empty
// (e.g. a test source that didn't populate it), Publish falls back to
// formatting Timestamp as UnixMilli so protocol.Event.SourceTime is still
// populated for downstream consumers that rely on it.
func TestPublishSourceTimeFallback(t *testing.T) {
	h := NewHub()
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 1)
	h.RegisterAndIsFirst(c)

	h.Publish(&event.RawEvent{
		EventID:   "evt-2",
		EventType: "t",
		Timestamp: time.UnixMilli(42),
	})

	msg := <-c.SendCh()
	ev := msg.(*protocol.Event)
	if ev.SourceTime != "42" {
		t.Errorf("SourceTime fallback: got %q, want %q", ev.SourceTime, "42")
	}
}

// testNetPipe is a test helper that returns an net.Pipe pair.
func testNetPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	return net.Pipe()
}
