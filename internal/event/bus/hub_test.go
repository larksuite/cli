// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"encoding/json"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

func TestHub_Subscribe(t *testing.T) {
	h := NewHub()
	c := newTestConn("mail.user_mailbox.event.message_received_v1", []string{"mail.event.v1"})
	h.RegisterAndIsFirst(c)

	if h.ConnCount() != 1 {
		t.Errorf("expected 1 conn, got %d", h.ConnCount())
	}
}

func TestHub_Publish_RoutesToSubscriber(t *testing.T) {
	h := NewHub()
	c := newTestConn("im.msg", []string{"im.message.receive_v1"})
	h.RegisterAndIsFirst(c)

	raw := &event.RawEvent{
		EventID:   "evt-1",
		EventType: "im.message.receive_v1",
		Payload:   json.RawMessage(`{}`),
	}
	h.Publish(raw)

	select {
	case msg := <-c.sendCh:
		evt, ok := msg.(*protocol.Event)
		if !ok {
			t.Fatalf("expected *Event, got %T", msg)
		}
		if evt.EventType != "im.message.receive_v1" {
			t.Errorf("got event_type %q", evt.EventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout waiting for event")
	}
}

func TestHub_Publish_SkipsUnmatchedSubscriber(t *testing.T) {
	h := NewHub()
	c := newTestConn("mail.new", []string{"mail.event.v1"})
	h.RegisterAndIsFirst(c)

	raw := &event.RawEvent{
		EventID:   "evt-1",
		EventType: "im.message.receive_v1",
		Payload:   json.RawMessage(`{}`),
	}
	h.Publish(raw)

	select {
	case <-c.sendCh:
		t.Fatal("should not receive unmatched event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_Publish_NonBlocking(t *testing.T) {
	h := NewHub()
	c := newTestConn("im", []string{"im.message.receive_v1"})
	c.sendCh = make(chan interface{}, 1)
	h.RegisterAndIsFirst(c)

	c.sendCh <- &protocol.Event{}

	done := make(chan struct{})
	go func() {
		raw := &event.RawEvent{
			EventType: "im.message.receive_v1",
			Payload:   json.RawMessage(`{}`),
		}
		h.Publish(raw)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Publish blocked on full channel")
	}
}

func TestHub_Unregister(t *testing.T) {
	h := NewHub()
	c := newTestConn("im", []string{"im.msg"})
	h.RegisterAndIsFirst(c)
	h.UnregisterAndIsLast(c)

	if h.ConnCount() != 0 {
		t.Errorf("expected 0 conns, got %d", h.ConnCount())
	}
}

func TestHub_UnregisterAndIsLast_NeverRegistered(t *testing.T) {
	h := NewHub()
	real := newTestConn("im", []string{"im.msg"})
	h.RegisterAndIsFirst(real)
	ghost := newTestConn("im", []string{"im.msg"})

	if h.UnregisterAndIsLast(ghost) {
		t.Error("ghost unregister returned true: must be false when subscriber never registered")
	}
	if got := h.EventKeyCount("im"); got != 1 {
		t.Errorf("keyCount for 'im' = %d after ghost unregister; want 1 (real still registered)", got)
	}
	if !h.UnregisterAndIsLast(real) {
		t.Error("real unregister returned false; expected true (sole subscriber)")
	}
}

func TestHub_UnregisterAndIsLast_DoubleUnregister(t *testing.T) {
	h := NewHub()
	c := newTestConn("im", []string{"im.msg"})
	h.RegisterAndIsFirst(c)

	if !h.UnregisterAndIsLast(c) {
		t.Fatal("first unregister returned false; expected true (sole subscriber)")
	}
	if h.UnregisterAndIsLast(c) {
		t.Error("second unregister returned true: duplicate unregister must report false")
	}
}

func TestHub_EventKeyCount(t *testing.T) {
	h := NewHub()
	c1 := newTestConn("mail.user_mailbox.event.message_received_v1", []string{"mail.v1"})
	c2 := newTestConn("mail.user_mailbox.event.message_received_v1", []string{"mail.v1"})
	h.RegisterAndIsFirst(c1)
	h.RegisterAndIsFirst(c2)

	if h.EventKeyCount("mail.user_mailbox.event.message_received_v1") != 2 {
		t.Errorf("expected 2, got %d", h.EventKeyCount("mail.user_mailbox.event.message_received_v1"))
	}

	h.UnregisterAndIsLast(c1)
	if h.EventKeyCount("mail.user_mailbox.event.message_received_v1") != 1 {
		t.Errorf("expected 1 after unregister, got %d", h.EventKeyCount("mail.user_mailbox.event.message_received_v1"))
	}
}

func TestHub_RegisterAndIsFirst_Concurrent(t *testing.T) {
	h := NewHub()
	const N = 200
	eventKey := "mail.user_mailbox.event.message_received_v1"

	var firstCount int32
	var wg sync.WaitGroup
	wg.Add(N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start
			c := newTestConn(eventKey, []string{"mail.v1"})
			if h.RegisterAndIsFirst(c) {
				atomic.AddInt32(&firstCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&firstCount); got != 1 {
		t.Errorf("RegisterAndIsFirst returned true %d times across %d concurrent registrants; want exactly 1", got, N)
	}
	if got := h.EventKeyCount(eventKey); got != N {
		t.Errorf("EventKeyCount = %d, want %d", got, N)
	}
}

func TestHub_UnregisterAndIsLast_Concurrent(t *testing.T) {
	h := NewHub()
	const N = 200
	eventKey := "im.message.receive_v1"

	conns := make([]*testConn, N)
	for i := 0; i < N; i++ {
		conns[i] = newTestConn(eventKey, []string{"im.v1"})
		h.RegisterAndIsFirst(conns[i])
	}

	var lastCount int32
	var wg sync.WaitGroup
	wg.Add(N)
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		c := conns[i]
		go func() {
			defer wg.Done()
			<-start
			if h.UnregisterAndIsLast(c) {
				atomic.AddInt32(&lastCount, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := atomic.LoadInt32(&lastCount); got != 1 {
		t.Errorf("UnregisterAndIsLast returned true %d times; want exactly 1", got)
	}
	if got := h.EventKeyCount(eventKey); got != 0 {
		t.Errorf("EventKeyCount after all unregister = %d, want 0", got)
	}
}

type testConn struct {
	eventKey   string
	eventTypes []string
	sendCh     chan interface{}
	pid        int
	received   atomic.Int64
}

func newTestConn(eventKey string, eventTypes []string) *testConn {
	return &testConn{
		eventKey:   eventKey,
		eventTypes: eventTypes,
		sendCh:     make(chan interface{}, 100),
		pid:        1,
	}
}

func (c *testConn) EventKey() string { return c.eventKey }

// SubscriptionID falls back to EventKey for test mocks that don't set a separate subscription ID.
func (c *testConn) SubscriptionID() string   { return c.eventKey }
func (c *testConn) EventTypes() []string     { return c.eventTypes }
func (c *testConn) SendCh() chan interface{} { return c.sendCh }
func (c *testConn) PID() int                 { return c.pid }
func (c *testConn) IncrementReceived()       { c.received.Add(1) }
func (c *testConn) Received() int64          { return c.received.Load() }

func (c *testConn) DroppedCount() int64 { return 0 }

func (c *testConn) IncrementDropped() {}

func (c *testConn) NextSeq() uint64 { return 0 }

func (c *testConn) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	select {
	case c.sendCh <- msg:
		return true, false
	default:
	}
	select {
	case <-c.sendCh:
		dropped = true
	default:
	}
	select {
	case c.sendCh <- msg:
		return true, dropped
	default:
		return false, dropped
	}
}

func (c *testConn) TrySend(msg interface{}) bool {
	select {
	case c.sendCh <- msg:
		return true
	default:
		return false
	}
}

func TestHub_SubscriptionID_Isolation(t *testing.T) {
	h := NewHub()
	c1, _ := net.Pipe()
	c2, _ := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	s1 := NewConn(c1, nil, "mail.x", []string{"mail.x"}, 1, "mail.x:alice")
	s2 := NewConn(c2, nil, "mail.x", []string{"mail.x"}, 2, "mail.x:bob")

	if !h.RegisterAndIsFirst(s1) {
		t.Error("s1 should be first for its subscription")
	}
	if !h.RegisterAndIsFirst(s2) {
		t.Error("s2 should ALSO be first (different SubscriptionID)")
	}
	if !h.UnregisterAndIsLast(s1) {
		t.Error("s1 should be last for mail.x:alice")
	}
	if !h.UnregisterAndIsLast(s2) {
		t.Error("s2 should be last for mail.x:bob")
	}
}

func TestHub_SameSubscriptionID_NotFirst(t *testing.T) {
	h := NewHub()
	c1, _ := net.Pipe()
	c2, _ := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	s1 := NewConn(c1, nil, "mail.x", []string{"mail.x"}, 1, "mail.x:alice")
	s2 := NewConn(c2, nil, "mail.x", []string{"mail.x"}, 2, "mail.x:alice")

	if !h.RegisterAndIsFirst(s1) {
		t.Error("s1 first")
	}
	if h.RegisterAndIsFirst(s2) {
		t.Error("s2 same SubscriptionID should NOT be first")
	}
}

func TestHub_EventKeyCount_AggregatesAcrossSubscriptions(t *testing.T) {
	h := NewHub()
	c1, _ := net.Pipe()
	c2, _ := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	s1 := NewConn(c1, nil, "mail.x", []string{"mail.x"}, 1, "mail.x:alice")
	s2 := NewConn(c2, nil, "mail.x", []string{"mail.x"}, 2, "mail.x:bob")
	h.RegisterAndIsFirst(s1)
	h.RegisterAndIsFirst(s2)
	if got := h.EventKeyCount("mail.x"); got != 2 {
		t.Errorf("EventKeyCount(mail.x) = %d, want 2 (aggregated across subscriptions)", got)
	}
	if got := h.SubCount("mail.x:alice"); got != 1 {
		t.Errorf("SubCount(mail.x:alice) = %d, want 1", got)
	}
	if got := h.SubCount("mail.x:bob"); got != 1 {
		t.Errorf("SubCount(mail.x:bob) = %d, want 1", got)
	}
}

func TestHub_Consumers_PopulatesSubscriptionID(t *testing.T) {
	h := NewHub()
	c1, _ := net.Pipe()
	defer c1.Close()
	s1 := NewConn(c1, nil, "mail.x", []string{"mail.x"}, 1, "mail.x:alice")
	h.RegisterAndIsFirst(s1)
	consumers := h.Consumers()
	if len(consumers) != 1 {
		t.Fatalf("got %d consumers, want 1", len(consumers))
	}
	if consumers[0].SubscriptionID != "mail.x:alice" {
		t.Errorf("Consumers()[0].SubscriptionID = %q, want %q", consumers[0].SubscriptionID, "mail.x:alice")
	}
}

func TestHub_TryRegisterExclusive(t *testing.T) {
	h := NewHub()
	first := newTestConn("k.exclusive", []string{"k.exclusive"})
	first.pid = 100
	ok, _ := h.TryRegisterExclusive(first)
	if !ok {
		t.Fatal("first exclusive register should succeed")
	}

	second := newTestConn("k.exclusive", []string{"k.exclusive"})
	second.pid = 200
	ok, reason := h.TryRegisterExclusive(second)
	if ok {
		t.Error("second exclusive register should be rejected")
	}
	if !strings.Contains(reason, "pid 100") {
		t.Errorf("reject reason = %q, want it to name existing pid 100", reason)
	}
	if got := h.SubCount("k.exclusive"); got != 1 {
		t.Errorf("SubCount = %d, want 1 (second not registered)", got)
	}
}

func TestHub_TryRegisterExclusive_CleanupWaitTimeout(t *testing.T) {
	// A cleanup lock that never releases must not wedge a new exclusive consumer
	// forever — TryRegisterExclusive bounds the wait and rejects with a timeout reason.
	saved := exclusiveCleanupWaitTimeout
	exclusiveCleanupWaitTimeout = 20 * time.Millisecond
	defer func() { exclusiveCleanupWaitTimeout = saved }()

	h := NewHub()
	first := newTestConn("k.timeout", []string{"k.timeout"})
	if ok, _ := h.TryRegisterExclusive(first); !ok {
		t.Fatal("first exclusive register should succeed")
	}
	// Hold the cleanup lock and never release it.
	if !h.AcquireCleanupLock("k.timeout") {
		t.Fatal("AcquireCleanupLock should succeed for the sole subscriber")
	}

	start := time.Now()
	second := newTestConn("k.timeout", []string{"k.timeout"})
	ok, reason := h.TryRegisterExclusive(second)
	if ok {
		t.Error("second exclusive register should be rejected on cleanup-wait timeout")
	}
	if !strings.Contains(reason, "timed out") {
		t.Errorf("reject reason = %q, want a timeout reason", reason)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("wait took %v, want bounded by the ~20ms timeout (no deadlock)", elapsed)
	}
}

func TestHub_TryRegisterExclusive_DistinctSubscriptions(t *testing.T) {
	h := NewHub()
	a := newTestConn("k.a", []string{"k.a"})
	b := newTestConn("k.b", []string{"k.b"})
	if ok, _ := h.TryRegisterExclusive(a); !ok {
		t.Fatal("register a failed")
	}
	if ok, _ := h.TryRegisterExclusive(b); !ok {
		t.Error("distinct subscription b should register")
	}
}
