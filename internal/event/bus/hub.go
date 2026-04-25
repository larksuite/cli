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
	// PushDropOldest enqueues atomically with drop-oldest backpressure.
	PushDropOldest(msg interface{}) (enqueued, dropped bool)
	// TrySend is non-evictive but shares PushDropOldest's mutex.
	TrySend(msg interface{}) bool
	DroppedCount() int64
	IncrementDropped()
	// NextSeq returns a monotonic per-subscriber seq; tests may return 0.
	NextSeq() uint64
}

type Hub struct {
	mu          sync.RWMutex
	subscribers map[Subscriber]struct{}
	keyCounts   map[string]int
	// cleanupInProgress[key] holds a channel closed on release; presence means a cleanup lock is held.
	cleanupInProgress map[string]chan struct{}
	logger            atomic.Pointer[log.Logger]
}

func NewHub() *Hub {
	return &Hub{
		subscribers:       make(map[Subscriber]struct{}),
		keyCounts:         make(map[string]int),
		cleanupInProgress: make(map[string]chan struct{}),
	}
}

// SetLogger attaches a logger (nil tolerated).
func (h *Hub) SetLogger(l *log.Logger) { h.logger.Store(l) }

// UnregisterAndIsLast removes s and reports whether it was last for its EventKey; stale unregisters are no-ops.
func (h *Hub) UnregisterAndIsLast(s Subscriber) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subscribers[s]; !registered {
		return false
	}
	delete(h.subscribers, s)
	h.keyCounts[s.EventKey()]--
	isLast := h.keyCounts[s.EventKey()] == 0
	if isLast {
		delete(h.keyCounts, s.EventKey())
	}
	return isLast
}

// AcquireCleanupLock reserves cleanup rights iff exactly one subscriber exists for eventKey and no lock is held.
// Count==0 is rejected (would block future Register calls). On true return, caller MUST Release.
func (h *Hub) AcquireCleanupLock(eventKey string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.keyCounts[eventKey] != 1 {
		return false
	}
	if _, alreadyLocked := h.cleanupInProgress[eventKey]; alreadyLocked {
		return false
	}
	h.cleanupInProgress[eventKey] = make(chan struct{})
	return true
}

// ReleaseCleanupLock is idempotent; OnClose calls unconditionally.
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
// worth applying back-pressure for). Routes through Subscriber.TrySend
// so the send shares PushDropOldest's sendMu — without this a status
// broadcast could slip into the tiny window between another
// goroutine's drop and its retry push and break the atomicity contract.
func (h *Hub) BroadcastSourceStatus(source, state, detail string) {
	msg := protocol.NewSourceStatus(source, state, detail)
	h.mu.RLock()
	defer h.mu.RUnlock()
	for s := range h.subscribers {
		s.TrySend(msg)
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
