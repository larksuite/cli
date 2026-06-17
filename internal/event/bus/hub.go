// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"fmt"
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/protocol"
)

// exclusiveCleanupWaitTimeout bounds how long TryRegisterExclusive waits for an
// in-progress cleanup of the same subscription before rejecting, so a stuck
// cleanup can never wedge new consumers forever. Kept below the consumer's
// hello_ack deadline (consume.helloAckTimeout = 5s) so the reject still reaches
// the consumer as a clean failed_precondition instead of a handshake timeout.
// Override with LARKSUITE_CLI_EVENT_EXCLUSIVE_WAIT_TIMEOUT (a Go duration such as
// "2s"); values at or above the 5s handshake deadline are not recommended.
var exclusiveCleanupWaitTimeout = resolveExclusiveCleanupWaitTimeout()

func resolveExclusiveCleanupWaitTimeout() time.Duration {
	const def = 3 * time.Second
	if v := os.Getenv("LARKSUITE_CLI_EVENT_EXCLUSIVE_WAIT_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// Subscriber is the interface a connection must satisfy for Hub registration.
type Subscriber interface {
	EventKey() string
	// SubscriptionID identifies the per-resource subscription for dedup purposes.
	// When no resource qualifier is needed it equals EventKey.
	SubscriptionID() string
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
	// subCounts is keyed by SubscriptionID (not EventKey) so that different
	// per-resource subscriptions sharing the same EventKey are deduped independently.
	subCounts map[string]int
	// cleanupInProgress[subscriptionID] holds a channel closed on release;
	// presence means a cleanup lock is held for that subscription.
	cleanupInProgress map[string]chan struct{}
	logger            atomic.Pointer[log.Logger]
}

func NewHub() *Hub {
	return &Hub{
		subscribers:       make(map[Subscriber]struct{}),
		subCounts:         make(map[string]int),
		cleanupInProgress: make(map[string]chan struct{}),
	}
}

// SetLogger attaches a logger (nil tolerated).
func (h *Hub) SetLogger(l *log.Logger) { h.logger.Store(l) }

// UnregisterAndIsLast removes s and reports whether it was last for its SubscriptionID; stale unregisters are no-ops.
func (h *Hub) UnregisterAndIsLast(s Subscriber) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, registered := h.subscribers[s]; !registered {
		return false
	}
	delete(h.subscribers, s)
	sid := s.SubscriptionID()
	h.subCounts[sid]--
	isLast := h.subCounts[sid] == 0
	if isLast {
		delete(h.subCounts, sid)
	}
	return isLast
}

// AcquireCleanupLock reserves cleanup rights iff exactly one subscriber exists for subscriptionID and no lock is held.
// Count==0 is rejected (would block future Register calls). On true return, caller MUST Release.
func (h *Hub) AcquireCleanupLock(subscriptionID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subCounts[subscriptionID] != 1 {
		return false
	}
	if _, alreadyLocked := h.cleanupInProgress[subscriptionID]; alreadyLocked {
		return false
	}
	h.cleanupInProgress[subscriptionID] = make(chan struct{})
	return true
}

// ReleaseCleanupLock is idempotent; OnClose calls unconditionally.
func (h *Hub) ReleaseCleanupLock(subscriptionID string) {
	h.mu.Lock()
	ch := h.cleanupInProgress[subscriptionID]
	delete(h.cleanupInProgress, subscriptionID)
	h.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

// RegisterAndIsFirst adds s to the hub and reports whether it's the first
// subscriber for its SubscriptionID. If a cleanup is in progress for
// s.SubscriptionID() (another conn holds the cleanup lock), this waits until
// cleanup releases before registering — closing the PreShutdownCheck ×
// Hello TOCTOU race. The wait releases h.mu before blocking on the
// channel, so concurrent operations on other subscriptions aren't stalled.
func (h *Hub) RegisterAndIsFirst(s Subscriber) bool {
	sid := s.SubscriptionID()
	for {
		h.mu.Lock()
		ch, locked := h.cleanupInProgress[sid]
		if locked {
			h.mu.Unlock()
			<-ch // wait for release, then re-check (defensive against races)
			continue
		}
		isFirst := h.subCounts[sid] == 0
		h.subscribers[s] = struct{}{}
		h.subCounts[sid]++
		h.mu.Unlock()
		return isFirst
	}
}

// TryRegisterExclusive registers s only when no subscriber holds s.SubscriptionID()
// and any in-progress cleanup for that subscription finishes within
// exclusiveCleanupWaitTimeout. On failure it returns (false, reason): either a
// duplicate consumer already holds the subscription, or the cleanup did not
// finish in time — the timeout guarantees a stuck cleanup can never wedge new
// consumers forever. reason is "" on success. Mirrors RegisterAndIsFirst's wait
// on in-progress cleanup, but bounded.
func (h *Hub) TryRegisterExclusive(s Subscriber) (bool, string) {
	sid := s.SubscriptionID()
	deadline := time.Now().Add(exclusiveCleanupWaitTimeout)
	for {
		h.mu.Lock()
		ch, locked := h.cleanupInProgress[sid]
		if locked {
			h.mu.Unlock()
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return false, "timed out waiting for the previous consumer's cleanup to finish; retry shortly"
			}
			timer := time.NewTimer(remaining)
			select {
			case <-ch:
				// Stop+drain so a timer that fired concurrently with Stop isn't left on .C.
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				continue
			case <-timer.C:
				return false, "timed out waiting for the previous consumer's cleanup to finish; retry shortly"
			}
		}
		if h.subCounts[sid] != 0 {
			pid := h.existingPIDForSubscriptionLocked(sid)
			h.mu.Unlock()
			return false, fmt.Sprintf("another consumer (pid %d) is already running for this subscription", pid)
		}
		h.subscribers[s] = struct{}{}
		h.subCounts[sid]++
		h.mu.Unlock()
		return true, ""
	}
}

// existingPIDForSubscriptionLocked returns the PID of one subscriber for sid.
// Caller must hold h.mu.
func (h *Hub) existingPIDForSubscriptionLocked(sid string) int {
	for sub := range h.subscribers {
		if sub.SubscriptionID() == sid {
			return sub.PID()
		}
	}
	return 0
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

// EventKeyCount returns total subscribers for the given EventKey, aggregating
// across all SubscriptionIDs. For per-subscription counts use SubCount.
func (h *Hub) EventKeyCount(eventKey string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	count := 0
	for s := range h.subscribers {
		if s.EventKey() == eventKey {
			count++
		}
	}
	return count
}

// SubCount returns the count of subscribers for the given SubscriptionID.
func (h *Hub) SubCount(subscriptionID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.subCounts[subscriptionID]
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
			PID:            s.PID(),
			EventKey:       s.EventKey(),
			SubscriptionID: s.SubscriptionID(),
			Received:       s.Received(),
			Dropped:        s.DroppedCount(),
		})
	}
	return result
}
