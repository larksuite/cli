// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// Subscriber is the interface a connection must satisfy for Hub registration.
type Subscriber interface {
	EventKey() string
	EventTypes() []string
	SendCh() chan interface{}
	PID() int
	IncrementReceived()
	Received() int64
	// PushDropOldest enqueues msg with drop-oldest backpressure atomically.
	// Returns enqueued=true iff msg is now in the channel, and dropped=true
	// iff an older event was evicted to make room. Implementations must
	// serialise drop+push so concurrent Publish callers cannot race each
	// other into a state where the channel refills between the drop and the
	// retry push.
	PushDropOldest(msg interface{}) (enqueued, dropped bool)
	// DroppedCount returns the total number of events evicted via the
	// drop-oldest path on this subscriber's send channel, surfaced to the
	// status command via ConsumerInfo.Dropped.
	DroppedCount() int64
	// IncrementDropped records that one event was evicted via the drop-oldest
	// path. Hub.Publish calls this when PushDropOldest reports dropped=true.
	IncrementDropped()
	// NextSeq returns a monotonically increasing sequence number for events
	// destined for this subscriber. Hub.Publish stamps the returned value on
	// protocol.Event.Seq so the consumer side can detect gaps from drops.
	// Implementations that do not care about sequence numbers (e.g. tests)
	// may return 0.
	NextSeq() uint64
}

// Hub manages event fan-out to registered subscribers.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[Subscriber]struct{}
	keyCounts   map[string]int
	// cleanupInProgress maps an EventKey to a "release" channel that is
	// closed when cleanup finishes. Presence of a key means a cleanup lock
	// is currently held; RegisterAndIsFirst waits on the channel to avoid
	// the PreShutdownCheck × Hello TOCTOU race.
	cleanupInProgress map[string]chan struct{}
	// logger is read from Publish (per-event hot path) and written by
	// SetLogger. atomic.Pointer avoids a race even though in today's
	// wiring SetLogger fires before any source goroutine starts — the
	// guarantee becomes an invariant of the type, not of the caller.
	logger atomic.Pointer[log.Logger]
}

// NewHub returns a freshly initialised Hub with no subscribers.
func NewHub() *Hub {
	return &Hub{
		subscribers:       make(map[Subscriber]struct{}),
		keyCounts:         make(map[string]int),
		cleanupInProgress: make(map[string]chan struct{}),
	}
}

// SetLogger attaches a logger for backpressure diagnostics. nil is tolerated.
func (h *Hub) SetLogger(l *log.Logger) { h.logger.Store(l) }

// UnregisterAndIsLast removes s and returns whether s was the last
// subscriber for s.EventKey() at the moment of removal. Pairs with
// RegisterAndIsFirst for atomic lifecycle decisions.
func (h *Hub) UnregisterAndIsLast(s Subscriber) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, s)
	if h.keyCounts[s.EventKey()] > 0 {
		h.keyCounts[s.EventKey()]--
	}
	isLast := h.keyCounts[s.EventKey()] == 0
	if isLast {
		delete(h.keyCounts, s.EventKey())
	}
	return isLast
}

// AcquireCleanupLock is the race-free replacement for "check last then
// cleanup". Returns true iff this subscriber is still the only one
// registered for eventKey AND no other cleanup is already in progress. On
// true return, caller MUST eventually call ReleaseCleanupLock to unblock
// any waiting RegisterAndIsFirst. Both checks (count <= 1 and
// already-locked) run under the same write lock so they are atomic —
// preventing a late-arriving Hello from slipping in between the check and
// the reservation.
func (h *Hub) AcquireCleanupLock(eventKey string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.keyCounts[eventKey] > 1 {
		return false
	}
	if _, alreadyLocked := h.cleanupInProgress[eventKey]; alreadyLocked {
		return false
	}
	h.cleanupInProgress[eventKey] = make(chan struct{})
	return true
}

