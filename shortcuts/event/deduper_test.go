// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"testing"
	"time"
)

func deduperHasKey(d *Deduper, key string) bool {
	if d == nil {
		return false
	}
	found := false
	d.seen.Range(func(k, _ any) bool {
		if k == key {
			found = true
			return false
		}
		return true
	})
	return found
}

func TestDeduperEvictsExpiredKeysDuringSteadyState(t *testing.T) {
	d := NewDeduper(time.Second)
	now := time.Unix(100, 0).UTC()
	if d.Seen("stale", now) {
		t.Fatal("first observation of stale key should not dedupe")
	}

	later := now.Add(2 * time.Second)
	for i := 0; i < 128; i++ {
		if d.Seen(fmt.Sprintf("fresh-%03d", i), later) {
			t.Fatalf("fresh key %d should not dedupe on first observation", i)
		}
	}

	if deduperHasKey(d, "stale") {
		t.Fatal("stale key should be evicted after periodic cleanup")
	}
}
