// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Focused unit tests for pure helpers in calendar_update_recurring.go. The
// end-to-end +update flow is exercised elsewhere; these tests cover the small
// deterministic pieces (attendee projection, dedup, key derivation, time
// comparison, pretty output) that would otherwise sit at 0% coverage.

package calendar

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

// -----------------------------------------------------------------------------
// projectInheritedAttendee
// -----------------------------------------------------------------------------

func TestProjectInheritedAttendee(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]interface{}
		want map[string]interface{}
	}{
		{"user ok", map[string]interface{}{"type": "user", "user_id": "u1"},
			map[string]interface{}{"type": "user", "user_id": "u1"}},
		{"user missing id → nil", map[string]interface{}{"type": "user"}, nil},
		{"resource ok", map[string]interface{}{"type": "resource", "room_id": "r1"},
			map[string]interface{}{"type": "resource", "room_id": "r1"}},
		{"resource with customization carried", map[string]interface{}{
			"type": "resource", "room_id": "r1",
			"resource_customization": []interface{}{"opt1"},
		}, map[string]interface{}{
			"type": "resource", "room_id": "r1",
			"resource_customization": []interface{}{"opt1"},
		}},
		{"resource missing room_id → nil", map[string]interface{}{"type": "resource"}, nil},
		{"chat ok", map[string]interface{}{"type": "chat", "chat_id": "oc"},
			map[string]interface{}{"type": "chat", "chat_id": "oc"}},
		{"chat missing id → nil", map[string]interface{}{"type": "chat"}, nil},
		{"third_party ok", map[string]interface{}{"type": "third_party", "third_party_email": "x@y"},
			map[string]interface{}{"type": "third_party", "third_party_email": "x@y"}},
		{"third_party missing email → nil", map[string]interface{}{"type": "third_party"}, nil},
		{"unknown type → nil", map[string]interface{}{"type": "alien", "user_id": "u1"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := projectInheritedAttendee(c.in)
			if !mapsShallowEqual(got, c.want) {
				t.Errorf("projectInheritedAttendee(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// filterOutAttendees / attendeeMatchesRemoval
// -----------------------------------------------------------------------------

func TestFilterOutAttendees(t *testing.T) {
	list := []map[string]interface{}{
		{"type": "user", "user_id": "u1"},
		{"type": "user", "user_id": "u2"},
		{"type": "chat", "chat_id": "oc1"},
		{"type": "resource", "room_id": "r1"},
	}
	t.Run("empty removeIDs → list unchanged", func(t *testing.T) {
		got := filterOutAttendees(list, map[string]struct{}{})
		if len(got) != len(list) {
			t.Errorf("len=%d, want %d", len(got), len(list))
		}
	})
	t.Run("drops matches across id fields", func(t *testing.T) {
		remove := map[string]struct{}{"u1": {}, "r1": {}, "oc1": {}}
		got := filterOutAttendees(list, remove)
		if len(got) != 1 || got[0]["user_id"] != "u2" {
			t.Errorf("got %+v, want only u2 to survive", got)
		}
	})
	t.Run("no match → list unchanged", func(t *testing.T) {
		got := filterOutAttendees(list, map[string]struct{}{"nobody": {}})
		if len(got) != len(list) {
			t.Errorf("len=%d, want %d", len(got), len(list))
		}
	})
}

func TestAttendeeMatchesRemoval(t *testing.T) {
	remove := map[string]struct{}{"u1": {}}
	if !attendeeMatchesRemoval(map[string]interface{}{"user_id": "u1"}, remove) {
		t.Error("user_id match should return true")
	}
	if attendeeMatchesRemoval(map[string]interface{}{"user_id": "u2"}, remove) {
		t.Error("non-match should return false")
	}
	if attendeeMatchesRemoval(map[string]interface{}{}, remove) {
		t.Error("no id fields → false")
	}
}

// -----------------------------------------------------------------------------
// attendeeKey / mergeAttendees
// -----------------------------------------------------------------------------

func TestAttendeeKey(t *testing.T) {
	cases := []struct {
		in   map[string]interface{}
		want string
	}{
		{map[string]interface{}{"type": "user", "user_id": "u1"}, "user:u1"},
		{map[string]interface{}{"type": "chat", "chat_id": "oc1"}, "chat:oc1"},
		{map[string]interface{}{"type": "resource", "room_id": "r1"}, "resource:r1"},
		{map[string]interface{}{"type": "user"}, ""},
		{map[string]interface{}{}, ""},
	}
	for _, c := range cases {
		got := attendeeKey(c.in)
		if got != c.want {
			t.Errorf("attendeeKey(%v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestMergeAttendees_DedupsAndPreservesInherited(t *testing.T) {
	inherited := []map[string]interface{}{
		{"type": "user", "user_id": "u1"},
	}
	added := []map[string]string{
		{"type": "user", "user_id": "u1"},  // duplicate → dropped
		{"type": "user", "user_id": "u2"},  // new → kept
		{"type": "chat", "chat_id": "oc1"}, // new → kept
		{"type": "user"},                   // no id → dropped
	}
	got := mergeAttendees(inherited, added)
	if len(got) != 3 {
		t.Fatalf("want 3 attendees, got %d: %+v", len(got), got)
	}
	if got[0]["user_id"] != "u1" {
		t.Errorf("first entry should be preserved inherited u1, got %+v", got[0])
	}
	// second and third are the added u2 and oc1, in order.
	if got[1]["user_id"] != "u2" || got[2]["chat_id"] != "oc1" {
		t.Errorf("unexpected merged order: %+v", got)
	}
}

// -----------------------------------------------------------------------------
// firstNonEmpty
// -----------------------------------------------------------------------------

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "a", "b"); got != "a" {
		t.Errorf("got %q want %q", got, "a")
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q want empty", got)
	}
	if got := firstNonEmpty("x"); got != "x" {
		t.Errorf("got %q want %q", got, "x")
	}
}

// -----------------------------------------------------------------------------
// parseDateOnly
// -----------------------------------------------------------------------------

func TestParseDateOnly(t *testing.T) {
	if s, ok := parseDateOnly("2026-03-21", time.UTC); !ok || s != "2026-03-21" {
		t.Errorf("got (%q,%v), want (2026-03-21,true)", s, ok)
	}
	if _, ok := parseDateOnly("2026-03-21T09:00:00", time.UTC); ok {
		t.Error("datetime input should not parse as bare date")
	}
	if _, ok := parseDateOnly("not-a-date", time.UTC); ok {
		t.Error("garbage input should not parse")
	}
	if _, ok := parseDateOnly("", time.UTC); ok {
		t.Error("empty input should not parse")
	}
}

// -----------------------------------------------------------------------------
// masterTimeChanged
// -----------------------------------------------------------------------------

// timeFlagsRuntime returns a runtime whose Cmd carries string --start / --end
// flags. Passing "" leaves the flag unchanged (as if the user omitted it).
func timeFlagsRuntime(t *testing.T, start, end string) *common.RuntimeContext {
	t.Helper()
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("start", "", "")
	cmd.Flags().String("end", "", "")
	if start != "" {
		if err := cmd.Flags().Set("start", start); err != nil {
			t.Fatalf("set --start: %v", err)
		}
	}
	if end != "" {
		if err := cmd.Flags().Set("end", end); err != nil {
			t.Fatalf("set --end: %v", err)
		}
	}
	return common.TestNewRuntimeContextForAPI(context.Background(), cmd, defaultConfig(), f, core.AsBot)
}

func TestMasterTimeChanged_FlagsUnset_ReturnsFalse(t *testing.T) {
	rt := timeFlagsRuntime(t, "", "")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Timestamp: "100"},
		EndTime:   &calendarEventTime{Timestamp: "200"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Error("expected changed=false when neither flag was set")
	}
}

func TestMasterTimeChanged_NilMaster_ReturnsTrue(t *testing.T) {
	rt := timeFlagsRuntime(t, "1700000000", "1700003600")
	changed, err := masterTimeChanged(rt, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Error("nil master with both flags set should be treated as changed")
	}
}

func TestMasterTimeChanged_TimedEcho_ReturnsFalse(t *testing.T) {
	rt := timeFlagsRuntime(t, "1700000000", "1700003600")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Timestamp: "1700000000"},
		EndTime:   &calendarEventTime{Timestamp: "1700003600"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Error("echoing master's timestamp should not be considered a change")
	}
}

func TestMasterTimeChanged_TimedDifferent_ReturnsTrue(t *testing.T) {
	rt := timeFlagsRuntime(t, "1700000000", "1700007200")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Timestamp: "1700000000"},
		EndTime:   &calendarEventTime{Timestamp: "1700003600"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Error("different end should be flagged as changed")
	}
}

func TestMasterTimeChanged_AllDayEcho_ReturnsFalse(t *testing.T) {
	rt := timeFlagsRuntime(t, "2026-03-21", "2026-03-22")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Date: "2026-03-21"},
		EndTime:   &calendarEventTime{Date: "2026-03-22"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if changed {
		t.Error("echoing all-day dates should not be treated as changed")
	}
}

func TestMasterTimeChanged_AllDayDifferent_ReturnsTrue(t *testing.T) {
	rt := timeFlagsRuntime(t, "2026-03-22", "2026-03-23")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Date: "2026-03-21"},
		EndTime:   &calendarEventTime{Date: "2026-03-22"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Error("different all-day date should be flagged as changed")
	}
}

func TestMasterTimeChanged_AllDayWithTimedInput_ReturnsTrue(t *testing.T) {
	// A timestamped input against an all-day master inherently drops all-day
	// semantics — must be treated as a change.
	rt := timeFlagsRuntime(t, "2026-03-21T09:00:00Z", "2026-03-21T10:00:00Z")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Date: "2026-03-21"},
		EndTime:   &calendarEventTime{Date: "2026-03-22"},
	}
	changed, err := masterTimeChanged(rt, master)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !changed {
		t.Error("timed input against all-day master should be flagged as changed")
	}
}