// ReleaseCleanupLock signals that cleanup for eventKey has finished. Any
// RegisterAndIsFirst calls waiting on this key will proceed. Safe to call
// even if no lock is held (no-op in that case), so OnClose can call it
// unconditionally on every disconnect path.
func (h *Hub) ReleaseCleanupLock(eventKey string) {
	h.mu.Lock()
	ch := h.cleanupInProgress[eventKey]
	delete(h.cleanupInProgress, eventKey)
	h.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// RegisterAndIsFirst adds s to the hub and reports whether it's the first
// subscriber for its EventKey. If a cleanup is in progress for
// s.EventKey() (another conn holds the cleanup lock), this waits until
// cleanup releases before registering — closing the PreShutdownCheck ×
// Hello TOCTOU race. The wait releases h.mu before blocking on the
// channel, so concurrent operations on other keys aren't stalled.
func (h *Hub) RegisterAndIsFirst(s Subscriber) bool {
	for {
		h.mu.Lock()
		ch, locked := h.cleanupInProgress[s.EventKey()]
		if locked {
			h.mu.Unlock()
			<-ch // wait for release, then re-check (defensive against races)
			continue
		}
		isFirst := h.keyCounts[s.EventKey()] == 0
		h.subscribers[s] = struct{}{}
		h.keyCounts[s.EventKey()]++
		h.mu.Unlock()
		return isFirst
	}
}

// Publish fans out a RawEvent to all matching subscribers (non-blocking).
//
// A fresh *protocol.Event is allocated per subscriber so each consumer sees
// its own monotonically-increasing Seq (assigned via Conn.NextSeq) — sharing
// a single msg struct across subscribers would alias Seq and defeat the
// gap-detection at the consume side. The extra allocation per fan-out is
// cheap compared to the socket write that follows.
func (h *Hub) Publish(raw *event.RawEvent) {
	h.mu.RLock()
	matches := make([]Subscriber, 0, len(h.subscribers))
	for s := range h.subscribers {
		for _, et := range s.EventTypes() {
			if et == raw.EventType {
				matches = append(matches, s)
				break
			}
		}
	}
	h.mu.RUnlock()

	// Resolve source time once per Publish (not per subscriber) — same value
	// across the fan-out. Prefer the upstream header create_time
	// (raw.SourceTime) over the local arrival timestamp so consumers see
	// original publisher intent; fall back to Timestamp when SourceTime
	// wasn't populated (e.g. test-only sources, pre-4.4 RawEvent producers).
	sourceTime := raw.SourceTime
	if sourceTime == "" && !raw.Timestamp.IsZero() {
		sourceTime = fmt.Sprintf("%d", raw.Timestamp.UnixMilli())
	}

	for _, s := range matches {
		msg := protocol.NewEvent(
			raw.EventType,
			raw.EventID,
			sourceTime,
			s.NextSeq(),
			raw.Payload,
		)

		enqueued, dropped := s.PushDropOldest(msg)
		if dropped {
			s.IncrementDropped()
			if lg := h.logger.Load(); lg != nil {
				lg.Printf("WARN: backpressure on conn pid=%d event_key=%s dropped_total=%d",
					s.PID(), s.EventKey(), s.DroppedCount())
			}
		}
		if enqueued {
			s.IncrementReceived()
		}
	}
}

// ConnCount returns the current number of registered subscribers.
func (h *Hub) ConnCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers)
}

// EventKeyCount returns the number of subscribers registered for eventKey.
func (h *Hub) EventKeyCount(eventKey string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.keyCounts[eventKey]
}

// BroadcastSourceStatus fans out a source-level status change to every
// subscriber. Best-effort: channel full → drop silently (status isn't
// worth applying back-pressure for).
func (h *Hub) BroadcastSourceStatus(source, state, detail string) {
	msg := protocol.NewSourceStatus(source, state, detail)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subscribers {
		select {
		case s.SendCh() <- msg:
		default:
		}
	}
}

// Consumers returns info about all connected consumers.
func (h *Hub) Consumers() []protocol.ConsumerInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	result := make([]protocol.ConsumerInfo, 0, len(h.subscribers))
	for s := range h.subscribers {
		result = append(result, protocol.ConsumerInfo{
			PID:      s.PID(),
			EventKey: s.EventKey(),
			Received: s.Received(),
			Dropped:  s.DroppedCount(),
		})
	}
	return result
}
