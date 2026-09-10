// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

// lockErr builds the wire shape of DTS lock contention: the stable subcode plus
// the lock marker, both carried on the message.
func lockErr() error {
	return errs.NewAPIError(errs.SubtypeUnknown,
		"%s", dtsInitFailedSubcode+"：[EnsureDTSTask] lock already held, workspace: workspace_x, branch: dev").
		WithCode(500002776)
}

// terminalStateErr stands in for the other cause behind the same subcode. Its
// wording is constructed, not captured: this package has only ever observed the
// lock variant on the wire. That is precisely why the classification refuses to
// call this one retryable — the tests below assert the conservative treatment of
// a variant we cannot recognise, not the behaviour of a known message.
func terminalStateErr() error {
	return errs.NewAPIError(errs.SubtypeUnknown,
		"%s", dtsInitFailedSubcode+"：volc dts task in terminal status").WithCode(500002776)
}

// shortenBackoff keeps the real waiting path under test — the cancellation
// behaviour lives in it — while making the schedule too short to slow the suite.
func shortenBackoff(t *testing.T) {
	t.Helper()
	orig := dtsLockRetryBaseDelay
	dtsLockRetryBaseDelay = time.Millisecond
	t.Cleanup(func() { dtsLockRetryBaseDelay = orig })
}

