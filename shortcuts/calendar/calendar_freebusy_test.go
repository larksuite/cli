// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// -----------------------------------------------------------------------------
// Pure helpers
// -----------------------------------------------------------------------------

func TestMergeBusyIntervals_OverlapAndAdjacent(t *testing.T) {
	in := []*freebusyInterval{
		{StartTime: "2026-03-21T10:00:00+08:00", EndTime: "2026-03-21T11:00:00+08:00"},
		{StartTime: "2026-03-21T10:30:00+08:00", EndTime: "2026-03-21T12:00:00+08:00"},
		{StartTime: "2026-03-21T12:00:00+08:00", EndTime: "2026-03-21T13:00:00+08:00"},
		{StartTime: "2026-03-21T14:00:00+08:00", EndTime: "2026-03-21T15:00:00+08:00"},
	}
	got := mergeBusyIntervals(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 merged intervals, got %d: %+v", len(got), got)
	}
	if got[0].StartTime[:19] != "2026-03-21T10:00:00" || got[0].EndTime[:19] != "2026-03-21T13:00:00" {
		t.Errorf("first merged interval = %+v", got[0])
	}
	if got[1].StartTime[:19] != "2026-03-21T14:00:00" || got[1].EndTime[:19] != "2026-03-21T15:00:00" {
		t.Errorf("second merged interval = %+v", got[1])
	}
}

func TestMergeBusyIntervals_DedupExactDuplicates(t *testing.T) {
	in := []*freebusyInterval{
		{StartTime: "2026-03-21T10:00:00+08:00", EndTime: "2026-03-21T11:00:00+08:00"},
		{StartTime: "2026-03-21T10:00:00+08:00", EndTime: "2026-03-21T11:00:00+08:00"},
	}
	got := mergeBusyIntervals(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 merged interval, got %d: %+v", len(got), got)
	}
}

func TestPerUserFree_ClipsToWindowAndFiltersMinDuration(t *testing.T) {
	// window 09:00 - 12:00; busy 10:00 - 10:30 → free: 09:00-10:00, 10:30-12:00.
	loc, _ := time.LoadLocation("Asia/Shanghai")
	winStart := time.Date(2026, 3, 21, 9, 0, 0, 0, loc)
	winEnd := time.Date(2026, 3, 21, 12, 0, 0, 0, loc)
	busy := []*freebusyInterval{
		{StartTime: "2026-03-21T10:00:00+08:00", EndTime: "2026-03-21T10:30:00+08:00"},
	}
	// no filter → both slots
	full := perUserFree(busy, winStart, winEnd, 0)
	if len(full) != 2 {
		t.Fatalf("expected 2 free slots without filter, got %d: %+v", len(full), full)
	}
	// filter 90m → only the 10:30-12:00 slot survives
	filtered := perUserFree(busy, winStart, winEnd, 90*time.Minute)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 free slot with 90m filter, got %d: %+v", len(filtered), filtered)
	}
	if filtered[0].StartTime[:19] != "2026-03-21T10:30:00" || filtered[0].EndTime[:19] != "2026-03-21T12:00:00" {
		t.Errorf("filtered slot = %+v", filtered[0])
	}
	if filtered[0].Duration != "1h30m0s" {
		t.Errorf("duration = %q, want 1h30m0s", filtered[0].Duration)
	}
}

// commonFree replays the case 202 regression: three targets whose union
// covers 09:00-17:00; only 17:00-18:00 survives.
func TestCommonFree_Case202Regression(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	winStart := time.Date(2026, 3, 21, 9, 0, 0, 0, loc)
	winEnd := time.Date(2026, 3, 21, 18, 0, 0, 0, loc)

	a := []*freebusyInterval{
		{StartTime: "2026-03-21T09:00:00+08:00", EndTime: "2026-03-21T11:00:00+08:00"},
		{StartTime: "2026-03-21T12:00:00+08:00", EndTime: "2026-03-21T13:00:00+08:00"},
		{StartTime: "2026-03-21T14:00:00+08:00", EndTime: "2026-03-21T16:00:00+08:00"},
	}
	b := []*freebusyInterval{
		{StartTime: "2026-03-21T10:00:00+08:00", EndTime: "2026-03-21T12:00:00+08:00"},
		{StartTime: "2026-03-21T15:00:00+08:00", EndTime: "2026-03-21T17:00:00+08:00"},
	}
	c := []*freebusyInterval{
		{StartTime: "2026-03-21T09:00:00+08:00", EndTime: "2026-03-21T10:00:00+08:00"},
		{StartTime: "2026-03-21T12:00:00+08:00", EndTime: "2026-03-21T14:00:00+08:00"},
	}

	got := commonFree([][]*freebusyInterval{a, b, c}, winStart, winEnd, 30*time.Minute)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 common-free slot (17-18), got %d: %+v", len(got), got)
	}
	if got[0].StartTime[:19] != "2026-03-21T17:00:00" || got[0].EndTime[:19] != "2026-03-21T18:00:00" {
		t.Errorf("free slot = %+v; want 17:00~18:00", got[0])
	}
	if got[0].Duration != "1h0m0s" {
		t.Errorf("duration = %q, want 1h0m0s", got[0].Duration)
	}
}

