// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/output"
)

// TestClampOpenedTimeWindow covers the 3-month / 1-year boundary logic that
// narrows --opened-since / --opened-until and generates the multi-slice notice.
func TestClampOpenedTimeWindow(t *testing.T) {
	t.Parallel()

	// Fixed "now" keeps RFC3339 output stable across runs.
	now := time.Date(2026, 4, 24, 16, 0, 0, 0, time.UTC)
	day := int64(86400)

	t.Run("no opened-since: no clamp, no notice", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{OpenedUntil: "2026-04-01"}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil || notice != "" {
			t.Fatalf("got notice=%q err=%v, want both empty", notice, err)
		}
		if spec.OpenedSince != "" || spec.OpenedUntil != "2026-04-01" {
			t.Fatalf("spec mutated unexpectedly: %+v", spec)
		}
	})

	t.Run("span within 90d: no clamp", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{OpenedSince: "30d"}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil || notice != "" {
			t.Fatalf("got notice=%q err=%v, want both empty", notice, err)
		}
		if spec.OpenedSince != "30d" {
			t.Fatalf("spec.OpenedSince mutated: %q", spec.OpenedSince)
		}
	})

	t.Run("exactly 90 days: no clamp", func(t *testing.T) {
		t.Parallel()
		since := now.Unix() - 90*day
		spec := driveSearchSpec{
			OpenedSince: time.Unix(since, 0).UTC().Format(time.RFC3339),
		}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil || notice != "" {
			t.Fatalf("got notice=%q err=%v, want no clamp at boundary", notice, err)
		}
	})

	t.Run("91 days: 2-slice clamp", func(t *testing.T) {
		t.Parallel()
		since := now.Unix() - 91*day
		spec := driveSearchSpec{
			OpenedSince: time.Unix(since, 0).UTC().Format(time.RFC3339),
		}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.Contains(notice, "2 slices total") {
			t.Fatalf("expected '2 slices total' in notice, got:\n%s", notice)
		}
		// Each slice line — including slice 1 — must spell out concrete
		// --opened-since / --opened-until values so a paginating agent can
		// copy them verbatim instead of re-using the user's original
		// relative time (which would drift against time.Now()).
		for _, label := range []string{"[slice 1/2 current]", "[slice 2/2]"} {
			var line string
			for _, l := range strings.Split(notice, "\n") {
				if strings.Contains(l, label) {
					line = l
					break
				}
			}
			if line == "" {
				t.Fatalf("missing %s line, got:\n%s", label, notice)
			}
			if !strings.Contains(line, "--opened-since ") || !strings.Contains(line, "--opened-until ") {
				t.Fatalf("%s line must spell out both flag values, got: %q\nfull notice:\n%s", label, line, notice)
			}
		}
		// After clamp the request window is exactly the most recent 90 days.
		clampedSince, err := parseTimeValue(spec.OpenedSince, now)
		if err != nil {
			t.Fatalf("rewritten opened-since not parseable: %v", err)
		}
		clampedUntil, err := parseTimeValue(spec.OpenedUntil, now)
		if err != nil {
			t.Fatalf("rewritten opened-until not parseable: %v", err)
		}
		if clampedUntil-clampedSince != 90*day {
			t.Fatalf("clamped span = %d days, want 90", (clampedUntil-clampedSince)/day)
		}
		if clampedUntil != now.Unix() {
			t.Fatalf("clamped until should default to now; got %d, want %d", clampedUntil, now.Unix())
		}
	})

	t.Run("8 months: 3-slice clamp with shorter tail", func(t *testing.T) {
		t.Parallel()
		since := now.Unix() - 240*day // 8m ≈ 240 days
		spec := driveSearchSpec{
			OpenedSince: time.Unix(since, 0).UTC().Format(time.RFC3339),
		}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		for _, want := range []string{"3 slices total", "[slice 1/3 current]", "[slice 2/3]", "[slice 3/3]"} {
			if !strings.Contains(notice, want) {
				t.Fatalf("missing %q in notice:\n%s", want, notice)
			}
		}
	})

	t.Run("365 days: 5-slice clamp at upper bound", func(t *testing.T) {
		t.Parallel()
		since := now.Unix() - 365*day
		spec := driveSearchSpec{
			OpenedSince: time.Unix(since, 0).UTC().Format(time.RFC3339),
		}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil {
			t.Fatalf("365 days should clamp, got err: %v", err)
		}
		if !strings.Contains(notice, "5 slices total") {
			t.Fatalf("expected '5 slices total' for 365-day span, got:\n%s", notice)
		}
	})

	t.Run("over 365 days: hard-cap error", func(t *testing.T) {
		t.Parallel()
		since := now.Unix() - 366*day
		spec := driveSearchSpec{
			OpenedSince: time.Unix(since, 0).UTC().Format(time.RFC3339),
		}
		_, err := clampOpenedTimeWindow(&spec, now)
		if err == nil {
			t.Fatal("expected error for 366-day span, got nil")
		}
		if !strings.Contains(err.Error(), "365-day") {
			t.Fatalf("error should mention 365-day cap, got: %v", err)
		}
	})

	t.Run("since > until: no clamp, defer to downstream", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{
			OpenedSince: "2026-04-01",
			OpenedUntil: "2026-03-01",
		}
		notice, err := clampOpenedTimeWindow(&spec, now)
		if err != nil || notice != "" {
			t.Fatalf("got notice=%q err=%v, want both empty for inverted range", notice, err)
		}
	})

	t.Run("invalid opened-since: validation error", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{OpenedSince: "not-a-date"}
		_, err := clampOpenedTimeWindow(&spec, now)
		if err == nil {
			t.Fatal("expected validation error for unparseable since")
		}
		if !strings.Contains(err.Error(), "--opened-since") {
			t.Fatalf("error should name the flag, got: %v", err)
		}
	})
}

func TestParseDriveSearchPageSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{"empty defaults to 15", "", 15, false},
		{"valid in-range", "10", 10, false},
		{"zero falls back to 15", "0", 15, false},
		{"negative falls back to 15", "-5", 15, false},
		{"clamps to 20 when exceeded", "100", 20, false},
		{"non-numeric is a hard error", "abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseDriveSearchPageSize(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestValidateDocTypes(t *testing.T) {
	t.Parallel()
	if err := validateDocTypes(nil); err != nil {
		t.Fatalf("nil slice should be valid, got: %v", err)
	}
	if err := validateDocTypes([]string{"DOC", "SHEET", "BITABLE"}); err != nil {
		t.Fatalf("known values should pass, got: %v", err)
	}
	err := validateDocTypes([]string{"DOC", "PIE"})
	if err == nil || !strings.Contains(err.Error(), "PIE") {
		t.Fatalf("expected error naming the unknown value, got: %v", err)
	}
}

func TestUpperAll(t *testing.T) {
	t.Parallel()
	if got := upperAll(nil); got != nil {
		t.Fatalf("nil input should return nil, got %v", got)
	}
	got := upperAll([]string{"docx", "Sheet", "BITABLE"})
	want := []string{"DOCX", "SHEET", "BITABLE"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestValidateDriveSearchIDs(t *testing.T) {
	t.Parallel()

	t.Run("all valid", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{
			CreatorIDs: []string{"ou_aaa"},
			ChatIDs:    []string{"oc_xxx"},
			SharerIDs:  []string{"ou_bbb"},
		}
		if err := validateDriveSearchIDs(spec); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("bad creator id format", func(t *testing.T) {
		t.Parallel()
		err := validateDriveSearchIDs(driveSearchSpec{CreatorIDs: []string{"u_bad"}})
		if err == nil || !strings.Contains(err.Error(), "--creator-ids") {
			t.Fatalf("expected --creator-ids error, got: %v", err)
		}
	})

	t.Run("bad chat id format", func(t *testing.T) {
		t.Parallel()
		err := validateDriveSearchIDs(driveSearchSpec{ChatIDs: []string{"chat_bad"}})
		if err == nil || !strings.Contains(err.Error(), "--chat-ids") {
			t.Fatalf("expected --chat-ids error, got: %v", err)
		}
	})

	t.Run("bad sharer id format", func(t *testing.T) {
		t.Parallel()
		err := validateDriveSearchIDs(driveSearchSpec{SharerIDs: []string{"u_bad"}})
		if err == nil || !strings.Contains(err.Error(), "--sharer-ids") {
			t.Fatalf("expected --sharer-ids error, got: %v", err)
		}
	})

	t.Run("chat ids exactly at cap is allowed", func(t *testing.T) {
		t.Parallel()
		ids := make([]string, driveSearchMaxChatIDs)
		for i := range ids {
			ids[i] = "oc_x"
		}
		if err := validateDriveSearchIDs(driveSearchSpec{ChatIDs: ids}); err != nil {
			t.Fatalf("exactly cap should pass, got: %v", err)
		}
	})

	t.Run("chat ids over cap", func(t *testing.T) {
		t.Parallel()
		ids := make([]string, driveSearchMaxChatIDs+1)
		for i := range ids {
			ids[i] = "oc_x"
		}
		err := validateDriveSearchIDs(driveSearchSpec{ChatIDs: ids})
		if err == nil || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected cap error, got: %v", err)
		}
	})

	t.Run("sharer ids exactly at cap is allowed", func(t *testing.T) {
		t.Parallel()
		ids := make([]string, driveSearchMaxSharerIDs)
		for i := range ids {
			ids[i] = "ou_x"
		}
		if err := validateDriveSearchIDs(driveSearchSpec{SharerIDs: ids}); err != nil {
			t.Fatalf("exactly cap should pass, got: %v", err)
		}
	})

	t.Run("sharer ids over cap", func(t *testing.T) {
		t.Parallel()
		ids := make([]string, driveSearchMaxSharerIDs+1)
		for i := range ids {
			ids[i] = "ou_x"
		}
		err := validateDriveSearchIDs(driveSearchSpec{SharerIDs: ids})
		if err == nil || !strings.Contains(err.Error(), "max") {
			t.Fatalf("expected cap error, got: %v", err)
		}
	})
}

func TestBuildTimeRangeFilter(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 24, 16, 0, 0, 0, time.UTC)

	t.Run("both empty: nil range, no notice", func(t *testing.T) {
		t.Parallel()
		rng, notices, err := buildTimeRangeFilter("open_time", "", "", now)
		if err != nil || rng != nil || len(notices) != 0 {
			t.Fatalf("got rng=%v notices=%v err=%v", rng, notices, err)
		}
	})

	t.Run("open_time passes through without snap", func(t *testing.T) {
		t.Parallel()
		rng, notices, err := buildTimeRangeFilter("open_time",
			"2026-04-20T10:30:45+08:00", "2026-04-21T11:45:30+08:00", now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(notices) != 0 {
			t.Fatalf("open_time should not snap, got notices: %v", notices)
		}
		if rng["start"] == nil || rng["end"] == nil {
			t.Fatalf("range missing endpoints: %v", rng)
		}
	})

	t.Run("my_edit_time snaps sub-hour values", func(t *testing.T) {
		t.Parallel()
		rng, notices, err := buildTimeRangeFilter("my_edit_time",
			"2026-04-20T10:30:45+08:00", "2026-04-21T11:45:30+08:00", now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(notices) != 2 {
			t.Fatalf("expected 2 snap notices (start + end), got %d: %v", len(notices), notices)
		}
		startUnix := rng["start"].(int64)
		endUnix := rng["end"].(int64)
		if startUnix%3600 != 0 || endUnix%3600 != 0 {
			t.Fatalf("snapped values should align to hour: start=%d end=%d", startUnix, endUnix)
		}
	})

	t.Run("invalid since surfaces with flag name", func(t *testing.T) {
		t.Parallel()
		_, _, err := buildTimeRangeFilter("my_edit_time", "garbage", "", now)
		if err == nil || !strings.Contains(err.Error(), "--edited-since") {
			t.Fatalf("expected --edited-since in error, got: %v", err)
		}
	})

	t.Run("invalid until surfaces with flag name", func(t *testing.T) {
		t.Parallel()
		_, _, err := buildTimeRangeFilter("open_time", "", "garbage", now)
		if err == nil || !strings.Contains(err.Error(), "--opened-until") {
			t.Fatalf("expected --opened-until in error, got: %v", err)
		}
	})
}

func TestFloorAndCeilHour(t *testing.T) {
	t.Parallel()
	// 16:23:45 = unix 1745195025 (arbitrary)
	t.Run("floor truncates", func(t *testing.T) {
		t.Parallel()
		if got := floorHour(1745195025); got%3600 != 0 || got >= 1745195025 {
			t.Fatalf("floor(1745195025)=%d invalid", got)
		}
	})
	t.Run("ceil rounds up", func(t *testing.T) {
		t.Parallel()
		got := ceilHour(1745195025)
		if got%3600 != 0 || got <= 1745195025 {
			t.Fatalf("ceil(1745195025)=%d invalid", got)
		}
	})
	t.Run("ceil at exact hour is no-op", func(t *testing.T) {
		t.Parallel()
		exact := int64(1745193600)
		if got := ceilHour(exact); got != exact {
			t.Fatalf("ceil at hour boundary should be identity, got %d", got)
		}
	})
}

func TestParseTimeValue(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 24, 16, 0, 0, 0, time.Local)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty errors", "", true},
		{"7d relative", "7d", false},
		{"1m relative", "1m", false},
		{"1y relative", "1y", false},
		{"date-only YYYY-MM-DD", "2026-04-01", false},
		{"datetime with space", "2026-04-01 10:00:00", false},
		{"datetime with T", "2026-04-01T10:00:00", false},
		{"RFC3339 with offset", "2026-04-01T10:00:00+08:00", false},
		{"unix seconds", "1745193600", false},
		{"too short to be unix, garbage", "12345", true},
		{"YYYYMMDD digits not unix", "20260423", true},
		{"unparseable text", "not-a-date", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseTimeValue(tt.input, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v, wantErr=%v", err, tt.wantErr)
			}
		})
	}

	// Sanity: relative units must scale correctly. A regression where "1m"
	// silently meant "1 minute" instead of "30 days" would slip past the
	// wantErr-only table above; this guards the unit semantics.
	t.Run("relative units scale: 7d < 1m < 1y", func(t *testing.T) {
		t.Parallel()
		got7d, err := parseTimeValue("7d", now)
		if err != nil {
			t.Fatalf("7d: %v", err)
		}
		got1m, err := parseTimeValue("1m", now)
		if err != nil {
			t.Fatalf("1m: %v", err)
		}
		got1y, err := parseTimeValue("1y", now)
		if err != nil {
			t.Fatalf("1y: %v", err)
		}
		// All three are "now minus N days"; larger N means smaller (older) unix.
		if !(got1y < got1m && got1m < got7d && got7d < now.Unix()) {
			t.Fatalf("expected got1y < got1m < got7d < now; got %d %d %d (now=%d)",
				got1y, got1m, got7d, now.Unix())
		}
		// Spot-check the conversions: "1m" = 30d, "1y" = 365d.
		const day = int64(86400)
		if now.Unix()-got1m != 30*day {
			t.Fatalf("'1m' should resolve to now-30d, got delta %d days", (now.Unix()-got1m)/day)
		}
		if now.Unix()-got1y != 365*day {
			t.Fatalf("'1y' should resolve to now-365d, got delta %d days", (now.Unix()-got1y)/day)
		}
	})

	// Sanity: unix-seconds round-trips exactly (no parsing as date).
	t.Run("unix-seconds input round-trips", func(t *testing.T) {
		t.Parallel()
		got, err := parseTimeValue("1745193600", now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != 1745193600 {
			t.Fatalf("unix round-trip got %d, want 1745193600", got)
		}
	})

	// Regression: a 13-digit epoch-millis timestamp must be normalized to
	// seconds. Previously it silently parsed as year-57000 and tripped the
	// 1-year cap downstream with a misleading "exceeds 365 days" message.
	t.Run("epoch-millis input normalizes to seconds", func(t *testing.T) {
		t.Parallel()
		got, err := parseTimeValue("1745193600000", now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got != 1745193600 {
			t.Fatalf("ms timestamp should normalize to %d seconds, got %d", int64(1745193600), got)
		}
	})
}

func TestUnixToISO8601(t *testing.T) {
	t.Parallel()
	const sec int64 = 1745193600                              // 2025-04-21 00:00 UTC; only the YYYY-MM-DD prefix is checked below to stay timezone-agnostic
	wantPrefix := time.Unix(sec, 0).Format(time.RFC3339)[:10] // YYYY-MM-DD prefix is timezone-stable

	tests := []struct {
		name string
		in   interface{}
		want string // empty means expect empty result
	}{
		{"int64", sec, wantPrefix},
		{"int", int(sec), wantPrefix},
		{"float64", float64(sec), wantPrefix},
		{"json.Number", json.Number("1745193600"), wantPrefix},
		{"string numeric", "1745193600", wantPrefix},
		{"milliseconds get divided", sec * 1000, wantPrefix},
		{"nil returns empty", nil, ""},
		{"bool ignored", true, ""},
		{"unparseable string", "abc", ""},
		{"NaN returns empty", math.NaN(), ""},
		{"Inf returns empty", math.Inf(1), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := unixToISO8601(tt.in)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("want empty, got %q", got)
				}
				return
			}
			if !strings.HasPrefix(got, tt.want) {
				t.Fatalf("got %q, want prefix %q", got, tt.want)
			}
		})
	}
}

