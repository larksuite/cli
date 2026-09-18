// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// calendar recurring-event support: apply-to scope helpers, exception scanning,
// bounded-concurrency worker, and rate-limit retry used by +update and +delete
// when acting on recurring series or exceptions.

package calendar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// applyTo* enumerate --apply-to values shared by +update and +delete. The
// values are surfaced to cobra via Flag.Enum so the framework rejects unknown
// values before any code in this package runs.
const (
	flagApplyTo             = "apply-to"
	applyToSingle           = "single"
	applyToAll              = "all"
	applyToThisAndFollowing = "this-and-following"
)

// applyToValues is the ordered public enum surfaced in flag registration and
// `--help`. Passed to Flag.Enum so the shared validateEnumFlags step returns
// a typed ValidationError for unknown values before Validate/Execute runs.
var applyToValues = []string{applyToSingle, applyToAll, applyToThisAndFollowing}

// recurringKind is what classifyRecurringEvent returns after a GET on the
// target event: normal (non-recurring), master, materialized exception, or a
// plain (mirrored) recurring instance whose id carries originalTime > 0.
type recurringKind int

const (
	recurringKindNormal recurringKind = iota
	recurringKindMaster
	recurringKindException
	recurringKindPlainInstance
)

// exceptionMaxCount caps how many exceptions we hold in memory at once, so a
// pathological series (thousands of exceptions) cannot exhaust RAM before we
// fail fast with actionable guidance.
const exceptionMaxCount = 5000

// exceptionInstance is a minimal projection of an /instances item: the caller
// only needs the exception's event_id and its instance start time (unix
// seconds), which together let us both operate on the exception and decide
// whether it sits on/after the "this" pivot for this-and-following.
type exceptionInstance struct {
	EventID   string
	StartUnix int64
}

// recurringWindow describes the scan range fed to the /instances API. The
// caller derives it from the master event's start_time and rrule (UNTIL /
// COUNT) with a 5-year cap when the rrule is open-ended.
type recurringWindow struct {
	Start int64
	End   int64
}

// parseInstanceOriginalTime extracts the numeric suffix from an event_id
// shaped like `{uid}_{originalTime}`. Returns (0, false) if the id is not a
// recurring instance id, or if the suffix is not a positive integer. A `_0`
// suffix returns (0, false) because zero is not a real occurrence position —
// the master and every normal (non-recurring) event carry `_0`.
func parseInstanceOriginalTime(eventID string) (int64, bool) {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 || idx == len(eventID)-1 {
		return 0, false
	}
	n, err := strconv.ParseInt(eventID[idx+1:], 10, 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// hasStandardEventIDShape returns true when the id matches the standard Lark
// event shape `{uid}_{digits}`. Every real calendar event id has this shape:
// the master and normal (non-recurring) events use `_0`; instances / exceptions
// use `_{originalTime > 0}`. Any id fitting the shape might therefore be
// recurring, so callers must GET the event to know for sure — we cannot
// classify from the id alone.
func hasStandardEventIDShape(eventID string) bool {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 || idx == len(eventID)-1 {
		return false
	}
	_, err := strconv.ParseInt(eventID[idx+1:], 10, 64)
	return err == nil
}

// errMalformedEventID returns the typed validation error surfaced when a
// caller passes an event_id that does not match the standard Lark shape
// `{uid}_{digits}`. Rejecting up front (instead of silently degrading to a
// single-event path) prevents guessing recurring vs. non-recurring from
// garbage input and gives the caller an actionable message.
func errMalformedEventID(eventID string) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"invalid --event-id %q: every Lark calendar event id has the shape `{uid}_{originalTime}` (`_0` for masters/normal events, `_{unix seconds}` for instances/exceptions)",
		eventID).WithParam("--event-id")
}

// classifyRecurringEvent looks at the GET /events/:event_id response and
// decides whether we are dealing with the master, an exception, a plain
// recurring instance whose id carries an originalTime, or a normal
// (non-recurring) event.
func classifyRecurringEvent(event *calendarEvent) recurringKind {
	if event == nil {
		return recurringKindNormal
	}
	if event.IsException {
		return recurringKindException
	}
	if event.Recurrence != "" {
		return recurringKindMaster
	}
	if event.EventID != "" {
		if ot, ok := parseInstanceOriginalTime(event.EventID); ok && ot > 0 {
			return recurringKindPlainInstance
		}
	}
	return recurringKindNormal
}

