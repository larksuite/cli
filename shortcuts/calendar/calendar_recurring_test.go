// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ---------------------------------------------------------------------------
// looksLikeRecurringEventID
// ---------------------------------------------------------------------------

func TestHasStandardEventIDShape(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"", false},
		{"evt_plain", false},
		{"evt_update1", false},
		{"uid_0", true},
		{"uid_1742515200", true},
		{"75d28f9b-e35c-4230-8a83-4a661497db54_0", true},
		{"75d28f9b-e35c-4230-8a83-4a661497db54_1602504000", true},
	}
	for _, tc := range cases {
		if got := hasStandardEventIDShape(tc.id); got != tc.want {
			t.Errorf("hasStandardEventIDShape(%q) = %v, want %v", tc.id, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseInstanceOriginalTime
// ---------------------------------------------------------------------------

func TestParseInstanceOriginalTime(t *testing.T) {
	if v, ok := parseInstanceOriginalTime("uid_0"); ok || v != 0 {
		t.Errorf("uid_0 should be (0,false), got (%d,%v)", v, ok)
	}
	if v, ok := parseInstanceOriginalTime("uid_1742515200"); !ok || v != 1742515200 {
		t.Errorf("uid_1742515200 => (%d,%v)", v, ok)
	}
	if _, ok := parseInstanceOriginalTime("noise"); ok {
		t.Error("noise should be false")
	}
}

// ---------------------------------------------------------------------------
// truncateRecurrenceUntil / inheritRRuleForFollowing
// ---------------------------------------------------------------------------

func TestTruncateRecurrenceUntil(t *testing.T) {
	pivot := time.Date(2026, 3, 20, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		in   string
		want string
	}{
		{"FREQ=WEEKLY;BYDAY=MO", "FREQ=WEEKLY;BYDAY=MO;UNTIL=20260320T080000Z"},
		{"FREQ=WEEKLY;COUNT=10", "FREQ=WEEKLY;UNTIL=20260320T080000Z"},
		{"FREQ=WEEKLY;UNTIL=20250101T000000Z", "FREQ=WEEKLY;UNTIL=20260320T080000Z"},
		{"RRULE:FREQ=DAILY;INTERVAL=2", "RRULE:FREQ=DAILY;INTERVAL=2;UNTIL=20260320T080000Z"},
	}
	for _, tc := range cases {
		got := truncateRecurrenceUntil(tc.in, pivot)
		if got != tc.want {
			t.Errorf("truncateRecurrenceUntil(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestInheritRRuleForFollowing(t *testing.T) {
	// UNTIL is retained (fixed wall-clock cutoff copies cleanly); COUNT is
	// dropped (occurrence count relative to old dtstart is meaningless for a
	// new pivot). Every other segment (FREQ, INTERVAL, BYDAY...) passes
	// through untouched.
	cases := map[string]string{
		"FREQ=WEEKLY":                        "FREQ=WEEKLY",
		"FREQ=WEEKLY;COUNT=10":               "FREQ=WEEKLY",
		"FREQ=WEEKLY;UNTIL=20260901T000000Z": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
		"RRULE:FREQ=WEEKLY;UNTIL=20260901T000000Z;COUNT=5;BYDAY=MO": "RRULE:FREQ=WEEKLY;UNTIL=20260901T000000Z;BYDAY=MO",
	}
	for in, want := range cases {
		if got := inheritRRuleForFollowing(in); got != want {
			t.Errorf("inheritRRuleForFollowing(%q) = %q, want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// recurringWindowFromMaster
// ---------------------------------------------------------------------------

func TestRecurringWindowFromMaster_OpenEndedFallsBackTo5Years(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	master := &calendarEvent{
		StartTime:  &calendarEventTime{Timestamp: "1500000000", Timezone: "UTC"},
		Recurrence: "FREQ=WEEKLY",
	}
	w, err := recurringWindowFromMaster(master, now)
	if err != nil {
		t.Fatal(err)
	}
	// 1500000000 = 2017-07-14T02:40:00Z; masterStartUnix rolls back to the
	// event-timezone day midnight (UTC 2017-07-14T00:00:00Z = 1499990400).
	if w.Start != 1499990400 {
		t.Errorf("start=%d, want 1499990400 (UTC midnight of 2017-07-14)", w.Start)
	}
	// end should be now + ~5 years
	fiveYearsLater := now.Add(5 * 365 * 24 * time.Hour).Unix()
	if w.End < fiveYearsLater-time.Hour.Milliseconds() || w.End > fiveYearsLater+time.Hour.Milliseconds() {
		// approximate check (5y default)
		if w.End < now.Add(4*365*24*time.Hour).Unix() {
			t.Errorf("end=%d unexpectedly low", w.End)
		}
	}
}

func TestRecurringWindowFromMaster_AllDayStart_ParsedInUTC(t *testing.T) {
	// All-day master: start_time carries date only. Lark's `date` is treated
	// as a UTC wall-clock day.
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	master := &calendarEvent{
		StartTime:  &calendarEventTime{Date: "2026-03-01", Timezone: "Asia/Shanghai"},
		Recurrence: "FREQ=DAILY;UNTIL=20260305T000000Z",
	}
	w, err := recurringWindowFromMaster(master, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).Unix()
	if w.Start != want {
		t.Errorf("start=%d, want %d (UTC midnight of 2026-03-01)", w.Start, want)
	}
}

func TestPivotDayMidnight_UsesEventTimezone(t *testing.T) {
	// A pivot at 2026-03-20T02:00:00Z is 10:00 Asia/Shanghai; the function
	// returns "one second before that day's Shanghai midnight" — the shape
	// truncateRecurrenceUntil consumes as UNTIL ("stop the series just before
	// the pivot day starts"), reused as the exception-scan lower bound.
	loc, _ := time.LoadLocation("Asia/Shanghai")
	pivot := time.Date(2026, 3, 20, 2, 0, 0, 0, time.UTC).Unix()
	got := pivotDayMidnight(pivot, loc)
	want := time.Date(2026, 3, 20, 0, 0, 0, 0, loc).Add(-time.Second)
	if !got.Equal(want) {
		t.Errorf("midnight=%s, want %s", got, want)
	}
}

func TestFormatPivotDatetime_UsesEventTimezone(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	pivot := time.Date(2026, 3, 20, 2, 0, 0, 0, time.UTC).Unix()
	got := formatPivotDatetime(pivot, loc)
	if got != "2026-03-20T10:00:00+08:00" {
		t.Errorf("got %q, want 2026-03-20T10:00:00+08:00", got)
	}
}

func TestListExceptionsInWindow_Dedupes(t *testing.T) {
	// Emulate two page hits returning the same exception id — the second copy
	// must be dropped.
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_100", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "100"},
					},
					map[string]interface{}{
						"event_id": "uid_100", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "100"},
					},
					map[string]interface{}{
						"event_id": "uid_200", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "200"},
					},
				},
			},
		},
	})
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, defaultConfig(), f, core.AsBot)
	warmTokenCache(t)
	got, err := listExceptionsInWindow(context.Background(), rt, "cal_r", "uid_0",
		recurringWindow{Start: 0, End: 1000}, false)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len=%d ids=%v; want 2 (duplicate uid_100 must be dropped)", len(got), got)
	}
}

