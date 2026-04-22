package bus

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/event"
)

// TestPublishRaceBookkeepingAccurate verifies that under concurrent Publish
// calls with a tiny send channel, the Received counter never drifts above
// the number of events actually enqueued. The pre-fix code handled full
// channels via two independent select statements:
//
//	select { case <-sendCh: default: }         // A2: drop oldest
//	select { case sendCh <- msg: default: }    // A3: push new
//	s.IncrementReceived()                      // unconditional
//
// Between A2 and A3 another Publish goroutine can refill the slot, causing
// A3 to hit default and silently drop the message — but Received is still
// incremented. With the fix, PushDropOldest holds a per-subscriber mutex
// across drop+push and Received is only incremented when enqueued == true.
//
// Strategy: use a test subscriber that counts actual enqueues in its
// PushDropOldest implementation. With the fix wired through Hub.Publish,
// IncrementReceived is gated by enqueued, so Received == actual_enqueues
// exactly. If Hub.Publish reverts to the old pattern (ignoring the return
// values and always calling IncrementReceived), the test observes
// Received > actual_enqueues under contention.
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

	// Layer-B invariant (Hub.Publish gates IncrementReceived on enqueued):
	//   Received == actual_enqueues
	// Hub.Publish must increment Received iff PushDropOldest returned
	// enqueued=true. A subscriber whose PushDropOldest tracks true enqueues
	// internally must have Received exactly equal to that count.
	//
	// Under the buggy two-select pattern in Hub.Publish (no PushDropOldest
	// gate), Received is incremented unconditionally, so under contention
	// Received > actual_enqueues whenever any A3 push fell through to default.
	if received != enqueued {
		t.Errorf("counter drift: Received=%d actual_enqueued=%d (diff=%d) "+
			"-- Hub.Publish is incrementing Received for pushes that did not enqueue",
			received, enqueued, received-enqueued)
	}

	// Also sanity-check Received never exceeds total Publish count.
	if received > int64(N) {
		t.Errorf("Received=%d > N=%d -- Received exceeds total Publish calls", received, N)
	}

	// Layer-A sensitivity (PushDropOldest holds sendMu across drop+push):
	// under the fix, PushDropOldest should never return enqueued=false in
	// this test — no SenderLoop is draining the channel, so either the fast
	// path succeeds (room available), the slow path drops one and refills
	// (we made room under the lock), or the slow-path fast-path succeeds
	// (empty-on-entry, other goroutines blocked on sendMu). The final
	// default branch of PushDropOldest is effectively unreachable under
	// sendMu with no external drainer.
	//
	// If sendMu is removed, the classic A2/A3 race becomes visible: between
	// one goroutine's drop (select <-sendCh) and its push (select sendCh <- msg),
	// another goroutine races in and refills, so the first's push hits the
	// default branch and returns (false, dropped). returnedFalse > 0 is a
	// direct witness that sendMu protection is missing or broken.
	if returnedFalse > 0 {
		t.Errorf("PushDropOldest returned enqueued=false %d times — sendMu missing or broken; A2/A3 race not fixed",
			returnedFalse)
	}

	// Conservation of Publish calls: every Publish reaches PushDropOldest
	// exactly once (filter match is 1:1 here), so each call must resolve
	// either to an actual enqueue or to a returned-false. Any drift here
	// signals lost accounting somewhere in the path.
	totalPublishes := int64(N)
	if enqueued+returnedFalse != totalPublishes {
		t.Errorf("publish accounting drift: enqueued=%d + returnedFalse=%d != total=%d",
			enqueued, returnedFalse, totalPublishes)
	}
}

// TestPublishDoesNotIncrementWhenPushDropOldestFails is a direct, non-racy
// check that Hub.Publish gates IncrementReceived on the enqueued flag. It
// uses a stub subscriber that always returns enqueued=false. If Hub.Publish
// ever drops the `if enqueued` guard (layer-B bug), Received will go up
// even though no events actually landed. This catches the subtle variant
// that the stress test above may miss because production PushDropOldest's
// sendMu makes enqueued=false essentially unreachable in practice.
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
		t.Errorf("Received=%d after 100 Publishes that all failed to enqueue; "+
			"Hub.Publish is ignoring the enqueued flag (layer-B bug)", got)
	}
}

// alwaysFailSubscriber implements Subscriber and always reports PushDropOldest
// as failing. Used to verify Hub.Publish honours the enqueued return value.
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
func (s *alwaysFailSubscriber) PushDropOldest(msg interface{}) (enqueued, dropped bool) {
	return false, false
}

// raceSubscriber implements Subscriber and tracks actual enqueues (vs.
// IncrementReceived calls) to let tests detect counter drift. The sendCh
// is deliberately tiny and never drained so PushDropOldest must hit the
// drop-and-retry path on every Publish after the first few.
type raceSubscriber struct {
	eventKey       string
	eventTypes     []string
	sendCh         chan interface{}
	pid            int
	received       atomic.Int64
	actualEnqueued int64        // incremented only when PushDropOldest actually enqueues
	returnedFalse  int64        // incremented each time PushDropOldest returns enqueued=false
	dropped        atomic.Int64 // incremented each time PushDropOldest evicts an older msg
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

// PushDropOldest mirrors the production Conn.PushDropOldest semantics and
// additionally tracks actual enqueues so the test can compare them to
// Received. Dropped accounting is driven by Hub.Publish via IncrementDropped
// (matching production), so this method does not touch s.dropped directly —
// it just signals dropped=true in the return value.
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