// case 268 replay: two users share a 08:00-09:00 gap while both are busy the
// rest of the window; the LLM missed it, the sweep line must not.
func TestCommonFree_Case268Regression(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	winStart := time.Date(2026, 8, 4, 7, 0, 0, 0, loc)
	winEnd := time.Date(2026, 8, 4, 18, 0, 0, 0, loc)
	a := []*freebusyInterval{
		{StartTime: "2026-08-04T07:00:00+08:00", EndTime: "2026-08-04T08:00:00+08:00"},
		{StartTime: "2026-08-04T09:00:00+08:00", EndTime: "2026-08-04T18:00:00+08:00"},
	}
	b := []*freebusyInterval{
		{StartTime: "2026-08-04T07:30:00+08:00", EndTime: "2026-08-04T08:00:00+08:00"},
		{StartTime: "2026-08-04T09:00:00+08:00", EndTime: "2026-08-04T18:00:00+08:00"},
	}
	got := commonFree([][]*freebusyInterval{a, b}, winStart, winEnd, time.Hour)
	if len(got) != 1 {
		t.Fatalf("expected 1 common-free slot, got %d: %+v", len(got), got)
	}
	if got[0].StartTime[:19] != "2026-08-04T08:00:00" || got[0].EndTime[:19] != "2026-08-04T09:00:00" {
		t.Errorf("free slot = %+v", got[0])
	}
}

// -----------------------------------------------------------------------------
// Batch endpoint / view switches
// -----------------------------------------------------------------------------

func TestFreebusy_UsesBatchEndpoint_And_MergesPerUser(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id": "ou_a",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T10:00:00+08:00",
								"end_time":   "2026-03-21T11:00:00+08:00",
							},
							map[string]interface{}{
								// overlap → must merge with the previous.
								"start_time": "2026-03-21T10:30:00+08:00",
								"end_time":   "2026-03-21T12:00:00+08:00",
							},
							map[string]interface{}{
								// adjacent → must merge.
								"start_time": "2026-03-21T12:00:00+08:00",
								"end_time":   "2026-03-21T13:00:00+08:00",
							},
						},
					},
					map[string]interface{}{
						"user_id": "ou_b",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T14:00:00+08:00",
								"end_time":   "2026-03-21T15:00:00+08:00",
							},
						},
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a,ou_b",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Assert the request body carried user_ids batch.
	var req struct {
		UserIDs []string `json:"user_ids"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &req); err != nil {
		t.Fatalf("captured body was not JSON: %v", err)
	}
	if len(req.UserIDs) != 2 || req.UserIDs[0] != "ou_a" || req.UserIDs[1] != "ou_b" {
		t.Errorf("user_ids body = %v", req.UserIDs)
	}

	// Assert response shape: users[].busy after merge.
	var payload struct {
		Data struct {
			Users []struct {
				UserID string              `json:"user_id"`
				Busy   []*freebusyInterval `json:"busy"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v (out=%s)", err, stdout.String())
	}
	if len(payload.Data.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(payload.Data.Users))
	}
	if payload.Data.Users[0].UserID != "ou_a" || len(payload.Data.Users[0].Busy) != 1 {
		t.Errorf("ou_a busy should merge to 1 interval, got %+v", payload.Data.Users[0].Busy)
	}
	if payload.Data.Users[0].Busy[0].EndTime[:19] != "2026-03-21T13:00:00" {
		t.Errorf("ou_a merged end wrong: %+v", payload.Data.Users[0].Busy[0])
	}
}

