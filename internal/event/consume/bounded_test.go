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

func TestBoundedLoop_MaxEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var emitted atomic.Int64
	opts := Options{MaxEvents: 3, ErrOut: io.Discard}

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

func TestExitReason_Limit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{MaxEvents: 5, Timeout: 0}
	reason := exitReason(ctx, 5, opts)
	if reason != "limit" {
		t.Errorf("reason = %q, want \"limit\"", reason)
	}
}

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

func TestExitReason_Signal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := Options{MaxEvents: 0, Timeout: 0}
	reason := exitReason(ctx, 0, opts)
	if reason != "signal" {
		t.Errorf("reason = %q, want \"signal\"", reason)
	}
}