func TestRecurringWindowFromMaster_UntilRespected(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	master := &calendarEvent{
		StartTime:  &calendarEventTime{Timestamp: "1500000000"},
		Recurrence: "FREQ=WEEKLY;UNTIL=20260401T000000Z",
	}
	w, err := recurringWindowFromMaster(master, now)
	if err != nil {
		t.Fatal(err)
	}
	// UNTIL 2026-04-01 + 1 day slack → 2026-04-02.
	want := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC).Unix()
	if w.End != want {
		t.Errorf("end=%d, want %d", w.End, want)
	}
}

// ---------------------------------------------------------------------------
// splitFutureThenPast / filterOnOrAfter
// ---------------------------------------------------------------------------

func TestSplitFutureThenPast_SortsFutureAscPastDesc(t *testing.T) {
	// Purposefully unsorted input: future should come back ascending,
	// past should come back descending.
	items := []exceptionInstance{
		{EventID: "c", StartUnix: 300},
		{EventID: "a", StartUnix: 100},
		{EventID: "d", StartUnix: 400},
		{EventID: "b", StartUnix: 200},
		{EventID: "z", StartUnix: 50},
	}
	f, p := splitFutureThenPast(items, 200)
	if len(f) != 3 || f[0].EventID != "b" || f[1].EventID != "c" || f[2].EventID != "d" {
		t.Errorf("future=%v; want [b c d] ascending", f)
	}
	if len(p) != 2 || p[0].EventID != "a" || p[1].EventID != "z" {
		t.Errorf("past=%v; want [a z] descending (100, 50)", p)
	}
}

