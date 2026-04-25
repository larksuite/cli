// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

// Under concurrent Publish with a tiny channel, Received must equal actual enqueues (sendMu + enqueued gate).
func TestPublishRaceBookkeepingAccurate(t *testing.T) {
	h := NewHub()
	sub := newRaceSubscriber("race.key", []string{"race.type"}, 2)
	h.RegisterAndIsFirst(sub)

	const publishers = 50
	const perPublisher = 500
	const N = publishers * perPublisher

	var wg sync.WaitGroup
	for i := 0; i < publishers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perPublisher; j++ {
				h.Publish(&event.RawEvent{
					EventType: "race.type",
					Payload:   json.RawMessage(`{}`),
				})
			}
		}()
	}
	const trySenders = 20
	for i := 0; i < trySenders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perPublisher; j++ {
				sub.TrySend("source-status")
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("publishers did not complete in 10s")
	}

	received := sub.Received()
	enqueued := atomic.LoadInt64(&sub.actualEnqueued)
	returnedFalse := atomic.LoadInt64(&sub.returnedFalse)

	if received != enqueued {
		t.Errorf("counter drift: Received=%d actual_enqueued=%d (diff=%d)",
			received, enqueued, received-enqueued)
	}

	if received > int64(N) {
		t.Errorf("Received=%d > N=%d", received, N)
	}

	if returnedFalse > 0 {
		t.Errorf("PushDropOldest returned enqueued=false %d times — sendMu missing or broken",
			returnedFalse)
	}

	totalPublishes := int64(N)
	if enqueued+returnedFalse != totalPublishes {
		t.Errorf("publish accounting drift: enqueued=%d + returnedFalse=%d != total=%d",
			enqueued, returnedFalse, totalPublishes)
	}
}

// Hub.Publish must gate IncrementReceived on enqueued=true.
func TestPublishDoesNotIncrementWhenPushDropOldestFails(t *testing.T) {
	h := NewHub()
	sub := &alwaysFailSubscriber{
		eventKey:   "fail.key",
		eventTypes: []string{"fail.type"},
		sendCh:     make(chan interface{}, 1),
	}
	h.RegisterAndIsFirst(sub)

	for i := 0; i < 100; i++ {
		h.Publish(&event.RawEvent{
			EventType: "fail.type",
			Payload:   json.RawMessage(`{}`),
		})
	}

	if got := sub.Received(); got != 0 {
		t.Errorf("Received=%d after 100 Publishes that all failed to enqueue", got)
	}
}

type alwaysFailSubscriber struct {
	eventKey   string
	eventTypes []string
	sendCh     chan interface{}
	received   atomic.Int64
	dropped    atomic.Int64
}

func (s *alwaysFailSubscriber) EventKey() string         { return s.eventKey }
func (s *alwaysFailSubscriber) EventTypes() []string     { return s.eventTypes }
func (s *alwaysFailSubscriber) SendCh() chan interface{} { return s.sendCh }
func (s *alwaysFailSubscriber) PID() int                 { return 0 }
func (s *alwaysFailSubscriber) IncrementReceived()       { s.received.Add(1) }
func (s *alwaysFailSubscriber) Received() int64          { return s.received.Load() }
func (s *alwaysFailSubscriber) DroppedCount() int64      { return s.dropped.Load() }
func (s *alwaysFailSubscriber) IncrementDropped()        { s.dropped.Add(1) }
func (s *alwaysFailSubscriber) NextSeq() uint64          { return 0 }
func (s *alwaysFailSubscriber) TrySend(msg interface{}) bool {
	select {
	case s.sendCh <- msg:
		return true
	default:
		return false
	}
}
func (s *alwaysFailSubscriber) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	return false, false
}

type raceSubscriber struct {
	eventKey       string
	eventTypes     []string
	sendCh         chan interface{}
	pid            int
	received       atomic.Int64
	actualEnqueued int64
	returnedFalse  int64
	dropped        atomic.Int64
	sendMu         sync.Mutex
}

func newRaceSubscriber(key string, types []string, capacity int) *raceSubscriber {
	return &raceSubscriber{
		eventKey:   key,
		eventTypes: types,
		sendCh:     make(chan interface{}, capacity),
		pid:        1,
	}
}

func (s *raceSubscriber) EventKey() string         { return s.eventKey }
func (s *raceSubscriber) EventTypes() []string     { return s.eventTypes }
func (s *raceSubscriber) SendCh() chan interface{} { return s.sendCh }
func (s *raceSubscriber) PID() int                 { return s.pid }
func (s *raceSubscriber) IncrementReceived()       { s.received.Add(1) }
func (s *raceSubscriber) Received() int64          { return s.received.Load() }
func (s *raceSubscriber) DroppedCount() int64      { return s.dropped.Load() }
func (s *raceSubscriber) IncrementDropped()        { s.dropped.Add(1) }
func (s *raceSubscriber) NextSeq() uint64          { return 0 }

func (s *raceSubscriber) TrySend(msg interface{}) bool {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	select {
	case s.sendCh <- msg:
		return true
	default:
		return false
	}
}

func (s *raceSubscriber) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	select {
	case s.sendCh <- msg:
		atomic.AddInt64(&s.actualEnqueued, 1)
		return true, false
	default:
	}
	select {
	case <-s.sendCh:
		dropped = true
	default:
	}
	select {
	case s.sendCh <- msg:
		atomic.AddInt64(&s.actualEnqueued, 1)
		return true, dropped
	default:
		atomic.AddInt64(&s.returnedFalse, 1)
		return false, dropped
	}
}
