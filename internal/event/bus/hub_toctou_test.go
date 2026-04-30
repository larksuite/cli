// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package bus

import (
	"testing"
	"time"
)

// While a subscriber holds the cleanup lock for its key, Register for same key must block until release.
func TestConcurrentPreShutdownAndHelloRaceFree(t *testing.T) {
	h := NewHub()
	subA := newTestConn("mail.key", []string{"mail.receive"})
	subA.pid = 1001
	h.RegisterAndIsFirst(subA)

	if !h.AcquireCleanupLock("mail.key") {
		t.Fatal("A should acquire cleanup lock — it's the only subscriber")
	}

	subB := newTestConn("mail.key", []string{"mail.receive"})
	subB.pid = 1002

	registered := make(chan bool, 1)
	go func() {
		isFirst := h.RegisterAndIsFirst(subB)
		registered <- isFirst
	}()

	select {
	case <-registered:
		t.Fatal("B registered DURING A's cleanup — TOCTOU race not fixed")
	case <-time.After(200 * time.Millisecond):
	}

	h.ReleaseCleanupLock("mail.key")

	select {
	case isFirst := <-registered:
		_ = isFirst
	case <-time.After(500 * time.Millisecond):
		t.Fatal("B never registered after cleanup released")
	}
}

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

func TestReleaseCleanupLockIsIdempotent(t *testing.T) {
	h := NewHub()
	h.ReleaseCleanupLock("never.locked.key")
	h.ReleaseCleanupLock("never.locked.key")
}

func TestAcquireCleanupLockRejectsIfZeroSubscribers(t *testing.T) {
	h := NewHub()

	if h.AcquireCleanupLock("never.registered.key") {
		t.Error("AcquireCleanupLock should reject for a never-registered key (count==0)")
	}

	sub := newTestConn("transient.key", []string{"t"})
	sub.pid = 1
	h.RegisterAndIsFirst(sub)
	h.UnregisterAndIsLast(sub)
	if h.AcquireCleanupLock("transient.key") {
		t.Error("AcquireCleanupLock should reject after all subscribers have unregistered (count==0)")
	}
}