func TestFilterOnOrAfter_SortsAscending(t *testing.T) {
	items := []exceptionInstance{{StartUnix: 300}, {StartUnix: 100}, {StartUnix: 200}}
	got := filterOnOrAfter(items, 150)
	if len(got) != 2 || got[0].StartUnix != 200 || got[1].StartUnix != 300 {
		t.Errorf("got=%v; want [200 300] ascending", got)
	}
}

// ---------------------------------------------------------------------------
// exceptionWorker: rate-limit retry & fail-fast
// ---------------------------------------------------------------------------

// makeIORuntime returns a minimal RuntimeContext with only the IO plumbing
// wired — enough for exceptionWorker's progress ticker without dragging in
// APIClient / token resolution.
func makeIORuntime(t *testing.T) *common.RuntimeContext {
	t.Helper()
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	return common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, defaultConfig(), f, core.AsBot)
}

func TestExceptionWorker_RetriesOnRateLimit(t *testing.T) {
	rt := makeIORuntime(t)

	var calls atomic.Int32
	worker := &exceptionWorker{
		Concurrency: 1,
		Runtime:     rt,
		Label:       "[test]",
		Total:       1,
		Do: func(_ context.Context, _ exceptionInstance) error {
			n := calls.Add(1)
			if n < 3 {
				return errs.NewAPIError(errs.SubtypeRateLimit, "slow").WithCode(190004).WithRetryable()
			}
			return nil
		},
	}
	if err := worker.Run(context.Background(), []exceptionInstance{{EventID: "e1"}}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestExceptionWorker_ContinuesAfterFailure(t *testing.T) {
	rt := makeIORuntime(t)

	worker := &exceptionWorker{
		Concurrency: 1,
		Runtime:     rt,
		Label:       "[test]",
		Total:       2,
		Do: func(_ context.Context, ex exceptionInstance) error {
			if ex.EventID == "e1" {
				return errs.NewAPIError(errs.SubtypeInvalidParameters, "boom").WithCode(190002)
			}
			return nil
		},
	}
	if err := worker.Run(context.Background(), []exceptionInstance{{EventID: "e1"}, {EventID: "e2"}}); err != nil {
		t.Fatalf("Run should not return per-item errors, got %v", err)
	}
	if worker.processed.Load() != 1 || worker.failed.Load() != 1 {
		t.Errorf("processed=%d failed=%d, want 1/1", worker.processed.Load(), worker.failed.Load())
	}
	failures := worker.Failures()
	if len(failures) != 1 || failures[0].EventID != "e1" {
		t.Errorf("failures = %+v, want single record for e1", failures)
	}
}

// ---------------------------------------------------------------------------
// +delete: dry-run, normal (non-recurring) fast path, apply-to enforcement
// ---------------------------------------------------------------------------

// A properly-shaped normal-event id ({uid}_0 with no recurrence field on the
// server response) is classified after one GET and then deleted; the total
// wire traffic is exactly one GET plus one DELETE.
func TestDelete_Normal_PreFetchesThenDeletes(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// GET the event: no recurrence, so classifyRecurringEvent returns
	// recurringKindNormal and --apply-to defaults to single.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_normal_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_normal_0",
				"summary":    "One-off",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_normal_0",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{}},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete",
		"--event-id", "uid_normal_0",
		"--calendar-id", "cal_test",
		"--yes", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), "uid_normal_0") {
		t.Errorf("stdout should contain event id, got: %s", stdout.String())
	}
}

