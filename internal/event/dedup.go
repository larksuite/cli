// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"sync"
	"time"
)

const (
	defaultDedupTTL = 5 * time.Minute
	defaultRingSize = 10000
)

// DedupFilter provides dual-layer deduplication: TTL map + ring buffer.
type DedupFilter struct {
	seen map[string]time.Time
	ring []string
	pos  int
	ttl  time.Duration
	mu   sync.Mutex
}

// NewDedupFilter creates a DedupFilter with default settings.
func NewDedupFilter() *DedupFilter {
	return NewDedupFilterWithSize(defaultRingSize, defaultDedupTTL)
}

// NewDedupFilterWithSize creates a DedupFilter with custom ring size and TTL.
func NewDedupFilterWithSize(ringSize int, ttl time.Duration) *DedupFilter {
	return &DedupFilter{
		seen: make(map[string]time.Time),
		ring: make([]string, ringSize),
		ttl:  ttl,
	}
}

// IsDuplicate returns true if eventID has been seen within TTL. The seen
// map is the sole authority for duplicate decisions; the ring buffer only
// bounds map size via overflow eviction (line 71 below).
//
// Earlier revisions also scanned the ring as a fallback when the map lookup
// missed. That scan introduced a correctness bug: after cleanupExpired runs
// (triggered every 1000 inserts) it removes TTL-expired IDs from seen but
// leaves them in the ring. A follow-up IsDuplicate on such an ID would miss
// the map, find the ring entry, and incorrectly flag it as duplicate — a
// valid event already past its TTL was then silently dropped by
// internal/event/bus.Publish's dedup check. The ring-scan branch is gone;
// TTL expiry now always means "treat as first-seen, record again".
func (d *DedupFilter) IsDuplicate(eventID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	if ts, ok := d.seen[eventID]; ok {
		if now.Sub(ts) < d.ttl {
			return true
		}
		// TTL expired: treat as not seen, remove stale entry and re-record below.
		delete(d.seen, eventID)
	}

	// Not seen (or TTL expired) — record it.
	d.seen[eventID] = now

	// Evict oldest entry from ring if the slot is occupied.
	if old := d.ring[d.pos]; old != "" {
		delete(d.seen, old)
	}
	d.ring[d.pos] = eventID
	d.pos = (d.pos + 1) % len(d.ring)

	// Periodic cleanup of expired TTL entries (every 1000 inserts).
	if d.pos%1000 == 0 {
		d.cleanupExpired(now)
	}

	return false
}

func (d *DedupFilter) cleanupExpired(now time.Time) {
	for id, ts := range d.seen {
		if now.Sub(ts) >= d.ttl {
			delete(d.seen, id)
		}
	}
}
