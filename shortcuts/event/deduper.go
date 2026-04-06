// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"sync"
	"sync/atomic"
	"time"
)

const deduperCleanupInterval = 64

// Deduper suppresses repeated keys seen within a TTL window.
type Deduper struct {
	ttl   time.Duration
	seen  sync.Map // key -> time.Time
	calls atomic.Uint64
}

// NewDeduper creates a deduper with the provided TTL.
func NewDeduper(ttl time.Duration) *Deduper {
	return &Deduper{ttl: ttl}
}

// Seen reports whether key has already been seen within ttl and records now.
func (d *Deduper) Seen(key string, now time.Time) bool {
	if d == nil || key == "" || d.ttl <= 0 {
		return false
	}
	if d.calls.Add(1)%deduperCleanupInterval == 0 {
		d.cleanup(now)
	}
	if v, loaded := d.seen.LoadOrStore(key, now); loaded {
		if ts, ok := v.(time.Time); ok && now.Sub(ts) < d.ttl {
			return true
		}
		d.seen.Store(key, now)
	}
	return false
}

func (d *Deduper) cleanup(now time.Time) {
	d.seen.Range(func(key, value any) bool {
		ts, ok := value.(time.Time)
		if ok && now.Sub(ts) >= d.ttl {
			d.seen.Delete(key)
		}
		return true
	})
}
