// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"testing"
	"time"
)

// TestConcurrentPreShutdownAndHelloRaceFree verifies that once a subscriber
// has acquired the cleanup lock for its key, a concurrent Hello registration
// for the same key blocks until cleanup releases. This prevents the bug
// where consume B subscribes during consume A's cleanup window and ends up
// silently black-holed when A's cleanup tears down the upstream subscription.
func TestConcurrentPreShutdownAndHelloRaceFree(t *testing.T) {
	h := NewHub()
	subA := newTestConn("mail.key", []string{"mail.receive"})
	subA.pid = 1001
	h.RegisterAndIsFirst(subA)

	// A acquires cleanup lock (simulating PreShutdownCheck -> ack true).
	if !h.AcquireCleanupLock("mail.key") {
		t.Fatal("A should acquire cleanup lock — it's the only subscriber")
	}

	// While A's cleanup is "in progress", B tries to register for same key.
	subB := newTestConn("mail.key", []string{"mail.receive"})
	subB.pid = 1002

	registered := make(chan bool, 1)
	go func() {
		isFirst := h.RegisterAndIsFirst(subB)
		registered <- isFirst
	}()

	// Register MUST NOT return while cleanup lock is held.
	select {
	case <-registered:
		t.Fatal("B registered DURING A's cleanup — TOCTOU race not fixed")
	case <-time.After(200 * time.Millisecond):
		// Good — B is blocked.
	}

	// Release cleanup lock. B should proceed.
	h.ReleaseCleanupLock("mail.key")

	select {
	case isFirst := <-registered:
		// After A's unregister happens in real flow, B would be first. In this
		// synthetic test we didn't unregister A, so B sees isFirst=false. Just
		// assert that B registered — isFirst semantics are covered elsewhere.
		_ = isFirst
	case <-time.After(500 * time.Millisecond):
		t.Fatal("B never registered after cleanup released")
	}
}

// TestAcquireCleanupLockRejectsIfMultipleSubscribers verifies that
// AcquireCleanupLock returns false when more than one subscriber is
// registered for the key — in that case there's no "last subscriber" to
// reserve cleanup for.
func TestAcquireCleanupLockRejectsIfMultipleSubscribers(t *testing.T) {
	h := NewHub()
	subA := newTestConn("shared.key", []string{"t"})
	subA.pid = 1
	subB := newTestConn("shared.key", []string{"t"})
	subB.pid = 2
	h.RegisterAndIsFirst(subA)
	h.RegisterAndIsFirst(subB)

	if h.AcquireCleanupLock("shared.key") {
		t.Fatal("AcquireCleanupLock should reject when >1 subscribers exist")
	}
}

// TestAcquireCleanupLockRejectsIfAlreadyLocked ensures the lock is
// exclusive — only one subscriber can hold the cleanup reservation at a time.
func TestAcquireCleanupLockRejectsIfAlreadyLocked(t *testing.T) {
	h := NewHub()
	sub := newTestConn("exclusive.key", []string{"t"})
	sub.pid = 1
	h.RegisterAndIsFirst(sub)

	if !h.AcquireCleanupLock("exclusive.key") {
		t.Fatal("first acquire should succeed")
	}
	if h.AcquireCleanupLock("exclusive.key") {
		t.Fatal("second acquire should fail — already locked")
	}

	h.ReleaseCleanupLock("exclusive.key")
	if !h.AcquireCleanupLock("exclusive.key") {
		t.Fatal("re-acquire after release should succeed")
	}
}

// TestReleaseCleanupLockIsIdempotent ensures calling Release without a prior
// Acquire is a no-op (doesn't panic).
func TestReleaseCleanupLockIsIdempotent(t *testing.T) {
	h := NewHub()
	h.ReleaseCleanupLock("never.locked.key") // should not panic
	h.ReleaseCleanupLock("never.locked.key") // still should not panic
}

// TestAcquireCleanupLockRejectsIfZeroSubscribers guards the count==0 hole:
// a bogus or duplicate PreShutdownCheck for a key with no live subscriber
// must NOT be granted a cleanup lock. Granting it would install a
// reservation for a key nobody owns, distorting last-subscriber
// bookkeeping and blocking any future RegisterAndIsFirst for that key
// until something happened to release it.
func TestAcquireCleanupLockRejectsIfZeroSubscribers(t *testing.T) {
	h := NewHub()

	// Never-registered key: count is 0, must reject.
	if h.AcquireCleanupLock("never.registered.key") {
		t.Error("AcquireCleanupLock should reject for a never-registered key (count==0)")
	}

	// Register then unregister: count returns to 0 (and the key entry
	// gets deleted by UnregisterAndIsLast). A late PreShutdownCheck from
	// an already-gone peer must still not slip through.
	sub := newTestConn("transient.key", []string{"t"})
	sub.pid = 1
	h.RegisterAndIsFirst(sub)
	h.UnregisterAndIsLast(sub)
	if h.AcquireCleanupLock("transient.key") {
		t.Error("AcquireCleanupLock should reject after all subscribers have unregistered (count==0)")
	}
}