func TestMasterTimeChanged_InvalidStart_ReturnsError(t *testing.T) {
	rt := timeFlagsRuntime(t, "not-a-time", "1700003600")
	master := &calendarEvent{
		StartTime: &calendarEventTime{Timestamp: "1700000000"},
		EndTime:   &calendarEventTime{Timestamp: "1700003600"},
	}
	if _, err := masterTimeChanged(rt, master); err == nil {
		t.Error("expected error for unparseable --start")
	}
}

// -----------------------------------------------------------------------------
// resolveFollowingSeriesTimes
// -----------------------------------------------------------------------------

func TestResolveFollowingSeriesTimes_UsesFlagsWhenChanged(t *testing.T) {
	rt := timeFlagsRuntime(t, "1700000000", "1700003600")
	startTs, endTs, err := resolveFollowingSeriesTimes(rt, nil, 1700000000)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if startTs != "1700000000" || endTs != "1700003600" {
		t.Errorf("got (%s,%s), want flag values", startTs, endTs)
	}
}

func TestResolveFollowingSeriesTimes_FallsBackToPivot(t *testing.T) {
	rt := timeFlagsRuntime(t, "", "")
	pivot := &calendarEvent{
		StartTime: &calendarEventTime{Timestamp: "1700100000"},
		EndTime:   &calendarEventTime{Timestamp: "1700103600"},
	}
	startTs, endTs, err := resolveFollowingSeriesTimes(rt, pivot, 1700100000)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if startTs != "1700100000" || endTs != "1700103600" {
		t.Errorf("got (%s,%s), want pivot times", startTs, endTs)
	}
}