func TestAddDriveSearchIsoTimeFields(t *testing.T) {
	t.Parallel()

	t.Run("non-array input returns nil", func(t *testing.T) {
		t.Parallel()
		if got := addDriveSearchIsoTimeFields("not-an-array"); got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})

	t.Run("annotates *_time at top level", func(t *testing.T) {
		t.Parallel()
		items := []interface{}{
			map[string]interface{}{"open_time": int64(1745193600)},
		}
		row := addDriveSearchIsoTimeFields(items)[0].(map[string]interface{})
		if _, ok := row["open_time_iso"].(string); !ok {
			t.Fatalf("open_time_iso should have been added, got: %v", row)
		}
	})

	t.Run("recurses into nested map and annotates", func(t *testing.T) {
		t.Parallel()
		items := []interface{}{
			map[string]interface{}{
				"result_meta": map[string]interface{}{
					"update_time": json.Number("1745193600"),
				},
			},
		}
		row := addDriveSearchIsoTimeFields(items)[0].(map[string]interface{})
		meta := row["result_meta"].(map[string]interface{})
		if _, ok := meta["update_time_iso"].(string); !ok {
			t.Fatalf("nested update_time_iso missing, got: %v", meta)
		}
	})

	t.Run("standalone *_time_iso key passes through", func(t *testing.T) {
		t.Parallel()
		// No sibling *_time key, so the iso-suffix passthrough branch is the
		// only one that touches this key — deterministic by construction.
		items := []interface{}{
			map[string]interface{}{"some_time_iso": "preserved"},
		}
		row := addDriveSearchIsoTimeFields(items)[0].(map[string]interface{})
		if row["some_time_iso"] != "preserved" {
			t.Fatalf("existing _time_iso value should pass through, got: %v", row["some_time_iso"])
		}
	})

	// Regression: when both *_time and *_time_iso are present in the same map,
	// the pre-existing _iso value must always win, regardless of map iteration
	// order. This used to be flaky (a generated iso could overwrite the input
	// one depending on which key got visited last).
	t.Run("pre-existing *_iso wins over generated when both keys coexist", func(t *testing.T) {
		t.Parallel()
		const preserved = "PRESERVED-ISO-VALUE"
		// Run several times to make a map-iteration-order race surface
		// quickly if the guard regresses.
		for i := 0; i < 50; i++ {
			items := []interface{}{
				map[string]interface{}{
					"open_time":     int64(1745193600),
					"open_time_iso": preserved,
				},
			}
			row := addDriveSearchIsoTimeFields(items)[0].(map[string]interface{})
			if row["open_time_iso"] != preserved {
				t.Fatalf("attempt %d: open_time_iso = %v, want %q (pre-existing must win)",
					i, row["open_time_iso"], preserved)
			}
		}
	})
}