// A malformed event id (no `_{digits}` suffix) is rejected up front with a
// typed validation error; no HTTP calls are attempted.
func TestDelete_MalformedEventID_Rejected(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete",
		"--event-id", "evt_plain",
		"--calendar-id", "cal_test",
		"--yes", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for malformed event id")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--event-id" {
		t.Errorf("param=%q, want --event-id", ve.Param)
	}
}

func TestDelete_Recurring_MissingApplyTo_Rejected(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// GET on the id: server says it's a recurring master.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_master_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"event": map[string]interface{}{
					"event_id":   "uid_master_0",
					"summary":    "Weekly",
					"start_time": map[string]interface{}{"timestamp": "1742515200"},
					"end_time":   map[string]interface{}{"timestamp": "1742518800"},
					"recurrence": "FREQ=WEEKLY",
				},
			},
		},
	})

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete",
		"--event-id", "uid_master_0",
		"--calendar-id", "cal_test",
		"--yes", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for missing --apply-to")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--"+flagApplyTo {
		t.Errorf("param=%q, want --%s", ve.Param, flagApplyTo)
	}
}

func TestDelete_Recurring_ApplyToSingleOnMaster_Rejected(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_master_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_master_0",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
				"recurrence": "FREQ=WEEKLY",
			}},
		},
	})
	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_master_0", "--calendar-id", "cal_test",
		"--apply-to", "single", "--yes", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --apply-to=single on master id")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %T", err)
	}
	if ve.Param != "--"+flagApplyTo {
		t.Errorf("param=%q", ve.Param)
	}
}

func TestDelete_Exception_ApplyToThisAndFollowing_Rejected(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_1742515200",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":     "uid_1742515200",
				"is_exception": true,
				"start_time":   map[string]interface{}{"timestamp": "1742515200"},
				"end_time":     map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_1742515200", "--calendar-id", "cal_test",
		"--apply-to", "this-and-following", "--yes", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --apply-to=this-and-following on exception")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %T", err)
	}
}

func TestDelete_ApplyToAll_DryRunLists4Steps(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_master_0", "--calendar-id", "cal_test",
		"--apply-to", "all", "--dry-run", "--yes", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	out := stdout.String()
	// Sanity: exception deletion step and master deletion step both appear.
	for _, want := range []string{"instances", "exception", "master"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run should mention %q, got: %s", want, out)
		}
	}
}

func TestDelete_InvalidApplyToValue(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_0", "--apply-to", "everything", "--yes", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected invalid value error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %T", err)
	}
	if ve.Param != "--"+flagApplyTo {
		t.Errorf("param=%q", ve.Param)
	}
}

// ---------------------------------------------------------------------------
// +delete: full apply-to=all flow (GET → instances → DELETE exception → DELETE master)
// ---------------------------------------------------------------------------

func TestDelete_ApplyToAll_DeletesExceptionsThenMaster(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// GET master.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	// /instances returns two confirmed and one cancelled exception. All three
	// are deleted: apply-to=all runs the delete_exception=true cleanup path,
	// which destroys cancelled placeholders (is_deleted=true) alongside live
	// rows so the series is fully cleared.
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
					map[string]interface{}{
						"event_id": "uid_1742700000", "is_exception": true, "status": "cancelled",
						"start_time": map[string]interface{}{"timestamp": "1742700000"},
					},
					map[string]interface{}{
						"event_id": "uid_1742800000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742800000"},
					},
				},
			},
		},
	})
	// Exception deletes (reusable — url matches all three exception ids).
	deleteEx := &httpmock.Stub{
		Method:   "DELETE",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_174",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteEx)
	// Master delete.
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--apply-to", "all", "--yes", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v; raw=%s", err, stdout.String())
	}
	data, _ := out["data"].(map[string]interface{})
	deletedEvent, _ := data["deleted_event"].(map[string]interface{})
	if deletedEvent["action"] != "deleted" {
		t.Errorf("deleted_event.action = %v", deletedEvent["action"])
	}
	exceptions, _ := data["exceptions"].(map[string]interface{})
	if v, _ := exceptions["deleted"].(float64); int(v) != 3 {
		t.Errorf("exceptions.deleted = %v, want 3 (cancelled placeholders are destroyed too)", exceptions["deleted"])
	}
	// All three exception DELETEs were captured.
	if len(deleteEx.CapturedBodies) != 3 {
		t.Errorf("expected 3 exception DELETEs, got %d", len(deleteEx.CapturedBodies))
	}
}