func TestFreebusy_FreeView(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id": "ou_a",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T10:00:00+08:00",
								"end_time":   "2026-03-21T11:00:00+08:00",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21T09:00:00+08:00",
		"--end", "2026-03-21T12:00:00+08:00",
		"--user-id", "ou_a",
		"--type", "free",
		"--min-duration", "30m",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var payload struct {
		Data struct {
			Users []struct {
				UserID string              `json:"user_id"`
				Free   []*freebusyFreeSlot `json:"free"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v (out=%s)", err, stdout.String())
	}
	if len(payload.Data.Users) != 1 {
		t.Fatalf("want 1 user, got %d", len(payload.Data.Users))
	}
	if len(payload.Data.Users[0].Free) != 2 {
		t.Fatalf("want 2 free slots (09-10, 11-12), got %d: %+v", len(payload.Data.Users[0].Free), payload.Data.Users[0].Free)
	}
	if payload.Data.Users[0].Free[0].Duration != "1h0m0s" {
		t.Errorf("first free duration = %q", payload.Data.Users[0].Free[0].Duration)
	}
}

func TestFreebusy_CommonFreeView(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id": "ou_a",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T09:00:00+08:00",
								"end_time":   "2026-03-21T10:00:00+08:00",
							},
						},
					},
					map[string]interface{}{
						"user_id": "ou_b",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T11:00:00+08:00",
								"end_time":   "2026-03-21T12:00:00+08:00",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21T09:00:00+08:00",
		"--end", "2026-03-21T18:00:00+08:00",
		"--user-id", "ou_a,ou_b",
		"--type", "common_free",
		"--min-duration", "30m",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var payload struct {
		Data struct {
			CommonFree []*freebusyFreeSlot `json:"common_free"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v (out=%s)", err, stdout.String())
	}
	// Expect free slots: 10-11 (1h), 12-18 (6h) — compare as absolute instants
	// so the assertion is stable regardless of the runner's local TZ.
	if len(payload.Data.CommonFree) != 2 {
		t.Fatalf("want 2 common-free slots, got %d: %+v", len(payload.Data.CommonFree), payload.Data.CommonFree)
	}
	assertFreebusySlotInstant(t, "first common-free slot",
		payload.Data.CommonFree[0],
		"2026-03-21T10:00:00+08:00", "2026-03-21T11:00:00+08:00")
	assertFreebusySlotInstant(t, "second common-free slot",
		payload.Data.CommonFree[1],
		"2026-03-21T12:00:00+08:00", "2026-03-21T18:00:00+08:00")
}

// assertFreebusySlotInstant compares a slot's emitted start/end to the expected
// wall-clock at a specific offset by parsing both sides as RFC3339, so the
// assertion holds under any TZ the test runner uses.
func assertFreebusySlotInstant(t *testing.T, label string, slot *freebusyFreeSlot, wantStart, wantEnd string) {
	t.Helper()
	wantStartT, err := time.Parse(time.RFC3339, wantStart)
	if err != nil {
		t.Fatalf("%s: bad wantStart %q: %v", label, wantStart, err)
	}
	wantEndT, err := time.Parse(time.RFC3339, wantEnd)
	if err != nil {
		t.Fatalf("%s: bad wantEnd %q: %v", label, wantEnd, err)
	}
	gotStart, err := time.Parse(time.RFC3339, slot.StartTime)
	if err != nil {
		t.Fatalf("%s: unparseable slot.StartTime %q: %v", label, slot.StartTime, err)
	}
	gotEnd, err := time.Parse(time.RFC3339, slot.EndTime)
	if err != nil {
		t.Fatalf("%s: unparseable slot.EndTime %q: %v", label, slot.EndTime, err)
	}
	if !gotStart.Equal(wantStartT) || !gotEnd.Equal(wantEndT) {
		t.Errorf("%s = %+v, want start=%s end=%s", label, slot, wantStart, wantEnd)
	}
}

// A single --user-id with common_free returns the same shape as multi-user
// common_free; spec explicitly forbids a distinct single-user error path.
func TestFreebusy_CommonFree_SingleUser_UsesSameShape(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id": "ou_a",
						"freebusy_items": []interface{}{
							map[string]interface{}{
								"start_time": "2026-03-21T10:00:00+08:00",
								"end_time":   "2026-03-21T11:00:00+08:00",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21T09:00:00+08:00",
		"--end", "2026-03-21T12:00:00+08:00",
		"--user-id", "ou_a",
		"--type", "common_free",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	var payload struct {
		Data struct {
			CommonFree []*freebusyFreeSlot `json:"common_free"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v (out=%s)", err, stdout.String())
	}
	if len(payload.Data.CommonFree) != 2 {
		t.Fatalf("want 2 free slots for single user, got %d: %+v", len(payload.Data.CommonFree), payload.Data.CommonFree)
	}
}

// -----------------------------------------------------------------------------
// Validation
// -----------------------------------------------------------------------------

func TestFreebusy_InvalidTypeEnum(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a",
		"--type", "bogus",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --type")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--type" {
		t.Errorf("param=%q, want --type", ve.Param)
	}
}

func TestFreebusy_InvalidMinDuration(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a",
		"--min-duration", "not-a-duration",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected validation error for --min-duration")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want *errs.ValidationError, got %T", err)
	}
	if ve.Param != "--min-duration" {
		t.Errorf("param=%q, want --min-duration", ve.Param)
	}
}

func TestFreebusy_UserIdSliceDedup(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id":        "ou_a",
						"freebusy_items": []interface{}{},
					},
					map[string]interface{}{
						"user_id":        "ou_b",
						"freebusy_items": []interface{}{},
					},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a,ou_a, ou_b ",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(string(stub.CapturedBody), `"user_ids":["ou_a","ou_b"]`) {
		t.Errorf("captured body did not carry deduped user_ids: %s", string(stub.CapturedBody))
	}
}

// -----------------------------------------------------------------------------
// raw_busy view
// -----------------------------------------------------------------------------

// raw_busy must NOT merge and MUST preserve rsvp_status per event, so callers
// can count events and read the per-event rsvp signal.
func TestFreebusy_RawBusyView_NoMerge_KeepsRSVP(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id": "ou_a",
						"freebusy_items": []interface{}{
							// Two overlapping events at the same time slice; busy view
							// would merge these into one, raw_busy must keep both.
							map[string]interface{}{
								"start_time":  "2026-03-21T10:00:00+08:00",
								"end_time":    "2026-03-21T11:00:00+08:00",
								"rsvp_status": "accept",
							},
							map[string]interface{}{
								"start_time":  "2026-03-21T10:30:00+08:00",
								"end_time":    "2026-03-21T11:30:00+08:00",
								"rsvp_status": "tentative",
							},
						},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a",
		"--type", "raw_busy",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	var payload struct {
		Data struct {
			Users []struct {
				UserID  string             `json:"user_id"`
				RawBusy []*freebusyRawItem `json:"raw_busy"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal stdout: %v (out=%s)", err, stdout.String())
	}
	if len(payload.Data.Users) != 1 || len(payload.Data.Users[0].RawBusy) != 2 {
		t.Fatalf("raw_busy must keep 2 events, got %+v", payload.Data.Users)
	}
	got := payload.Data.Users[0].RawBusy
	if got[0].RSVPStatus != "accept" || got[1].RSVPStatus != "tentative" {
		t.Errorf("rsvp_status lost or reordered: %+v, %+v", got[0], got[1])
	}
	// Sort by start ascending.
	if got[0].StartTime > got[1].StartTime {
		t.Errorf("raw_busy items must be sorted by start ascending, got: %+v, %+v", got[0], got[1])
	}
}

// raw_busy on a user with no events emits an empty array, not nil, keeping
// the JSON shape stable for downstream parsers.
func TestFreebusy_RawBusyView_EmptyUserYieldsEmptyArray(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id":        "ou_a",
						"freebusy_items": []interface{}{},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a",
		"--type", "raw_busy",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stdout.String(), `"raw_busy": []`) {
		t.Errorf("expected explicit empty array raw_busy: [], got: %s", stdout.String())
	}
}

// --min-duration is documented as ignored for raw_busy: caller passing it must
// receive a stderr hint but the command must not fail and payload is unchanged.
func TestFreebusy_RawBusy_MinDurationEmitsHintOnStderr(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/freebusy/batch",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"freebusy_lists": []interface{}{
					map[string]interface{}{
						"user_id":        "ou_a",
						"freebusy_items": []interface{}{},
					},
				},
			},
		},
	})

	err := mountAndRun(t, CalendarFreebusy, []string{
		"+freebusy",
		"--start", "2026-03-21",
		"--end", "2026-03-21",
		"--user-id", "ou_a",
		"--type", "raw_busy",
		"--min-duration", "30m",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !strings.Contains(stderr.String(), "ignored when --type=raw_busy") {
		t.Errorf("expected stderr hint about ignored min-duration for raw_busy, got: %s", stderr.String())
	}
}

// -----------------------------------------------------------------------------
// off-hours hint (--type free / common_free)
// -----------------------------------------------------------------------------

func TestSlotOverlapsOffHours(t *testing.T) {
	cases := []struct {
		name  string
		start string
		end   string
		want  bool
	}{
		{"pure working hours", "2026-03-21T10:00:00+08:00", "2026-03-21T12:00:00+08:00", false},
		{"early-morning start", "2026-03-21T07:00:00+08:00", "2026-03-21T09:00:00+08:00", true},
		{"exact 08 boundary is working", "2026-03-21T08:00:00+08:00", "2026-03-21T09:00:00+08:00", false},
		{"late-night end", "2026-03-21T21:00:00+08:00", "2026-03-21T23:30:00+08:00", true},
		{"exact 22 boundary hits off-hours", "2026-03-21T22:00:00+08:00", "2026-03-21T23:00:00+08:00", true},
		{"cross-midnight into next day early morning", "2026-03-21T21:00:00+08:00", "2026-03-22T09:00:00+08:00", true},
		{"multi-day but only working hours each day (impossible)", "2026-03-21T09:00:00+08:00", "2026-03-21T18:00:00+08:00", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := slotOverlapsOffHours(c.start, c.end)
			if got != c.want {
				t.Errorf("slotOverlapsOffHours(%s,%s)=%v want %v", c.start, c.end, got, c.want)
			}
		})
	}
}