// fetchCalendarEvent reads the calendar event by id. It only issues the GET
// and parses the response envelope — callers apply classifyRecurringEvent
// afterward to decide whether the event is a master, exception, plain
// instance, or normal one-shot. Named without a "recurring" qualifier
// because the fetch itself is oblivious to the recurring/non-recurring split.
func fetchCalendarEvent(runtime *common.RuntimeContext, calendarID, eventID string) (*calendarEvent, error) {
	data, err := runtime.CallAPITyped("GET",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
			validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID)),
		nil, nil)
	if err != nil {
		return nil, err
	}
	return parseCalendarEvent(data)
}

// validateApplyTo enforces the shared contract:
//   - single is legal on any non-master event
//   - all / this-and-following are only meaningful on recurring events
//   - master ids reject `single` and `this-and-following` because they lack a
//     concrete instance position; caller must pass an instance id instead
//   - exception ids reject `this-and-following` (an exception cannot be the
//     pivot; caller must pass the underlying instance id)
//
// Every rejection message states the concrete reason and lists the scopes
// legal on the current event kind, so the caller can self-correct in one read.
//
// The framework's Enum check has already rejected unknown values before this
// function runs, so we only reason about the four legal states here.
func validateApplyTo(runtime *common.RuntimeContext, kind recurringKind, eventID string) (string, error) {
	scope := strings.TrimSpace(runtime.Str(flagApplyTo))

	switch kind {
	case recurringKindNormal:
		if scope != "" && scope != applyToSingle {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s=%s` is not valid on a normal (non-recurring) event; only `--%s=%s` is accepted (or leave `--%s` unset)",
				flagApplyTo, scope, flagApplyTo, applyToSingle, flagApplyTo).
				WithParam("--" + flagApplyTo)
		}
		return applyToSingle, nil

	case recurringKindException:
		if scope == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s` is required on a recurring exception; valid scopes are `%s` (this exception only) or `%s` (the whole series and every exception)",
				flagApplyTo, applyToSingle, applyToAll).WithParam("--" + flagApplyTo)
		}
		if scope == applyToThisAndFollowing {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s=%s` is not valid on a recurring exception; valid scopes here are `%s` | `%s`. To truncate the series from a specific occurrence, pass the `event_id` of a non-exception instance (`{uid}_{originalTime}`, originalTime > 0) — look it up via `+agenda` or `+search-event`",
				flagApplyTo, applyToThisAndFollowing, applyToSingle, applyToAll).
				WithParam("--" + flagApplyTo)
		}
		return scope, nil

	case recurringKindPlainInstance:
		if scope == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s` is required on a recurring instance; valid scopes are `%s` | `%s` | `%s`",
				flagApplyTo, applyToSingle, applyToAll, applyToThisAndFollowing).WithParam("--" + flagApplyTo)
		}
		return scope, nil

	case recurringKindMaster:
		if scope == "" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s` is required on the recurring master; only `--%s=%s` is valid on the master id (the whole series and every exception). To operate on one occurrence, pass the instance `event_id` (`{uid}_{originalTime}`, originalTime > 0) — look it up via `+agenda` or `+search-event`",
				flagApplyTo, flagApplyTo, applyToAll).WithParam("--" + flagApplyTo)
		}
		if scope == applyToSingle {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s=%s` is not valid on the recurring master; only `--%s=%s` is accepted here. To operate on one occurrence, pass the instance `event_id` (`{uid}_{originalTime}`, originalTime > 0) — look it up via `+agenda` or `+search-event`",
				flagApplyTo, applyToSingle, flagApplyTo, applyToAll).
				WithParam("--" + flagApplyTo)
		}
		if scope == applyToThisAndFollowing {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
				"`--%s=%s` is not valid on the recurring master; only `--%s=%s` is accepted here. To truncate the series from a specific occurrence, pass that instance's `event_id`",
				flagApplyTo, applyToThisAndFollowing, flagApplyTo, applyToAll).
				WithParam("--" + flagApplyTo)
		}
		return scope, nil
	}
	return "", errs.NewInternalError(errs.SubtypeUnknown, "unknown recurring kind %d", kind)
}