func TestIsDTSLockContention(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"lock contention", lockErr(), true},
		{
			// Not resolved by waiting a second, so it must not consume the backoff.
			name: "same subcode without the lock marker",
			err:  terminalStateErr(),
			want: false,
		},
		{
			// Guards against matching on the human-readable phrase alone.
			name: "lock marker without the subcode",
			err:  errs.NewAPIError(errs.SubtypeUnknown, "some other lock already held").WithCode(500002776),
			want: false,
		},
		{"nil", nil, false},
		{"untyped", errors.New("lock already held k_dl_1600039"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDTSLockContention(tc.err); got != tc.want {
				t.Fatalf("isDTSLockContention = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDTSLockRetryDelayGrows pins the schedule shape directly, so the call-path
// tests do not have to observe timing to prove it.
func TestDTSLockRetryDelayGrows(t *testing.T) {
	// Jitter is additive on top of the doubling base, so the floor of each attempt
	// still exceeds the ceiling of the one before it.
	for attempt := 0; attempt < dtsLockRetryAttempts-1; attempt++ {
		base := dtsLockRetryBaseDelay * (1 << uint(attempt))
		maxThis := base + base/2
		minNext := dtsLockRetryBaseDelay * (1 << uint(attempt+1))
		if minNext <= maxThis {
			t.Fatalf("attempt %d can be >= attempt %d; lockstep callers would collide again", attempt, attempt+1)
		}
	}
	// And jitter must actually vary, or the separation it exists for never happens.
	seen := map[time.Duration]bool{}
	for i := 0; i < 50; i++ {
		seen[dtsLockRetryDelay(2)] = true
	}
	if len(seen) < 2 {
		t.Fatal("dtsLockRetryDelay is constant; collided callers would retry in lockstep")
	}
}

func TestCallWithDTSLockRetry_SucceedsAfterContention(t *testing.T) {
	shortenBackoff(t)
	calls := 0
	data, err := callWithDTSLockRetry(context.Background(), func() (map[string]interface{}, error) {
		calls++
		if calls < 3 {
			return nil, lockErr()
		}
		return map[string]interface{}{"ok": true}, nil
	})
	if err != nil {
		t.Fatalf("expected success once contention cleared, got %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (two contended, one succeeding)", calls)
	}
	if data["ok"] != true {
		t.Fatalf("payload from the successful attempt was lost: %v", data)
	}
}

func TestCallWithDTSLockRetry_BoundedWhenContentionPersists(t *testing.T) {
	shortenBackoff(t)
	calls := 0
	_, err := callWithDTSLockRetry(context.Background(), func() (map[string]interface{}, error) {
		calls++
		return nil, lockErr()
	})
	if !isDTSLockContention(err) {
		t.Fatalf("original error must survive exhaustion, got %v", err)
	}
	if calls != dtsLockRetryAttempts+1 {
		t.Fatalf("calls = %d, want %d (first attempt plus the bounded repeats)", calls, dtsLockRetryAttempts+1)
	}
}

// TestCallWithDTSLockRetry_DoesNotRetryOtherFailures is the guard that keeps this
// from becoming a general retry wrapper around a write: only lock contention is
// known to leave no partial state behind.
func TestCallWithDTSLockRetry_DoesNotRetryOtherFailures(t *testing.T) {
	shortenBackoff(t)
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"terminal-state sibling", terminalStateErr()},
		{"already enabled", errs.NewAPIError(errs.SubtypeUnknown, "k_dl_4000019：Audit is already enabled for table 'orders'").WithCode(400002476)},
		{"unrelated", errors.New("network down")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, err := callWithDTSLockRetry(context.Background(), func() (map[string]interface{}, error) {
				calls++
				return nil, tc.err
			})
			if calls != 1 {
				t.Fatalf("calls = %d, want 1 — only lock contention may be repeated", calls)
			}
			if !errors.Is(err, tc.err) && err != tc.err {
				t.Fatalf("error was not returned unchanged: %v", err)
			}
		})
	}
}

func TestCallWithDTSLockRetry_NoRetryOnSuccess(t *testing.T) {
	shortenBackoff(t)
	calls := 0
	if _, err := callWithDTSLockRetry(context.Background(), func() (map[string]interface{}, error) {
		calls++
		return map[string]interface{}{"ok": true}, nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, a first-try success must not repeat", calls)
	}
}

// TestCallWithDTSLockRetry_CancellationStopsBeforeNextWrite is the reason the wait
// is a select and not a sleep: audit set is a write, and a cancelled command must
// not send another one after the caller has walked away.
func TestCallWithDTSLockRetry_CancellationStopsBeforeNextWrite(t *testing.T) {
	// Long enough that the test would notice if cancellation were ignored.
	orig := dtsLockRetryBaseDelay
	dtsLockRetryBaseDelay = 2 * time.Second
	t.Cleanup(func() { dtsLockRetryBaseDelay = orig })

	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	start := time.Now()
	_, err := callWithDTSLockRetry(ctx, func() (map[string]interface{}, error) {
		calls++
		cancel() // caller gives up while the first contended attempt is in flight
		return nil, lockErr()
	})

	if calls != 1 {
		t.Fatalf("calls = %d, want 1 — cancellation must prevent the additional write", calls)
	}
	if elapsed := time.Since(start); elapsed >= dtsLockRetryBaseDelay {
		t.Fatalf("returned after %s; cancellation must not wait out the backoff", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cause chain lost: errors.Is(err, context.Canceled) = false, err = %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryNetwork {
		t.Fatalf("want a typed network error on cancellation, got %#v", err)
	}
}

// TestWithAppsHint_DTSLockContentionWording pins what the caller sees once the
// retries are spent: a server-side classification carrying the retryable flag, and
// wording that says this was a collision rather than a rejection.
func TestWithAppsHint_DTSLockContentionWording(t *testing.T) {
	cause := errors.New("upstream transport failure")
	in := lockErr().(*errs.APIError).WithCause(cause)
	out := withAppsHint(in, dbAuditSetHint)

	if !errors.Is(out, cause) {
		t.Errorf("cause chain lost: errors.Is(out, cause) = false")
	}
	p, _ := errs.ProblemOf(out)
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeServerError {
		t.Fatalf("category/subtype = %s/%s, want api/server_error", p.Category, p.Subtype)
	}
	if !p.Retryable {
		t.Fatal("Retryable = false; repeating this request unchanged is what resolves it")
	}
	if strings.Contains(p.Hint, "verify --app-id") {
		t.Fatalf("hint = %q, must not blame the request", p.Hint)
	}
	// The caller must be able to tell this was a collision, not a rejection, and
	// that the CLI already tried — otherwise "run it again" reads as superstition.
	if !strings.Contains(p.Hint, "concurrent") || !strings.Contains(p.Hint, "already retried") {
		t.Fatalf("hint = %q, must name the concurrency and say the CLI already retried", p.Hint)
	}
	if !strings.Contains(p.Message, "another request") {
		t.Fatalf("message = %q, want the contention stated in the caller's terms", p.Message)
	}
	// Internal wording must not reach users; log_id/troubleshooter keep the original.
	for _, internal := range []string{"EnsureDTSTask", "lock already held", "workspace_", "branch:"} {
		if strings.Contains(p.Message, internal) {
			t.Fatalf("message = %q leaks internal wording %q", p.Message, internal)
		}
	}
}

// TestWithAppsHint_DTSTerminalStateKeepsNeutralWording covers the other cause
// behind the same subcode: with no lock marker it must keep the table's neutral
// entry, not be described as a concurrency collision it is not.
func TestWithAppsHint_DTSTerminalStateKeepsNeutralWording(t *testing.T) {
	out := withAppsHint(terminalStateErr(), dbAuditSetHint)

	p, _ := errs.ProblemOf(out)
	if p.Subtype != errs.SubtypeServerError {
		t.Fatalf("subtype = %s, want server_error", p.Subtype)
	}
	// Not retryable: this package has never observed this variant's wire message,
	// and a task stuck in a terminal state is not fixed by calling again. Promising
	// otherwise would walk an agent through attempts that cannot work.
	if p.Retryable {
		t.Fatal("Retryable = true for a cause never observed to clear on its own")
	}
	if !strings.Contains(p.Hint, "needs attention") {
		t.Fatalf("hint = %q, must not promise that repeating alone resolves it", p.Hint)
	}
	if strings.Contains(p.Hint, "concurrent") || p.Message == dtsLockContentionMessage {
		t.Fatalf("non-contention cause was described as contention: msg=%q hint=%q", p.Message, p.Hint)
	}
	if p.Message != "volc dts task in terminal status" {
		t.Fatalf("message = %q, want the server wording kept for the unrecognised variant", p.Message)
	}
}