// A --type free response whose slots straddle off-hours must emit a stderr
// hint about the off-hours band. Exercised directly against
// writeFreebusyOffHoursHint so the slot's own offset (+08:00) drives the check
// and the assertion is independent of the runner's local TZ.
func TestFreebusy_FreeView_OffHoursHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	runtime := &common.RuntimeContext{Factory: f}
	slots := []*freebusyFreeSlot{
		{
			StartTime: "2026-03-21T06:00:00+08:00",
			EndTime:   "2026-03-21T09:00:00+08:00",
			Duration:  "3h0m0s",
		},
	}
	writeFreebusyOffHoursHint(runtime, freebusyTypeFree, slots)
	if !strings.Contains(stderr.String(), "off-hours 22:00-08:00") {
		t.Errorf("expected off-hours hint on stderr, got: %q", stderr.String())
	}
}

// A --type free response that stays inside working hours must NOT emit the
// off-hours hint. Same rationale as TestFreebusy_FreeView_OffHoursHint.
func TestFreebusy_FreeView_NoHintDuringWorkingHours(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	runtime := &common.RuntimeContext{Factory: f}
	slots := []*freebusyFreeSlot{
		{
			StartTime: "2026-03-21T09:00:00+08:00",
			EndTime:   "2026-03-21T18:00:00+08:00",
			Duration:  "9h0m0s",
		},
	}
	writeFreebusyOffHoursHint(runtime, freebusyTypeFree, slots)
	if strings.Contains(stderr.String(), "off-hours") {
		t.Errorf("unexpected off-hours hint on stderr for 09-18 window: %q", stderr.String())
	}
}