// masterEventID rewrites an instance / exception event_id into its master id
// by stripping the `_{originalTime}` suffix and re-appending `_0`. Callers use
// this when the user passed an exception id under --apply-to=all so the
// downstream truncation / PATCH goes to the master.
func masterEventID(eventID string) string {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 {
		return eventID
	}
	return eventID[:idx] + "_0"
}

// eventLocation returns the IANA timezone declared on the event (start_time
// preferred, end_time as fallback), or time.UTC when the field is missing or
// unrecognised. Used for all-day boundary math and pivot-day UNTIL cutoff.
func eventLocation(event *calendarEvent) *time.Location {
	if event == nil {
		return time.Local
	}
	tz := ""
	if event.StartTime != nil {
		tz = event.StartTime.Timezone
	}
	if tz == "" && event.EndTime != nil {
		tz = event.EndTime.Timezone
	}
	if tz == "" {
		return time.Local
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Local
	}
	return loc
}

// isAllDayEvent reports whether the event is stored as an all-day event: the
// API returns `date` (YYYY-MM-DD) without `timestamp` for both endpoints.
func isAllDayEvent(event *calendarEvent) bool {
	if event == nil || event.StartTime == nil {
		return false
	}
	return event.StartTime.Date != "" && event.StartTime.Timestamp == ""
}

// masterStartUnix returns the unix second at 00:00 of the master's start day,
// interpreted in the event's own timezone. Used as the exception-scan window's
// lower bound: pinning to day-midnight (rather than the raw start timestamp)
// keeps the window aligned with how downstream logic — pivotDayMidnight for
// this-and-following truncation, and /instances originalTime probes — thinks
// about a "day" in the event's timezone, and can only widen the scan (an
// exception can never sit before the master's day anyway).
//
// Timed events: shift the stored timestamp back to that day's midnight in the
// event's timezone. All-day events: parse `date` in the event's timezone
// (Lark's `date` is a wall-clock day, not a UTC instant).
func masterStartUnix(master *calendarEvent) (int64, error) {
	if master == nil || master.StartTime == nil {
		return 0, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"missing start_time on the recurring master event")
	}
	loc := eventLocation(master)
	if ts := master.StartTime.Timestamp; ts != "" {
		n, err := strconv.ParseInt(ts, 10, 64)
		if err != nil || n <= 0 {
			return 0, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"invalid start_time.timestamp %q on the recurring master event", ts)
		}
		t := time.Unix(n, 0).In(loc)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix(), nil
	}
	if date := master.StartTime.Date; date != "" {
		t, err := time.ParseInLocation("2006-01-02", date, time.UTC)
		if err != nil {
			return 0, errs.NewInternalError(errs.SubtypeInvalidResponse,
				"invalid all-day start_time.date %q on the recurring master event: %v", date, err)
		}
		return t.Unix(), nil
	}
	return 0, errs.NewInternalError(errs.SubtypeInvalidResponse,
		"start_time on the recurring master event has neither timestamp nor date")
}

// recurringWindowFromMaster derives the exception scan window. Start is the
// master's start-day midnight in the event's timezone (see masterStartUnix);
// End is the master's UNTIL if the rrule has one, an approximation from
// COUNT+INTERVAL/FREQ,
// or start + 5 years.
func recurringWindowFromMaster(master *calendarEvent, now time.Time) (recurringWindow, error) {
	startSec, err := masterStartUnix(master)
	if err != nil {
		return recurringWindow{}, err
	}
	end := recurringEndFromRRule(master.Recurrence, startSec, now)
	if end < startSec {
		end = startSec
	}
	return recurringWindow{Start: startSec, End: end}, nil
}

const recurringDefaultHorizon = 5 * 365 * 24 * time.Hour