// Exception-cleanup DELETEs (inside --apply-to=all) carry delete_exception=true
// so the API destroys the exception row (is_deleted=true); the master DELETE,
// which is the user-facing target, must NOT carry that flag so downstream
// series computations still see it as a normal terminal delete.
func TestDelete_ApplyToAll_ExceptionDeletesUseDeleteExceptionFlag(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
				},
			},
		},
	})

	var (
		mu         sync.Mutex
		exURLs     []string
		masterURLs []string
	)
	// Exception DELETE — must include delete_exception=true.
	reg.Register(&httpmock.Stub{
		Method:   "DELETE",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742600000",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
		OnMatch: func(req *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			exURLs = append(exURLs, req.URL.String())
		},
	})
	// Master DELETE — user-facing target, must NOT include delete_exception.
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
		OnMatch: func(req *http.Request) {
			mu.Lock()
			defer mu.Unlock()
			masterURLs = append(masterURLs, req.URL.String())
		},
	})

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--apply-to", "all", "--yes", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(exURLs) != 1 {
		t.Fatalf("expected 1 exception DELETE, got %d (urls=%v)", len(exURLs), exURLs)
	}
	if !strings.Contains(exURLs[0], "delete_exception=true") {
		t.Errorf("exception DELETE must carry delete_exception=true, got url=%s", exURLs[0])
	}
	if len(masterURLs) != 1 {
		t.Fatalf("expected 1 master DELETE, got %d (urls=%v)", len(masterURLs), masterURLs)
	}
	if strings.Contains(masterURLs[0], "delete_exception") {
		t.Errorf("master DELETE must NOT carry delete_exception, got url=%s", masterURLs[0])
	}
}

// The apply-to=all cleanup pass destroys exceptions (delete_exception=true →
// is_deleted=true). cancelled exception rows are still occupying series-view
// slots, so they must be scanned in and destroyed along with the confirmed
// ones — otherwise a subsequent series-view would still see the cancelled
// placeholder.
func TestDelete_ApplyToAll_CancelledExceptionsAreAlsoDeleted(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
					map[string]interface{}{
						"event_id": "uid_1742700000", "is_exception": true, "status": "cancelled",
						"start_time": map[string]interface{}{"timestamp": "1742700000"},
					},
				},
			},
		},
	})

	var (
		mu         sync.Mutex
		deletedIDs []string
	)
	captured := func(req *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		deletedIDs = append(deletedIDs, req.URL.Path)
	}
	reg.Register(&httpmock.Stub{
		Method:   "DELETE",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742600000",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
		OnMatch:  captured,
	})
	reg.Register(&httpmock.Stub{
		Method:   "DELETE",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742700000",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
		OnMatch:  captured,
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	})

	if err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--apply-to", "all", "--yes", "--as", "bot",
	}, f, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if len(deletedIDs) != 2 {
		t.Fatalf("expected 2 exception DELETEs (confirmed + cancelled), got %d: %v", len(deletedIDs), deletedIDs)
	}
	// Both ids must have been hit.
	var sawConfirmed, sawCancelled bool
	for _, p := range deletedIDs {
		if strings.HasSuffix(p, "/uid_1742600000") {
			sawConfirmed = true
		}
		if strings.HasSuffix(p, "/uid_1742700000") {
			sawCancelled = true
		}
	}
	if !sawConfirmed || !sawCancelled {
		t.Errorf("cleanup must delete both confirmed and cancelled exceptions; got %v", deletedIDs)
	}
}

// DELETE returning 193003 ("event is deleted") is idempotent from the CLI's
// viewpoint — the row is already gone. deleteEventOnce swallows the error so
// exception-cleanup batches and master-delete steps do not fail when the
// target has been removed concurrently or on a previous run.
func TestDelete_193003_TreatedAsSuccess(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// Normal (non-recurring) event so the flow goes single-delete.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	// The server returns 193003 — deleteEventOnce must treat this as success.
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body:   map[string]interface{}{"code": 193003, "msg": "event is deleted"},
	})

	if err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--yes", "--as", "bot",
	}, f, stdout); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v; raw=%s", err, stdout.String())
	}
	data, _ := out["data"].(map[string]interface{})
	deleted, _ := data["deleted_event"].(map[string]interface{})
	if deleted["action"] != "deleted" {
		t.Errorf("deleted_event.action = %v, want deleted (193003 must be treated as success)", deleted["action"])
	}
}