func TestEnrichDriveSearchError(t *testing.T) {
	t.Parallel()

	t.Run("non-ExitError passes through", func(t *testing.T) {
		t.Parallel()
		orig := errors.New("plain error")
		if got := enrichDriveSearchError(orig); got != orig {
			t.Fatalf("plain error should pass through unchanged")
		}
	})

	t.Run("ExitError without Detail passes through", func(t *testing.T) {
		t.Parallel()
		orig := &output.ExitError{Code: 1}
		if got := enrichDriveSearchError(orig); got != orig {
			t.Fatalf("ExitError without Detail should pass through unchanged")
		}
	})

	t.Run("ExitError with non-matching code passes through", func(t *testing.T) {
		t.Parallel()
		orig := &output.ExitError{
			Code:   1,
			Detail: &output.ErrDetail{Code: 12345, Message: "other"},
		}
		if got := enrichDriveSearchError(orig); got != orig {
			t.Fatalf("non-matching code should pass through unchanged")
		}
	})

	t.Run("matching code rewrites Hint without mutating original", func(t *testing.T) {
		t.Parallel()
		orig := &output.ExitError{
			Code: 1,
			Detail: &output.ErrDetail{
				Code:    driveSearchErrUserNotVisible,
				Message: "[99992351] user not visible",
				Hint:    "",
			},
		}
		enriched := enrichDriveSearchError(orig)
		eErr, ok := enriched.(*output.ExitError)
		if !ok {
			t.Fatalf("expected *output.ExitError, got %T", enriched)
		}
		if eErr == orig {
			t.Fatal("should return a new ExitError, not mutate the original")
		}
		if orig.Detail.Hint != "" {
			t.Fatal("original Detail.Hint must remain unchanged")
		}
		if !strings.Contains(eErr.Detail.Hint, "--creator-ids") {
			t.Fatalf("hint should mention --creator-ids, got %q", eErr.Detail.Hint)
		}
		if eErr.Detail.Message != orig.Detail.Message {
			t.Fatalf("Message should be preserved, got %q", eErr.Detail.Message)
		}
	})
}