// recurringEndFromRRule parses the rrule and returns a unix-seconds upper
// bound for the last instance's end time. It errs on the side of being
// generous (adds a small buffer) so an instance whose end sits exactly on the
// boundary is still returned by /instances.
func recurringEndFromRRule(rrule string, startSec int64, now time.Time) int64 {
	upper := now.Add(recurringDefaultHorizon).Unix()
	if rrule == "" {
		return upper
	}
	// The API prefixes the rule with RRULE:; strip it before parsing.
	rule := strings.TrimPrefix(strings.TrimSpace(rrule), "RRULE:")
	parts := strings.Split(rule, ";")
	kv := map[string]string{}
	for _, p := range parts {
		if eq := strings.IndexByte(p, '='); eq > 0 {
			kv[strings.ToUpper(strings.TrimSpace(p[:eq]))] = strings.TrimSpace(p[eq+1:])
		}
	}
	if until, ok := kv["UNTIL"]; ok {
		if t, ok := parseRRuleUntil(until); ok {
			// Add a day of slack so the query window includes the last instance
			// regardless of the event duration.
			return t.Add(24 * time.Hour).Unix()
		}
	}
	if countStr, ok := kv["COUNT"]; ok {
		if n, err := strconv.Atoi(countStr); err == nil && n > 0 {
			interval := 1
			if s, ok := kv["INTERVAL"]; ok {
				if v, err := strconv.Atoi(s); err == nil && v > 0 {
					interval = v
				}
			}
			step := recurringFreqDuration(kv["FREQ"])
			if step > 0 {
				horizon := time.Unix(startSec, 0).Add(step * time.Duration(interval) * time.Duration(n+1))
				h := horizon.Unix()
				if h < upper {
					return h
				}
			}
		}
	}
	return upper
}

// parseRRuleUntil accepts the two RFC5545 UNTIL grammars we see in Lark
// events: `20260901T090000Z` (UTC) and `20260901` (date-only). Everything
// else falls back to (_, false).
func parseRRuleUntil(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	for _, layout := range []string{"20060102T150405Z", "20060102"} {
		if t, err := time.ParseInLocation(layout, v, time.UTC); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func recurringFreqDuration(freq string) time.Duration {
	switch strings.ToUpper(strings.TrimSpace(freq)) {
	case "DAILY":
		return 24 * time.Hour
	case "WEEKLY":
		return 7 * 24 * time.Hour
	case "MONTHLY":
		return 31 * 24 * time.Hour
	case "YEARLY":
		return 366 * 24 * time.Hour
	}
	return 0
}

// listExceptionsInWindow scans /events/:event_id/instances across the given
// window in ≤ 1-year chunks (well below the API's 2-year cap) and returns
// exceptions. When includeCancelled is false (default use: apply-to=all
// non-time PATCH), cancelled exceptions are skipped — patching a cancelled row
// would only revive noise. When includeCancelled is true (apply-to=all /
// this-and-following exception cleanup, which reissue DELETE with
// delete_exception=true), cancelled rows are also returned so the cleanup pass
// can destroy the placeholder (mark it is_deleted=true) alongside the live
// exceptions — otherwise the cancelled row would keep participating in
// downstream series expansion.
//
// Note both states are soft deletions at the DB level: `cancelled` is a still-
// valid business status (occurrence-subtracted placeholder), while
// `is_deleted=true` means the exception has been destroyed and no longer
// participates. The scan de-duplicates by event_id — page or chunk overlap can
// otherwise surface the same exception twice — and enforces exceptionMaxCount
// to protect against memory blowup.
func listExceptionsInWindow(ctx context.Context, runtime *common.RuntimeContext, calendarID, masterID string, w recurringWindow, includeCancelled bool) ([]exceptionInstance, error) {
	if w.End <= w.Start {
		return nil, nil
	}
	const oneYearSec int64 = 365 * 24 * 60 * 60
	seen := make(map[string]struct{})
	var out []exceptionInstance
	chunkStart := w.Start
	for chunkStart < w.End {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunkEnd := chunkStart + oneYearSec
		if chunkEnd > w.End {
			chunkEnd = w.End
		}
		pageToken := ""
		for {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			params := map[string]interface{}{
				"start_time": strconv.FormatInt(chunkStart, 10),
				"end_time":   strconv.FormatInt(chunkEnd, 10),
				"page_size":  500,
			}
			if pageToken != "" {
				params["page_token"] = pageToken
			}
			path := fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/instances",
				validate.EncodePathSegment(calendarID), validate.EncodePathSegment(masterID))
			data, err := runtime.CallAPITyped("GET", path, params, nil)
			if err != nil {
				return nil, err
			}
			items, _ := data["items"].([]interface{})
			for _, raw := range items {
				item, _ := raw.(map[string]interface{})
				if item == nil {
					continue
				}
				isException, _ := item["is_exception"].(bool)
				if !isException {
					continue
				}
				status, _ := item["status"].(string)
				if status == "cancelled" && !includeCancelled {
					continue
				}
				eventID, _ := item["event_id"].(string)
				if eventID == "" {
					continue
				}
				if _, dup := seen[eventID]; dup {
					continue
				}
				seen[eventID] = struct{}{}
				var startSec int64
				if st, ok := item["start_time"].(map[string]interface{}); ok {
					if ts, _ := st["timestamp"].(string); ts != "" {
						startSec, _ = strconv.ParseInt(ts, 10, 64)
					}
				}
				if startSec == 0 {
					if ot, ok := parseInstanceOriginalTime(eventID); ok {
						startSec = ot
					}
				}
				out = append(out, exceptionInstance{EventID: eventID, StartUnix: startSec})
				if len(out) > exceptionMaxCount {
					return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
						"this recurring series has more than %d exceptions; refuse to load them all into memory. Narrow --apply-to or clean up exceptions incrementally", exceptionMaxCount).
						WithParam("--" + flagApplyTo)
				}
			}
			pageToken, _ = data["page_token"].(string)
			hasMore, _ := data["has_more"].(bool)
			if !hasMore || pageToken == "" {
				break
			}
		}
		chunkStart = chunkEnd + 1
	}
	return out, nil
}