// The hint also fires for --type common_free.
func TestFreebusy_CommonFree_OffHoursHint(t *testing.T) {
	f, _, stderr, _ := cmdutil.TestFactory(t, defaultConfig())
	runtime := &common.RuntimeContext{Factory: f}
	slots := []*freebusyFreeSlot{
		{
			StartTime: "2026-03-21T21:00:00+08:00",
			EndTime:   "2026-03-21T23:30:00+08:00",
			Duration:  "2h30m0s",
		},
	}
	writeFreebusyOffHoursHint(runtime, freebusyTypeCommonFree, slots)
	if !strings.Contains(stderr.String(), "off-hours 22:00-08:00") {
		t.Errorf("expected off-hours hint on stderr for common_free spanning 21-23:30, got: %q", stderr.String())
	}
}

// The off-hours hint must NOT fire for --type busy or --type raw_busy since
// those views are not about candidate slots.
func TestFreebusy_BusyAndRawBusy_NoOffHoursHint(t *testing.T) {
	for _, typ := range []string{"busy", "raw_busy"} {
		t.Run(typ, func(t *testing.T) {
			f, stdout, stderr, reg := cmdutil.TestFactory(t, defaultConfig())
			reg.Register(&httpmock.Stub{
				Method: "POST",
				URL:    "/open-apis/calendar/v4/freebusy/batch",
				Body: map[string]interface{}{
					"code": 0, "msg": "ok",
					"data": map[string]interface{}{
						"freebusy_lists": []interface{}{
							map[string]interface{}{
								"user_id":        "ou_a",
								"freebusy_items": []interface{}{},
							},
						},
					},
				},
			})

			err := mountAndRun(t, CalendarFreebusy, []string{
				"+freebusy",
				"--start", "2026-03-21T06:00:00+08:00",
				"--end", "2026-03-21T09:00:00+08:00",
				"--user-id", "ou_a",
				"--type", typ,
				"--as", "bot",
			}, f, stdout)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if strings.Contains(stderr.String(), "off-hours") {
				t.Errorf("%s must not emit off-hours hint, got: %q", typ, stderr.String())
			}
		})
	}
}