// Direct unit-level pin on isEventAlreadyDeleted so the classification stays
// wired to APIError.Code == 193003 even if the surrounding call flow changes.
func TestIsEventAlreadyDeleted(t *testing.T) {
	if isEventAlreadyDeleted(nil) {
		t.Error("nil err must not be reported as already-deleted")
	}
	if isEventAlreadyDeleted(errors.New("plain")) {
		t.Error("non-APIError must not be reported as already-deleted")
	}
	if isEventAlreadyDeleted(errs.NewAPIError(errs.SubtypeNotFound, "gone").WithCode(193001)) {
		t.Error("193001 (not found) must not be reported as already-deleted")
	}
	if !isEventAlreadyDeleted(errs.NewAPIError(errs.SubtypeNotFound, "gone").WithCode(193003)) {
		t.Error("193003 must be reported as already-deleted")
	}
}

// Regression: --apply-to=all with --start/--end that echo the master's stored
// time must not push start_time/end_time onto the PATCH bodies. Prior to the
// fix, buildCalendarUpdateEventData added start_time/end_time whenever the
// flags were set, even though masterTimeChanged had already classified this as
// a non-time change and taken the exception-PATCH branch. Every exception then
// got rewritten to the master's first-instance time, collapsing the series.
func TestUpdate_ApplyToAll_EchoedTime_DoesNotOverwriteExceptionTime(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// Master at ts=1742515200 / 1742518800, weekly rrule.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	// One live exception at a different time — this is the row the bug would
	// clobber back to the master's 1742515200.
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
				},
			},
		},
	})
	patchException := &httpmock.Stub{
		Method:   "PATCH",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742600000",
		Reusable: true,
		Body: map[string]interface{}{"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{"event_id": "uid_1742600000"}}},
	}
	reg.Register(patchException)
	patchMaster := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{"event_id": "uid_0"}}},
	}
	reg.Register(patchMaster)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--apply-to", "all",
		// Echo master's stored time — masterTimeChanged should return false.
		"--start", "1742515200", "--end", "1742518800",
		"--summary", "NEW",
		"--skip-room-check", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	assertNoTimeFields := func(name string, raw []byte) {
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("%s: unmarshal body: %v; raw=%s", name, err, string(raw))
		}
		if _, ok := body["start_time"]; ok {
			t.Errorf("%s PATCH must NOT carry start_time (echoed time = non-time change); body=%s", name, string(raw))
		}
		if _, ok := body["end_time"]; ok {
			t.Errorf("%s PATCH must NOT carry end_time (echoed time = non-time change); body=%s", name, string(raw))
		}
		if got, _ := body["summary"].(string); got != "NEW" {
			t.Errorf("%s PATCH should still carry summary=NEW, got %v", name, body["summary"])
		}
	}
	if len(patchException.CapturedBodies) != 1 {
		t.Fatalf("expected 1 exception PATCH, got %d", len(patchException.CapturedBodies))
	}
	assertNoTimeFields("exception", patchException.CapturedBodies[0])
	assertNoTimeFields("master", patchMaster.CapturedBody)
}

// Sanity: a real time change (different --start/--end) still triggers the
// exception-deletion branch, so no exception PATCH goes out and the master
// PATCH carries the new start_time/end_time.
func TestUpdate_ApplyToAll_RealTimeChange_DeletesExceptionsAndPatchesMasterTime(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY;UNTIL=20260901T000000Z",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
				},
			},
		},
	})
	// Time change → the exception gets DELETEd, not PATCHed.
	reg.Register(&httpmock.Stub{
		Method:   "DELETE",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742600000",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
	})
	patchMaster := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{"event_id": "uid_0"}}},
	}
	reg.Register(patchMaster)

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update", "--event-id", "uid_0", "--calendar-id", "cal_r",
		"--apply-to", "all",
		"--start", "1742600000", "--end", "1742603600",
		"--skip-room-check", "--as", "bot",
	}, f, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(patchMaster.CapturedBody, &body); err != nil {
		t.Fatalf("master PATCH body: %v", err)
	}
	st, _ := body["start_time"].(map[string]interface{})
	if got, _ := st["timestamp"].(string); got != "1742600000" {
		t.Errorf("real time change: master PATCH start_time.timestamp = %v, want 1742600000", st)
	}
}

