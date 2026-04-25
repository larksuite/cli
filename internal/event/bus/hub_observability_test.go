// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"net"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

func TestHubDroppedCountIncrements(t *testing.T) {
	h := NewHub()
	server, client := testNetPipe(t)
	defer server.Close()
	defer client.Close()
	c := NewConn(server, nil, "k", []string{"t"}, 1)
	c.sendCh = make(chan interface{}, 1)
	h.RegisterAndIsFirst(c)

	h.Publish(&event.RawEvent{EventType: "t"})
	h.Publish(&event.RawEvent{EventType: "t"})
	h.Publish(&event.RawEvent{EventType: "t"})

	if got := c.DroppedCount(); got != 2 {
		t.Errorf("expected 2 drops, got %d", got)
	}
}

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

// Explicit SourceTime (upstream header.create_time) must win over local Timestamp.
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
		Timestamp:  time.UnixMilli(1999999999999),
	})

	msg := <-c.SendCh()
	ev := msg.(*protocol.Event)
	if ev.SourceTime != upstreamTs {
		t.Errorf("SourceTime: got %q, want %q", ev.SourceTime, upstreamTs)
	}
}

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

func testNetPipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	return net.Pipe()
}
