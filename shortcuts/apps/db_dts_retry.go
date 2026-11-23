// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"math/rand"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
)

// Turning table audit on or off makes the server initialize the app's data-sync
// (DTS) task, and that initialization takes a workspace-wide lock. When a
// concurrent request already holds it, the server reports a failure instead of
// waiting — so two audit commands issued close together leave one caller with a
// generic "System error" for what is only contention.
//
// Retrying is safe here, which is not true of writes in general: the server rolls
// its own changelog write back before returning, so a contended call leaves audit
// off rather than half-applied, and a repeat either succeeds or reports "already
// enabled" (k_dl_4000019, classified). Verified against a live multi-env app —
// every contended table stayed disabled, and a serial repeat turned it on.
//
// Deliberately bounded and short. The server-side lock TTL is minutes, so a slow
// holder — one actually creating the remote task rather than adjusting it — cannot
// be waited out inside an interactive command; those exhaust the attempts and
// surface as a retryable server error. What this does cover is the common case
// where the holder finishes in well under a second.
//
// Retrying does not hide the defect from the service owners: the server logs a
// warning on every contention regardless of what the client does next.
const (
	dtsInitFailedSubcode = "k_dl_1600039"

	// dtsLockHeldMarker distinguishes lock contention from the other cause that
	// shares dtsInitFailedSubcode — a remote sync task in a terminal state, which
	// is not resolved by retrying in the next few seconds. Matching the subcode
	// alone would burn the whole backoff on failures that cannot recover from it.
	dtsLockHeldMarker = "lock already held"
)

// dtsLockContentionMessage / dtsLockContentionHint describe the contention in the
// caller's terms, replacing the server's "lock already held, workspace: …,
// branch: …" once it has been recognised.
//
// The hint states that the CLI already retried. Without that, the advice to run
// the command again reads as "you did something wrong, do it again" — and a caller
// who has just waited through the backoff would reasonably expect the CLI to have
// tried, so saying so is what makes a further manual attempt look sensible rather
// than superstitious.
const (
	dtsLockContentionMessage = "another request is already initializing this app's data-sync task"

	dtsLockContentionHint = "this is a collision with a concurrent request, not a problem with this command — " +
		"audit was left unchanged. The CLI already retried and the other request was still running. " +
		"Run the same command again in a few seconds, and avoid issuing audit changes for one app in parallel."
)

// Package-level so tests can shorten the schedule; production never reassigns them.
var (
	dtsLockRetryAttempts  = 3
	dtsLockRetryBaseDelay = 400 * time.Millisecond
)

// isDTSLockContention reports whether err is the transient "another request holds
// the DTS init lock" failure. Requires both the stable subcode and the lock marker;
// see dtsLockHeldMarker for why the subcode alone is not enough.
func isDTSLockContention(err error) bool {
	if err == nil {
		return false
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		return false
	}
	return strings.Contains(p.Message, dtsInitFailedSubcode) &&
		strings.Contains(p.Message, dtsLockHeldMarker)
}

// dtsLockRetryDelay is exponential with jitter. The jitter matters more than the
// growth: callers that collided once are in lockstep, and retrying on identical
// schedules would just have them collide again.
func dtsLockRetryDelay(attempt int) time.Duration {
	base := dtsLockRetryBaseDelay * (1 << uint(attempt))
	return base + time.Duration(rand.Int63n(int64(base/2)+1))
}

// callWithDTSLockRetry runs call, repeating it while it fails with DTS lock
// contention, up to dtsLockRetryAttempts extra times.
//
// Anything else — success, or any other failure — returns immediately: this must
// not become a general-purpose retry around a write.
//
// The wait honours ctx, matching pollUntil. Sleeping unconditionally would let a
// cancelled command issue another audit write after the caller had already walked
// away — the one thing that turns a harmless retry into a surprising one.
func callWithDTSLockRetry(ctx context.Context, call func() (map[string]interface{}, error)) (map[string]interface{}, error) {
	data, err := call()
	for attempt := 0; attempt < dtsLockRetryAttempts && isDTSLockContention(err); attempt++ {
		select {
		case <-ctx.Done():
			return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
				"cancelled while waiting to retry after a concurrent request").WithCause(ctx.Err())
		case <-time.After(dtsLockRetryDelay(attempt)):
		}
		data, err = call()
	}
	return data, err
}