func TestCloneDriveSearchFilter(t *testing.T) {
	t.Parallel()
	src := map[string]interface{}{"a": 1, "b": "x"}
	dst := cloneDriveSearchFilter(src)
	if !reflect.DeepEqual(src, dst) {
		t.Fatalf("clone should equal source")
	}
	dst["a"] = 99
	if src["a"] != 1 {
		t.Fatalf("mutating clone should not affect source")
	}
}

func TestBuildDriveSearchRequest(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 4, 24, 16, 0, 0, 0, time.UTC)
	const userOpenID = "ou_self"

	t.Run("empty spec emits both filters as empty maps", func(t *testing.T) {
		t.Parallel()
		req, notices, err := buildDriveSearchRequest(driveSearchSpec{}, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(notices) != 0 {
			t.Fatalf("expected no notices, got %v", notices)
		}
		if _, ok := req["doc_filter"].(map[string]interface{}); !ok {
			t.Fatalf("doc_filter missing")
		}
		if _, ok := req["wiki_filter"].(map[string]interface{}); !ok {
			t.Fatalf("wiki_filter missing")
		}
		if req["page_size"] != 15 {
			t.Fatalf("default page_size should be 15, got %v", req["page_size"])
		}
	})

	t.Run("--mine fills creator_ids from userOpenID", func(t *testing.T) {
		t.Parallel()
		req, _, err := buildDriveSearchRequest(driveSearchSpec{Mine: true}, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		got := req["doc_filter"].(map[string]interface{})["creator_ids"].([]string)
		if len(got) != 1 || got[0] != userOpenID {
			t.Fatalf("expected [userOpenID], got %v", got)
		}
	})

	t.Run("--mine without userOpenID errors", func(t *testing.T) {
		t.Parallel()
		_, _, err := buildDriveSearchRequest(driveSearchSpec{Mine: true}, "", now)
		if err == nil || !strings.Contains(err.Error(), "--mine") {
			t.Fatalf("expected --mine error, got: %v", err)
		}
	})

	t.Run("--mine + --creator-ids mutually exclusive", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{Mine: true, CreatorIDs: []string{"ou_x"}}
		_, _, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err == nil || !strings.Contains(err.Error(), "--mine") {
			t.Fatalf("expected exclusion error, got: %v", err)
		}
	})

	t.Run("--folder-tokens + --space-ids mutually exclusive", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{
			FolderTokens: []string{"fld_a"},
			SpaceIDs:     []string{"sp_b"},
		}
		_, _, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err == nil || !strings.Contains(err.Error(), "--folder-tokens") {
			t.Fatalf("expected exclusion error, got: %v", err)
		}
	})

	t.Run("--folder-tokens scopes only doc_filter", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{FolderTokens: []string{"fld_a"}}
		req, _, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if _, ok := req["wiki_filter"]; ok {
			t.Fatalf("wiki_filter should not be set when --folder-tokens is given")
		}
		df := req["doc_filter"].(map[string]interface{})
		if _, ok := df["folder_tokens"]; !ok {
			t.Fatalf("doc_filter must carry folder_tokens")
		}
	})

	t.Run("--space-ids scopes only wiki_filter", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{SpaceIDs: []string{"sp_x"}}
		req, _, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if _, ok := req["doc_filter"]; ok {
			t.Fatalf("doc_filter should not be set when --space-ids is given")
		}
		wf := req["wiki_filter"].(map[string]interface{})
		if _, ok := wf["space_ids"]; !ok {
			t.Fatalf("wiki_filter must carry space_ids")
		}
	})

	t.Run("sort=default maps to DEFAULT_TYPE", func(t *testing.T) {
		t.Parallel()
		req, _, err := buildDriveSearchRequest(driveSearchSpec{Sort: "default"}, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got := req["doc_filter"].(map[string]interface{})["sort_type"]; got != "DEFAULT_TYPE" {
			t.Fatalf("sort_type=%v, want DEFAULT_TYPE", got)
		}
	})

	t.Run("sort=edit_time upper-cases 1:1", func(t *testing.T) {
		t.Parallel()
		req, _, err := buildDriveSearchRequest(driveSearchSpec{Sort: "edit_time"}, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if got := req["doc_filter"].(map[string]interface{})["sort_type"]; got != "EDIT_TIME" {
			t.Fatalf("sort_type=%v, want EDIT_TIME", got)
		}
	})

	t.Run("invalid doc-types surfaces", func(t *testing.T) {
		t.Parallel()
		_, _, err := buildDriveSearchRequest(driveSearchSpec{DocTypes: []string{"PIE"}}, userOpenID, now)
		if err == nil || !strings.Contains(err.Error(), "--doc-types") {
			t.Fatalf("expected --doc-types error, got: %v", err)
		}
	})

	t.Run("opened-since 8m triggers clamp notice", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{
			OpenedSince: time.Unix(now.Unix()-240*86400, 0).UTC().Format(time.RFC3339),
		}
		_, notices, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		joined := strings.Join(notices, "\n")
		if !strings.Contains(joined, "3 slices total") {
			t.Fatalf("expected 3-slice clamp notice, got: %s", joined)
		}
	})

	t.Run("scalar filters land in both doc and wiki filters", func(t *testing.T) {
		t.Parallel()
		spec := driveSearchSpec{
			DocTypes:    []string{"DOCX"},
			ChatIDs:     []string{"oc_a"},
			OnlyTitle:   true,
			OnlyComment: true,
		}
		req, _, err := buildDriveSearchRequest(spec, userOpenID, now)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		df := req["doc_filter"].(map[string]interface{})
		wf := req["wiki_filter"].(map[string]interface{})
		for _, side := range []map[string]interface{}{df, wf} {
			if _, ok := side["doc_types"]; !ok {
				t.Fatal("doc_types missing")
			}
			if _, ok := side["chat_ids"]; !ok {
				t.Fatal("chat_ids missing")
			}
			if side["only_title"] != true {
				t.Fatal("only_title missing")
			}
			if side["only_comment"] != true {
				t.Fatal("only_comment missing")
			}
		}
	})
}