func TestResolveFollowingSeriesTimes_LastResortDefaultsToPivotPlusHour(t *testing.T) {
	rt := timeFlagsRuntime(t, "", "")
	// pivot has empty timestamps → last-resort branch.
	startTs, endTs, err := resolveFollowingSeriesTimes(rt, nil, 1700200000)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if startTs != "1700200000" || endTs != "1700203600" {
		t.Errorf("got (%s,%s), want (1700200000,1700203600)", startTs, endTs)
	}
}

func TestResolveFollowingSeriesTimes_InvalidStartInput_ReturnsError(t *testing.T) {
	rt := timeFlagsRuntime(t, "not-a-time", "1700003600")
	if _, _, err := resolveFollowingSeriesTimes(rt, nil, 0); err == nil {
		t.Error("expected error for unparseable --start")
	}
}

// -----------------------------------------------------------------------------
// writeUpdatePretty
// -----------------------------------------------------------------------------

func TestWriteUpdatePretty_RendersHeaderEventAndExceptions(t *testing.T) {
	var buf bytes.Buffer
	result := map[string]interface{}{
		"updated_event": map[string]interface{}{
			"event_id": "evt_1",
			"action":   "patched",
		},
		"exceptions": map[string]interface{}{
			"total":   float64(2),
			"patched": float64(2),
		},
	}
	writeUpdatePretty(&buf, result, "evt_1", "all", "updated_event",
		"Updated event", "Exceptions patched")
	out := buf.String()
	for _, want := range []string{
		"Applied `all` on evt_1",
		"Updated event:",
		"Exceptions patched:",
		"Event updated successfully",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestWriteUpdatePretty_SkipsMissingExceptionsSection(t *testing.T) {
	var buf bytes.Buffer
	result := map[string]interface{}{
		"updated_event": map[string]interface{}{"event_id": "evt_1"},
	}
	writeUpdatePretty(&buf, result, "evt_1", "all", "updated_event",
		"Updated event", "Exceptions patched")
	out := buf.String()
	if strings.Contains(out, "Exceptions patched:") {
		t.Errorf("should not render exceptions section when absent, got:\n%s", out)
	}
	if !strings.Contains(out, "Event updated successfully") {
		t.Errorf("footer missing:\n%s", out)
	}
}

// -----------------------------------------------------------------------------
// writeDeletePretty
// -----------------------------------------------------------------------------

func TestWriteDeletePretty_RendersHeaderEventAndExceptions(t *testing.T) {
	var buf bytes.Buffer
	result := map[string]interface{}{
		"deleted_event": map[string]interface{}{
			"event_id": "evt_1",
			"action":   "deleted",
		},
		"exceptions": map[string]interface{}{
			"total":   float64(1),
			"deleted": float64(1),
		},
	}
	writeDeletePretty(&buf, result, "evt_1", "all", "deleted_event", "Deleted event")
	out := buf.String()
	for _, want := range []string{
		"Applied `all` on evt_1",
		"Deleted event:",
		"Exceptions removed:",
		"Event deleted successfully",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

func mapsShallowEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		// Two nil interface values compare equal; a slice compare below covers
		// the resource_customization case (single-level).
		as, aOK := va.([]interface{})
		bs, bOK := vb.([]interface{})
		if aOK && bOK {
			if len(as) != len(bs) {
				return false
			}
			for i := range as {
				if as[i] != bs[i] {
					return false
				}
			}
			continue
		}
		if va != vb {
			return false
		}
	}
	return true
}