// splitFutureThenPast partitions exceptions into a future-first pair
// (relative to nowSec) and sorts each half for predictable processing:
// future ascending (near-term first), past descending (recent past first).
func splitFutureThenPast(items []exceptionInstance, nowSec int64) (future, past []exceptionInstance) {
	future = make([]exceptionInstance, 0, len(items))
	past = make([]exceptionInstance, 0, len(items))
	for _, it := range items {
		if it.StartUnix >= nowSec {
			future = append(future, it)
		} else {
			past = append(past, it)
		}
	}
	sort.SliceStable(future, func(i, j int) bool { return future[i].StartUnix < future[j].StartUnix })
	sort.SliceStable(past, func(i, j int) bool { return past[i].StartUnix > past[j].StartUnix })
	return future, past
}

// filterOnOrAfter returns items whose StartUnix is >= pivot, sorted ascending
// so callers process the earliest post-pivot exception first.
func filterOnOrAfter(items []exceptionInstance, pivot int64) []exceptionInstance {
	out := make([]exceptionInstance, 0, len(items))
	for _, it := range items {
		if it.StartUnix >= pivot {
			out = append(out, it)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartUnix < out[j].StartUnix })
	return out
}

// exceptionOperationFailure is the per-item error record collected by
// exceptionWorker. Callers surface it in the JSON result so a partial failure
// is machine-readable and the user can retry the specific ids.
type exceptionOperationFailure struct {
	EventID string `json:"event_id"`
	Error   string `json:"error"`
}

// exceptionWorker orchestrates concurrent per-exception work with a 3-way
// worker pool, rate-limit retry, and a 2s progress ticker. It does NOT
// short-circuit on failure: a failing exception is recorded via Failures()
// and the rest of the batch keeps running, matching the requirement that
// per-item failures should not block later exceptions from being processed.
type exceptionWorker struct {
	Concurrency int
	Runtime     *common.RuntimeContext
	Label       string // e.g. "[calendar +delete]"
	Total       int
	Do          func(ctx context.Context, ex exceptionInstance) error

	processed atomic.Int64
	failed    atomic.Int64

	mu       sync.Mutex
	failures []exceptionOperationFailure
}

// exceptionRetryBudget is the max retries per exception when the API returns
// a rate-limit / concurrency code.
const (
	exceptionRetryBudget      = 5
	exceptionRetryInitialWait = 500 * time.Millisecond
	exceptionRetryMaxWait     = 10 * time.Second
)

// isRateLimitedAPIError returns true when the API classified the failure as
// rate-limit (99991400) or when the calendar-side codes 190004 / 190005 /
// 190010 surface unclassified through the raw APIError.Code.
func isRateLimitedAPIError(err error) bool {
	if err == nil {
		return false
	}
	if errs.IsRetryable(err) {
		if p, ok := errs.ProblemOf(err); ok && p.Subtype == errs.SubtypeRateLimit {
			return true
		}
	}
	var ae *errs.APIError
	if errors.As(err, &ae) {
		switch ae.Code {
		case 190004, 190005, 190010:
			return true
		}
	}
	return false
}