func TestRenderDriveSearchTable(t *testing.T) {
	t.Parallel()

	t.Run("empty items prints fallback message", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		renderDriveSearchTable(&buf, map[string]interface{}{}, nil)
		if !strings.Contains(buf.String(), "No matching results found") {
			t.Fatalf("expected fallback message, got: %s", buf.String())
		}
	})

	t.Run("strips both <h> and <hb> highlight tags", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		items := []interface{}{
			map[string]interface{}{
				"title_highlighted": "<h>hi</h> there <hb>bold</hb>!",
				"entity_type":       "DOC",
				"result_meta":       map[string]interface{}{"url": "https://example.com/x"},
			},
		}
		renderDriveSearchTable(&buf, map[string]interface{}{}, items)
		out := buf.String()
		if strings.Contains(out, "<h>") || strings.Contains(out, "<hb>") || strings.Contains(out, "</h>") || strings.Contains(out, "</hb>") {
			t.Fatalf("highlight tags leaked: %s", out)
		}
		if !strings.Contains(out, "hi there bold!") {
			t.Fatalf("plain text should remain after stripping, got: %s", out)
		}
	})

	t.Run("falls back to title when title_highlighted is missing", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		items := []interface{}{
			map[string]interface{}{
				"title":       "plain title",
				"entity_type": "DOC",
				"result_meta": map[string]interface{}{
					"url":             "https://example.com/x",
					"update_time_iso": "2026-04-01T00:00:00Z",
					"doc_types":       "DOC",
				},
			},
		}
		renderDriveSearchTable(&buf, map[string]interface{}{}, items)
		out := buf.String()
		if !strings.Contains(out, "plain title") {
			t.Fatalf("expected fallback title, got: %s", out)
		}
		if strings.Contains(out, "<nil>") {
			t.Fatalf("title fallback should not produce <nil>, got: %s", out)
		}
	})

	// Regression: when result_meta is missing url / update_time_iso (or
	// result_meta itself is absent), the table must render empty cells, not
	// the literal string "<nil>". This used to leak via fmt.Sprintf("%v",
	// nil) before the type-assertion guard was added.
	t.Run("missing url and update_time_iso render as empty, not <nil>", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		items := []interface{}{
			// minimal item: title only, no result_meta keys at all
			map[string]interface{}{
				"title_highlighted": "row1",
				"entity_type":       "DOC",
				"result_meta":       map[string]interface{}{},
			},
			// item with no result_meta at all
			map[string]interface{}{
				"title_highlighted": "row2",
				"entity_type":       "DOC",
			},
		}
		renderDriveSearchTable(&buf, map[string]interface{}{}, items)
		out := buf.String()
		if strings.Contains(out, "<nil>") {
			t.Fatalf("table must not render <nil> for missing url/edit_time, got:\n%s", out)
		}
	})

	t.Run("appends has_more hint when there are more pages", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		items := []interface{}{
			map[string]interface{}{
				"title":       "x",
				"entity_type": "DOC",
				"result_meta": map[string]interface{}{"url": "https://example.com/x"},
			},
		}
		renderDriveSearchTable(&buf, map[string]interface{}{"has_more": true}, items)
		if !strings.Contains(buf.String(), "more available") {
			t.Fatalf("expected has_more hint, got: %s", buf.String())
		}
	})
}
