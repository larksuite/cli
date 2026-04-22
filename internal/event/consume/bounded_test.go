// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package consume

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

// TestBoundedLoop_MaxEvents — when MaxEvents is set, the loop cancels the
// context after that many successful emits, regardless of how many more
// events are queued.
func TestBoundedLoop_MaxEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var emitted atomic.Int64
	opts := Options{MaxEvents: 3, ErrOut: io.Discard}

	// Simulate 5 successful emits.
	for i := 0; i < 5; i++ {
		emitted.Add(1)
		stopNow := checkMaxEvents(opts, &emitted)
		if (i + 1) >= 3 {
			if !stopNow {
				t.Fatalf("checkMaxEvents should return true at emit %d (max=3)", i+1)
			}
		} else {
			if stopNow {
				t.Fatalf("checkMaxEvents should not return true at emit %d (max=3)", i+1)
			}
		}
	}
	_ = ctx
}

// TestBoundedLoop_NoLimitWhenZero — MaxEvents=0 means unlimited.
func TestBoundedLoop_NoLimitWhenZero(t *testing.T) {
	var emitted atomic.Int64
	opts := Options{MaxEvents: 0, ErrOut: io.Discard}
	for i := 0; i < 100; i++ {
		emitted.Add(1)
		if checkMaxEvents(opts, &emitted) {
			t.Fatalf("checkMaxEvents should never return true when MaxEvents=0; returned true at emit %d", i+1)
		}
	}
}

// TestExitReason_Limit — emitted >= MaxEvents → reason="limit".
func TestExitReason_Limit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // mimic loop cancelling itself after hitting limit

	opts := Options{MaxEvents: 5, Timeout: 0}
	reason := exitReason(ctx, 5, opts)
	if reason != "limit" {
		t.Errorf("reason = %q, want \"limit\"", reason)
	}
}

// TestExitReason_Timeout — ctx.Err() == DeadlineExceeded → reason="timeout".
func TestExitReason_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)

	opts := Options{MaxEvents: 5, Timeout: 1 * time.Millisecond}
	reason := exitReason(ctx, 0, opts)
	if reason != "timeout" {
		t.Errorf("reason = %q, want \"timeout\"", reason)
	}
}

// TestExitReason_Signal — ctx cancelled, no timeout deadline → reason="signal".
func TestExitReason_Signal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{MaxEvents: 0, Timeout: 0}
	reason := exitReason(ctx, 0, opts)
	if reason != "signal" {
		t.Errorf("reason = %q, want \"signal\"", reason)
	}
}