// backoffWait returns exponential + jitter (bounded by exceptionRetryMaxWait)
// for the given attempt (0-indexed).
func backoffWait(attempt int) time.Duration {
	d := exceptionRetryInitialWait << attempt
	if d > exceptionRetryMaxWait {
		d = exceptionRetryMaxWait
	}
	// Add up to 20% jitter so N workers waking together do not all retry in
	// lock-step and trip the same limiter window again.
	jitter := time.Duration(rand.Int63n(int64(d) / 5)) //nolint:gosec // jitter randomness only
	return d + jitter
}

// Run consumes the given exception slice. It always processes every item —
// per-item failures are recorded in w.failures and reported afterward by the
// caller via Failures(). The only ways Run returns a non-nil error are
// context cancellation or a developer error (Do not set).
func (w *exceptionWorker) Run(ctx context.Context, items []exceptionInstance) error {
	if w.Concurrency <= 0 {
		w.Concurrency = 3
	}
	if w.Do == nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "exceptionWorker.Do not set")
	}
	if len(items) == 0 {
		return nil
	}

	tickerStop := w.startProgressTicker(ctx)
	defer tickerStop()

	sem := make(chan struct{}, w.Concurrency)
	var wg sync.WaitGroup

	for _, ex := range items {
		if err := ctx.Err(); err != nil {
			wg.Wait()
			return err
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		}
		wg.Add(1)
		go func(ex exceptionInstance) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := w.runOne(ctx, ex); err != nil {
				w.failed.Add(1)
				w.recordFailure(ex.EventID, err)
				return
			}
			w.processed.Add(1)
		}(ex)
	}
	wg.Wait()
	return nil
}

// recordFailure stores the (id, message) pair so callers can surface the
// per-item outcomes after the whole batch finishes.
func (w *exceptionWorker) recordFailure(id string, err error) {
	msg := err.Error()
	if m := unwrapCalendarAPIError(err); m != "" {
		msg = m
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.failures = append(w.failures, exceptionOperationFailure{EventID: id, Error: msg})
}

// Failures returns a copy of per-item failures collected during Run so the
// caller can serialise them into the JSON result without holding the mutex.
func (w *exceptionWorker) Failures() []exceptionOperationFailure {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]exceptionOperationFailure, len(w.failures))
	copy(out, w.failures)
	return out
}