// ---------------------------------------------------------------------------
// +delete: this-and-following truncates rrule with UNTIL = pivot-1s
// ---------------------------------------------------------------------------

func TestDelete_ApplyToThisAndFollowing_TruncatesMaster(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())

	// Pivot instance id contains original time 1742600000; the server returns
	// it as a plain (non-exception) recurring instance, which our validator
	// allows for --apply-to=this-and-following.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_1742600000",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_1742600000",
				"start_time": map[string]interface{}{"timestamp": "1742600000"},
				"end_time":   map[string]interface{}{"timestamp": "1742603600"},
			}},
		},
	})
	// Fetch master via masterEventID rewrite.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY",
				"start_time": map[string]interface{}{"timestamp": "1742500000"},
				"end_time":   map[string]interface{}{"timestamp": "1742503600"},
			}},
		},
	})
	// instances returns two exceptions: one before pivot (ignored), one on/after
	// pivot (deleted).
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/calendar/v4/calendars/cal_r/events/uid_0/instances",
		Reusable: true,
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"has_more": false,
				"items": []interface{}{
					map[string]interface{}{
						"event_id": "uid_1742500000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742500000"},
					},
					map[string]interface{}{
						"event_id": "uid_1742600000", "is_exception": true, "status": "confirmed",
						"start_time": map[string]interface{}{"timestamp": "1742600000"},
					},
				},
			},
		},
	})
	// The one exception ≥ pivot gets DELETEd. Reusable+BodyFilter is not
	// needed here because the shape of the URL is unambiguous — only this
	// stub carries method=DELETE.
	deleteEx := &httpmock.Stub{
		Method:   "DELETE",
		URL:      "cal_r/events/uid_1742600000",
		Reusable: true,
		Body:     map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(deleteEx)
	// Master PATCH.
	patchMaster := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/calendar/v4/calendars/cal_r/events/uid_0",
		Body:   map[string]interface{}{"code": 0, "msg": "ok", "data": map[string]interface{}{"event": map[string]interface{}{"event_id": "uid_0"}}},
	}
	reg.Register(patchMaster)

	err := mountAndRun(t, CalendarDelete, []string{
		"+delete", "--event-id", "uid_1742600000", "--calendar-id", "cal_r",
		"--apply-to", "this-and-following", "--yes", "--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(patchMaster.CapturedBody, &body); err != nil {
		t.Fatalf("patch body: %v", err)
	}
	rrule, _ := body["recurrence"].(string)
	if !strings.Contains(rrule, "UNTIL=") {
		t.Errorf("expected UNTIL in truncated rrule, got %q", rrule)
	}
	if len(deleteEx.CapturedBodies) == 0 {
		t.Error("expected DELETE for the on/after-pivot exception")
	}
}

// ---------------------------------------------------------------------------
// +update: apply-to enforcement mirrors +delete
// ---------------------------------------------------------------------------

func TestUpdate_Recurring_MissingApplyTo_Rejected(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/events/uid_0",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{"event": map[string]interface{}{
				"event_id":   "uid_0",
				"recurrence": "FREQ=WEEKLY",
				"start_time": map[string]interface{}{"timestamp": "1742515200"},
				"end_time":   map[string]interface{}{"timestamp": "1742518800"},
			}},
		},
	})

	err := mountAndRun(t, CalendarUpdate, []string{
		"+update", "--event-id", "uid_0", "--calendar-id", "cal_test",
		"--summary", "New", "--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected --apply-to validation error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("got %T", err)
	}
	if ve.Param != "--"+flagApplyTo {
		t.Errorf("param=%q", ve.Param)
	}
}
