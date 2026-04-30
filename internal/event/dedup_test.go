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
	d := NewDedupFilterWithSize(5, 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		d.IsDuplicate("evt-" + string(rune('a'+i)))
	}
	for i := 0; i < 5; i++ {
		if !d.IsDuplicate("evt-" + string(rune('a'+i))) {
			t.Errorf("evt-%c should still be duplicate", rune('a'+i))
		}
	}
	time.Sleep(20 * time.Millisecond)
	for i := 5; i < 10; i++ {
		d.IsDuplicate("evt-" + string(rune('a'+i)))
	}
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

// Under N concurrent writers, exactly N IsDuplicate calls must observe first-seen.
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
			results <- d.IsDuplicate(id)
		}(ids[i])
	}

	firstSeen := 0
	for i := 0; i < n; i++ {
		if !<-results {
			firstSeen++
		}
	}
	if firstSeen != n {
		t.Errorf("first-seen count = %d, want %d", firstSeen, n)
	}

	for _, id := range ids {
		if !d.IsDuplicate(id) {
			t.Errorf("ID %q not flagged as duplicate on second call", id)
			break
		}
	}
}

// Reinserting an ID that already occupies its own ring slot must not delete the fresh seen entry.
func TestDedupFilter_SelfEvictionPreservesFreshEntry(t *testing.T) {
	d := NewDedupFilterWithSize(2, time.Hour)
	d.ring[0] = "X"
	d.pos = 0

	if d.IsDuplicate("X") {
		t.Fatal("first call should not be duplicate (seen empty)")
	}
	if !d.IsDuplicate("X") {
		t.Error("self-slot reinsert wiped seen[X] — duplicate signal lost")
	}
}

// After cleanupExpired, an ID past its TTL must not be reported as duplicate even if still in the ring.
func TestDedupFilter_TTLExpiryAfterCleanupRunRespected(t *testing.T) {
	d := NewDedupFilterWithSize(10, 10*time.Millisecond)
	if d.IsDuplicate("A") {
		t.Fatal("first IsDuplicate(A) should be false")
	}
	time.Sleep(25 * time.Millisecond)
	for i := 0; i < 9; i++ {
		d.IsDuplicate("f" + string(rune('0'+i)))
	}
	if d.IsDuplicate("A") {
		t.Error("A is past TTL — must NOT be reported as duplicate")
	}
}

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

	time.Sleep(10 * time.Millisecond)
	for i := 0; i < ringSize*4; i++ {
		d.IsDuplicate("evt-fill-" + string(rune('0'+i%10)) + string(rune('a'+i/10)))
	}
	if d.IsDuplicate("evt-w0-0a") {
		t.Error("evicted ID should not be reported as duplicate")
	}
}
