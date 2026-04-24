// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"sync"
	"testing"
	"time"
)

func TestDedupFilter_FirstSeen(t *testing.T) {
	d := NewDedupFilter()
	if d.IsDuplicate("evt-1") {
		t.Error("first occurrence should not be duplicate")
	}
}

func TestDedupFilter_SecondSeen(t *testing.T) {
	d := NewDedupFilter()
	d.IsDuplicate("evt-1")
	if !d.IsDuplicate("evt-1") {
		t.Error("second occurrence within TTL should be duplicate")
	}
}

func TestDedupFilter_TTLExpiry(t *testing.T) {
	d := NewDedupFilterWithSize(defaultRingSize, 10*time.Millisecond)
	d.IsDuplicate("evt-1")
	time.Sleep(20 * time.Millisecond)
	if d.IsDuplicate("evt-1") {
		t.Error("should not be duplicate after TTL expires")
	}
}

func TestDedupFilter_RingBuffer(t *testing.T) {
	d := NewDedupFilterWithSize(5, 10*time.Millisecond) // small ring + short TTL for test
	// Fill ring buffer
	for i := 0; i < 5; i++ {
		d.IsDuplicate("evt-" + string(rune('a'+i)))
	}
	// All 5 should be duplicates (still in TTL map + ring)
	for i := 0; i < 5; i++ {
		if !d.IsDuplicate("evt-" + string(rune('a'+i))) {
			t.Errorf("evt-%c should still be duplicate", rune('a'+i))
		}
	}
	// Wait for TTL to expire
	time.Sleep(20 * time.Millisecond)
	// Push 5 more, evicting the first 5 from ring
	for i := 5; i < 10; i++ {
		d.IsDuplicate("evt-" + string(rune('a'+i)))
	}
	// After eviction + TTL expiry, first 5 should no longer be duplicates
	for i := 0; i < 5; i++ {
		if d.IsDuplicate("evt-" + string(rune('a'+i))) {
			t.Errorf("evt-%c should not be duplicate after ring eviction + TTL expiry", rune('a'+i))
		}
	}
}

func TestDedupFilter_ConcurrentSafe(t *testing.T) {
	d := NewDedupFilter()
	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func(id string) {
			d.IsDuplicate(id)
			done <- struct{}{}
		}("evt-" + string(rune(i)))
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestDedupFilter_ConcurrentFirstSeenExactlyOnce asserts the stronger
// invariant the existing "ConcurrentSafe" test fails to check: under
// N concurrent writers, exactly N IsDuplicate calls observe "first
// seen" (returned false) across the set of unique IDs — not N-1 (a
// lost update) and not N+1 (unlikely but would mean the map + ring
// drift). Without this, a buggy IsDuplicate that silently dropped
// writes would still pass ConcurrentSafe.
func TestDedupFilter_ConcurrentFirstSeenExactlyOnce(t *testing.T) {
	const n = 200
	d := NewDedupFilter()

	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = "evt-unique-" + string(rune('A'+i%26)) + string(rune('a'+(i/26)%26)) + string(rune('0'+i%10))
	}

	results := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func(id string) {
			results <- d.IsDuplicate(id) // false on first-seen
		}(ids[i])
	}

	firstSeen := 0
	for i := 0; i < n; i++ {
		if !<-results {
			firstSeen++
		}
	}
	if firstSeen != n {
		t.Errorf("first-seen count = %d, want %d (IsDuplicate lost a write under contention)", firstSeen, n)
	}

	// Follow-up: every ID must now read as duplicate, even after the
	// concurrent burst. Catches a class of bug where the map write and
	// ring update race and one side loses.
	for _, id := range ids {
		if !d.IsDuplicate(id) {
			t.Errorf("ID %q not flagged as duplicate on second call (map/ring inconsistency)", id)
			break
		}
	}
}

// TestDedupFilter_TTLExpiryAfterCleanupRunRespected guards the bug where
// cleanupExpired (fired every 1000 inserts once d.pos wraps to 0) removed
// TTL-expired IDs from seen but left them in the ring — a prior revision
// of IsDuplicate then fell back to a ring scan, found the stale ID, and
// incorrectly returned true, causing the bus to silently drop a legitimate
// (post-TTL, not-actually-duplicate) event. With ring scan removed, the
// seen map is the sole authority and TTL expiry really means "re-accept".
func TestDedupFilter_TTLExpiryAfterCleanupRunRespected(t *testing.T) {
	// ringSize=10 so d.pos wraps to 0 on the 10th insert, triggering
	// cleanupExpired there. TTL=10ms so "A" is expired by the time
	// cleanupExpired scans.
	d := NewDedupFilterWithSize(10, 10*time.Millisecond)
	if d.IsDuplicate("A") {
		t.Fatal("first IsDuplicate(A) should be false")
	}
	// Let A's TTL elapse before the fillers run, so cleanupExpired deletes
	// seen[A]. Fillers themselves are inserted rapidly so their TTL hasn't
	// expired — only A's has.
	time.Sleep(25 * time.Millisecond)
	for i := 0; i < 9; i++ {
		d.IsDuplicate("f" + string(rune('0'+i)))
	}
	// After the 10th total insert: cleanupExpired has removed seen[A];
	// ring[0] still contains "A" (ring hasn't wrapped far enough to
	// overwrite slot 0 yet — ring[0] only gets rewritten on insert 11).
	if d.IsDuplicate("A") {
		t.Error("A is past TTL — must NOT be reported as duplicate, " +
			"even though the ring still carries it")
	}
}

// TestDedupFilter_ConcurrentRingEviction exercises the ring's eviction
// path under concurrent writers. With ringSize=16 and 100 unique IDs,
// eviction runs continuously; the invariant is "once evicted and TTL
// expired, ID is no longer considered duplicate" — no deadlock, no
// panic, no wedged-state.
func TestDedupFilter_ConcurrentRingEviction(t *testing.T) {
	const ringSize = 16
	const writers = 8
	const perWriter = 40
	d := NewDedupFilterWithSize(ringSize, 5*time.Millisecond)

	var wg sync.WaitGroup
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				d.IsDuplicate("evt-w" + string(rune('0'+w)) + "-" + string(rune('0'+i%10)) + string(rune('a'+i/10)))
			}
		}(w)
	}
	wg.Wait()

	// After TTL expires and more writes push them out of the ring,
	// early IDs must be re-acceptable as first-seen.
	time.Sleep(10 * time.Millisecond)
	for i := 0; i < ringSize*4; i++ {
		d.IsDuplicate("evt-fill-" + string(rune('0'+i%10)) + string(rune('a'+i/10)))
	}
	// This old ID should have been evicted from both map (TTL) and ring (capacity).
	if d.IsDuplicate("evt-w0-0a") {
		t.Error("evicted ID should not be reported as duplicate")
	}
}