// runOne calls Do with rate-limit retry. Any non-rate-limit failure is
// returned immediately; a rate-limit failure sleeps with exponential
// backoff (respecting Retry-After when the API provided one) and retries up
// to exceptionRetryBudget times.
func (w *exceptionWorker) runOne(ctx context.Context, ex exceptionInstance) error {
	var lastErr error
	for attempt := 0; attempt <= exceptionRetryBudget; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := w.Do(ctx, ex)
		if err == nil {
			return nil
		}
		if !isRateLimitedAPIError(err) {
			return err
		}
		lastErr = err
		if attempt == exceptionRetryBudget {
			break
		}
		wait := backoffWait(attempt)
		if server, ok := errs.RetryAfter(err); ok && server > wait {
			wait = server
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
	return lastErr
}

// startProgressTicker prints one status line to stderr every 2 seconds until
// the returned stop function is called (typically from `defer`). It never
// panics or blocks the worker.
func (w *exceptionWorker) startProgressTicker(ctx context.Context) func() {
	if w.Runtime == nil || w.Runtime.IO() == nil {
		return func() {}
	}
	out := w.Runtime.IO().ErrOut
	if out == nil {
		return func() {}
	}
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan struct{})
	var once sync.Once
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				w.writeProgress(out)
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (w *exceptionWorker) writeProgress(out io.Writer) {
	processed := w.processed.Load()
	failed := w.failed.Load()
	fmt.Fprintf(out, "%s progress: %d/%d exceptions processed (%d failed)\n",
		w.Label, processed+failed, w.Total, failed)
}

// finalSummary writes a one-line completion notice used by callers after Run
// returns. Kept separate so callers control whether to emit success or
// failure text.
func (w *exceptionWorker) finalSummary(out io.Writer, verb string) {
	processed := w.processed.Load()
	failed := w.failed.Load()
	fmt.Fprintf(out, "%s %s %d exception(s) (%d failed)\n",
		w.Label, verb, processed+failed, failed)
}

// deleteEventOnce issues a DELETE for one calendar event.
//
//   - notify controls need_notification (whether attendees are notified).
//   - deleteException controls the new delete_exception query flag. Both values
//     are soft deletions at the DB level; the difference is business state:
//     false (default): the API marks the exception as `cancelled`. That is a
//     still-valid business status — the row is kept as a data point so it can
//     be subtracted from the recurring series when re-computing occurrences
//     downstream.
//     true: the API marks the exception as destroyed (is_deleted=true). Used
//     by +delete / +update paths that intentionally clear exceptions ahead of
//     a series-wide operation, where a cancelled placeholder would only add
//     noise to a subsequent series-view.
//
// Callers deleting the user-facing target (a normal event, an exception the
// caller pointed at, or the master itself) must pass deleteException=false;
// only the exception-cleanup passes inside apply-to=all / this-and-following
// pass true.
//
// A 193003 ("event is deleted") response is treated as success: the target has
// already been destroyed on the server, which is the outcome the caller asked
// for. This makes the batch idempotent under concurrent or repeated runs.
func deleteEventOnce(runtime *common.RuntimeContext, calendarID, eventID string, notify, deleteException bool) error {
	params := map[string]interface{}{"need_notification": notify}
	if deleteException {
		params["delete_exception"] = true
	}
	_, err := runtime.CallAPITyped("DELETE",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
			validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID)),
		params, nil)
	if isEventAlreadyDeleted(err) {
		return nil
	}
	return err
}

// isEventAlreadyDeleted reports whether err is a calendar 193003 ("event is
// deleted") API error. The server has already destroyed the target, which is
// exactly what the caller asked for — the exception-cleanup and master-delete
// paths treat it as success so a racing / repeated cleanup does not fail the
// batch.
func isEventAlreadyDeleted(err error) bool {
	if err == nil {
		return false
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == 193003
}

// truncateRecurrenceUntil rewrites a Lark RRULE's UNTIL to the given time
// (formatted UTC, RFC5545 "YYYYMMDDThhmmssZ"). It appends UNTIL if none
// exists, or replaces the existing UNTIL / COUNT clauses (they are mutually
// exclusive in RFC5545).
func truncateRecurrenceUntil(rrule string, until time.Time) string {
	body := strings.TrimSpace(rrule)
	prefix := ""
	if strings.HasPrefix(body, "RRULE:") {
		prefix = "RRULE:"
		body = strings.TrimPrefix(body, "RRULE:")
	}
	newUntil := "UNTIL=" + until.UTC().Format("20060102T150405Z")
	var kept []string
	replaced := false
	for _, part := range strings.Split(body, ";") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		up := strings.ToUpper(p)
		if strings.HasPrefix(up, "UNTIL=") {
			if !replaced {
				kept = append(kept, newUntil)
				replaced = true
			}
			continue
		}
		if strings.HasPrefix(up, "COUNT=") {
			// UNTIL and COUNT are mutually exclusive per RFC5545; drop COUNT.
			continue
		}
		kept = append(kept, p)
	}
	if !replaced {
		kept = append(kept, newUntil)
	}
	return prefix + strings.Join(kept, ";")
}

// pivotDayMidnight returns midnight (00:00:00) of the calendar day that
// pivotUnix lands on, interpreted in the event's timezone. Used both as the
// UNTIL cutoff for a this-and-following truncation ("stop the series just
// before the pivot day starts") and as the exception-scan lower bound.
func pivotDayMidnight(pivotUnix int64, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	t := time.Unix(pivotUnix, 0).In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Add(-time.Second)
}

// formatPivotDatetime renders pivotUnix as an RFC3339 string in the event's
// timezone so log lines are human-readable ("2026-03-20T09:00:00+08:00")
// alongside the raw unix seconds.
func formatPivotDatetime(pivotUnix int64, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return time.Unix(pivotUnix, 0).In(loc).Format(time.RFC3339)
}
