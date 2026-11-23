// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	freebusyBatchPath = "/open-apis/calendar/v4/freebusy/batch"

	flagFreebusyUserID      = "user-id"
	flagFreebusyType        = "type"
	flagFreebusyMinDuration = "min-duration"

	freebusyTypeBusy       = "busy"
	freebusyTypeRawBusy    = "raw_busy"
	freebusyTypeFree       = "free"
	freebusyTypeCommonFree = "common_free"
)

// freebusyInterval is a merged busy period for one user (busy view).
type freebusyInterval struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
}

// freebusyRawItem is an un-merged upstream busy item carrying rsvp_status
// (raw_busy view). Same time slice may repeat when the user has multiple
// overlapping events.
type freebusyRawItem struct {
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	RSVPStatus string `json:"rsvp_status,omitempty"`
}

// freebusyFreeSlot is a free window with its duration.
type freebusyFreeSlot struct {
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Duration  string `json:"duration"`
}

// freebusyUserBusy is the busy view payload for one user.
type freebusyUserBusy struct {
	UserID string              `json:"user_id"`
	Busy   []*freebusyInterval `json:"busy"`
}

// freebusyUserRawBusy is the raw_busy view payload for one user.
type freebusyUserRawBusy struct {
	UserID  string             `json:"user_id"`
	RawBusy []*freebusyRawItem `json:"raw_busy"`
}

// freebusyUserFree is the free view payload for one user.
type freebusyUserFree struct {
	UserID string              `json:"user_id"`
	Free   []*freebusyFreeSlot `json:"free"`
}

// parseFreebusyTimeRange parses --start / --end into
// (timeMinRFC3339, timeMaxRFC3339, startUnix, endUnix, error).
func parseFreebusyTimeRange(runtime *common.RuntimeContext) (string, string, int64, int64, error) {
	startInput, endInput := resolveStartEnd(runtime)

	startTs, err := common.ParseTime(startInput)
	if err != nil {
		return "", "", 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
	}
	endTs, err := common.ParseTime(endInput, "end")
	if err != nil {
		return "", "", 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
	}

	startSec, err := strconv.ParseInt(startTs, 10, 64)
	if err != nil {
		return "", "", 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid start timestamp: %v", err)
	}
	endSec, err := strconv.ParseInt(endTs, 10, 64)
	if err != nil {
		return "", "", 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid end timestamp: %v", err)
	}
	if endSec <= startSec {
		return "", "", 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--end must be after --start").WithParam("--end")
	}
	timeMin := time.Unix(startSec, 0).Format(time.RFC3339)
	timeMax := time.Unix(endSec, 0).Format(time.RFC3339)
	return timeMin, timeMax, startSec, endSec, nil
}

// collectFreebusyUserIDs resolves --user-id (repeatable / CSV) into a
// deduplicated list. When empty and identity is user, defaults to current
// user; bot identity must provide at least one.
func collectFreebusyUserIDs(runtime *common.RuntimeContext) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, raw := range runtime.StrSlice(flagFreebusyUserID) {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if _, err := common.ValidateUserIDTyped("--user-id", id); err != nil {
			return nil, err
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		if runtime.IsBot() {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--user-id is required for bot identity").WithParam("--user-id")
		}
		me := runtime.UserOpenId()
		if me == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"cannot determine user ID, specify --user-id or ensure you are logged in").WithParam("--user-id")
		}
		out = append(out, me)
	}
	return out, nil
}

func parseFreebusyType(runtime *common.RuntimeContext) (string, error) {
	v := strings.TrimSpace(runtime.Str(flagFreebusyType))
	if v == "" {
		return freebusyTypeBusy, nil
	}
	switch v {
	case freebusyTypeBusy, freebusyTypeRawBusy, freebusyTypeFree, freebusyTypeCommonFree:
		return v, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--type %q: expected one of %s | %s | %s | %s", v,
			freebusyTypeBusy, freebusyTypeRawBusy, freebusyTypeFree, freebusyTypeCommonFree).
			WithParam("--type")
	}
}

func parseFreebusyMinDuration(runtime *common.RuntimeContext) (time.Duration, error) {
	raw := strings.TrimSpace(runtime.Str(flagFreebusyMinDuration))
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--min-duration %q: %v (expected Go duration like 30m or 1h)", raw, err).
			WithParam("--min-duration")
	}
	if d < 0 {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--min-duration %s: must be non-negative", d).WithParam("--min-duration")
	}
	return d, nil
}

// mergeBusyIntervals sorts by start and merges overlapping / adjacent
// intervals. Adjacent means end == next.start. Merging is lossless for the
// busy/free question: it does not change "is this instant busy for this user".
// todo hg 逻辑cr
func mergeBusyIntervals(items []*freebusyInterval) []*freebusyInterval {
	if len(items) == 0 {
		return nil
	}
	type parsed struct {
		start time.Time
		end   time.Time
	}
	arr := make([]parsed, 0, len(items))
	for _, it := range items {
		s, err := time.Parse(time.RFC3339, it.StartTime)
		if err != nil {
			continue
		}
		e, err := time.Parse(time.RFC3339, it.EndTime)
		if err != nil {
			continue
		}
		if !e.After(s) {
			continue
		}
		arr = append(arr, parsed{s, e})
	}
	if len(arr) == 0 {
		return nil
	}
	sort.SliceStable(arr, func(i, j int) bool {
		if !arr[i].start.Equal(arr[j].start) {
			return arr[i].start.Before(arr[j].start)
		}
		return arr[i].end.Before(arr[j].end)
	})
	loc := arr[0].start.Location()
	merged := []parsed{arr[0]}
	for _, cur := range arr[1:] {
		last := &merged[len(merged)-1]
		if !cur.start.After(last.end) {
			if cur.end.After(last.end) {
				last.end = cur.end
			}
			continue
		}
		merged = append(merged, cur)
	}
	out := make([]*freebusyInterval, 0, len(merged))
	for _, m := range merged {
		out = append(out, &freebusyInterval{
			StartTime: m.start.In(loc).Format(time.RFC3339),
			EndTime:   m.end.In(loc).Format(time.RFC3339),
		})
	}
	return out
}

// clipInterval intersects [s, e] with [winStart, winEnd] and reports whether
// the result is non-empty.
func clipInterval(s, e, winStart, winEnd time.Time) (time.Time, time.Time, bool) {
	if s.Before(winStart) {
		s = winStart
	}
	if e.After(winEnd) {
		e = winEnd
	}
	if !e.After(s) {
		return time.Time{}, time.Time{}, false
	}
	return s, e, true
}

// perUserFree computes W \ busy for one user within [winStart, winEnd],
// clipping and filtering by minDur.
// todo hg 逻辑cr
func perUserFree(busy []*freebusyInterval, winStart, winEnd time.Time, minDur time.Duration) []*freebusyFreeSlot {
	loc := winStart.Location()
	if len(busy) > 0 {
		if t, err := time.Parse(time.RFC3339, busy[0].StartTime); err == nil {
			loc = t.Location()
		}
	}
	emit := func(out *[]*freebusyFreeSlot, from, to time.Time) {
		if !to.After(from) {
			return
		}
		d := to.Sub(from)
		if minDur > 0 && d < minDur {
			return
		}
		*out = append(*out, &freebusyFreeSlot{
			StartTime: from.In(loc).Format(time.RFC3339),
			EndTime:   to.In(loc).Format(time.RFC3339),
			Duration:  d.String(),
		})
	}
	cursor := winStart
	var out []*freebusyFreeSlot
	for _, b := range busy {
		s, err := time.Parse(time.RFC3339, b.StartTime)
		if err != nil {
			continue
		}
		e, err := time.Parse(time.RFC3339, b.EndTime)
		if err != nil {
			continue
		}
		cs, ce, ok := clipInterval(s, e, winStart, winEnd)
		if !ok {
			continue
		}
		emit(&out, cursor, cs)
		if ce.After(cursor) {
			cursor = ce
		}
	}
	emit(&out, cursor, winEnd)
	return out
}

// commonFree computes W \ ∪ busy_i via sweep line, clipped to [winStart,
// winEnd] and filtered by minDur.
// todo hg 逻辑cr
func commonFree(usersBusy [][]*freebusyInterval, winStart, winEnd time.Time, minDur time.Duration) []*freebusyFreeSlot {
	type event struct {
		t     time.Time
		delta int
	}
	var evs []event
	loc := winStart.Location()
	for _, busy := range usersBusy {
		for _, b := range busy {
			s, err := time.Parse(time.RFC3339, b.StartTime)
			if err != nil {
				continue
			}
			e, err := time.Parse(time.RFC3339, b.EndTime)
			if err != nil {
				continue
			}
			cs, ce, ok := clipInterval(s, e, winStart, winEnd)
			if !ok {
				continue
			}
			evs = append(evs, event{cs, +1}, event{ce, -1})
		}
	}
	sort.SliceStable(evs, func(i, j int) bool {
		if !evs[i].t.Equal(evs[j].t) {
			return evs[i].t.Before(evs[j].t)
		}
		// same instant: process -1 before +1 so a touching pair of busy
		// intervals never emits a zero-length free slot in the middle.
		return evs[i].delta < evs[j].delta
	})
	emit := func(out *[]*freebusyFreeSlot, from, to time.Time) {
		if !to.After(from) {
			return
		}
		d := to.Sub(from)
		if minDur > 0 && d < minDur {
			return
		}
		*out = append(*out, &freebusyFreeSlot{
			StartTime: from.In(loc).Format(time.RFC3339),
			EndTime:   to.In(loc).Format(time.RFC3339),
			Duration:  d.String(),
		})
	}
	var out []*freebusyFreeSlot
	busy := 0
	freeStart := winStart
	for _, e := range evs {
		if busy == 0 {
			emit(&out, freeStart, e.t)
		}
		busy += e.delta
		if busy == 0 {
			freeStart = e.t
		}
	}
	if busy == 0 {
		emit(&out, freeStart, winEnd)
	}
	return out
}

// parseFreebusyBatchResponse extracts per-user raw busy items from
// /freebusy/batch's `freebusy_lists`, preserving upstream order and each
// item's rsvp_status. Callers that need merged busy intervals feed the
// result into mergeBusyIntervals; raw_busy view uses it directly.
func parseFreebusyBatchResponse(data map[string]interface{}, userOrder []string) map[string][]*freebusyRawItem {
	out := make(map[string][]*freebusyRawItem, len(userOrder))
	for _, u := range userOrder {
		out[u] = nil
	}
	raw, _ := data["freebusy_lists"].([]interface{})
	for _, item := range raw {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		uid, _ := m["user_id"].(string)
		if uid == "" {
			continue
		}
		items, _ := m["freebusy_items"].([]interface{})
		bucket := make([]*freebusyRawItem, 0, len(items))
		for _, it := range items {
			im, ok := it.(map[string]interface{})
			if !ok {
				continue
			}
			startStr, _ := im["start_time"].(string)
			endStr, _ := im["end_time"].(string)
			if startStr == "" || endStr == "" {
				continue
			}
			rsvp, _ := im["rsvp_status"].(string)
			bucket = append(bucket, &freebusyRawItem{
				StartTime:  startStr,
				EndTime:    endStr,
				RSVPStatus: rsvp,
			})
		}
		out[uid] = bucket
	}
	return out
}

// rawToBusyIntervals strips rsvp_status and returns the plain busy list ready
// for mergeBusyIntervals / free-slot computation.
func rawToBusyIntervals(raw []*freebusyRawItem) []*freebusyInterval {
	if len(raw) == 0 {
		return nil
	}
	out := make([]*freebusyInterval, 0, len(raw))
	for _, r := range raw {
		out = append(out, &freebusyInterval{StartTime: r.StartTime, EndTime: r.EndTime})
	}
	return out
}

// sortRawItemsByStart sorts raw items ascending by start_time to give the
// caller a stable ordering; end_time / rsvp_status are otherwise preserved.
func sortRawItemsByStart(items []*freebusyRawItem) []*freebusyRawItem {
	if len(items) <= 1 {
		return items
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].StartTime < items[j].StartTime
	})
	return items
}

func writeFreebusyMinDurationHints(runtime *common.RuntimeContext, typ string, minDur time.Duration, winStart, winEnd time.Time) {
	if minDur == 0 {
		return
	}
	if runtime == nil || runtime.Factory == nil || runtime.Factory.IOStreams == nil {
		return
	}
	stderr := runtime.Factory.IOStreams.ErrOut
	if stderr == nil {
		return
	}
	if typ == freebusyTypeBusy || typ == freebusyTypeRawBusy {
		fmt.Fprintf(stderr, "hint: --min-duration is ignored when --type=%s\n", typ)
	}
	if window := winEnd.Sub(winStart); window > 0 && minDur > window {
		fmt.Fprintf(stderr, "hint: --min-duration %s exceeds the requested window %s; no free slot can satisfy it\n",
			minDur, window)
	}
}

// Off-hours window is [22:00, 24:00) ∪ [00:00, 08:00) in each slot's timezone.
const (
	freebusyEarlyMorningEndHour = 8
	freebusyLateNightStartHour  = 22
)

// slotOverlapsOffHours reports whether the free slot [startStr, endStr]
// (RFC3339) intersects 22:00-24:00 or 00:00-08:00 in the slot's own timezone.
func slotOverlapsOffHours(startStr, endStr string) bool {
	s, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return false
	}
	e, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		return false
	}
	if !e.After(s) {
		return false
	}
	loc := s.Location()
	// Walk each calendar day the slot touches so DST-adjacent days still
	// use the correct 22-24 / 0-8 boundaries.
	day := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, loc)
	for !day.After(e) {
		morningEnd := time.Date(day.Year(), day.Month(), day.Day(),
			freebusyEarlyMorningEndHour, 0, 0, 0, loc)
		if s.Before(morningEnd) && e.After(day) {
			return true
		}
		lateStart := time.Date(day.Year(), day.Month(), day.Day(),
			freebusyLateNightStartHour, 0, 0, 0, loc)
		nextDay := day.AddDate(0, 0, 1)
		if s.Before(nextDay) && e.After(lateStart) {
			return true
		}
		day = nextDay
	}
	return false
}

// writeFreebusyOffHoursHint prints a single stderr hint the first time any of
// the given free slots touches the off-hours window; no-op otherwise.
func writeFreebusyOffHoursHint(runtime *common.RuntimeContext, typ string, slots []*freebusyFreeSlot) {
	if runtime == nil || runtime.Factory == nil || runtime.Factory.IOStreams == nil {
		return
	}
	stderr := runtime.Factory.IOStreams.ErrOut
	if stderr == nil {
		return
	}
	for _, sl := range slots {
		if slotOverlapsOffHours(sl.StartTime, sl.EndTime) {
			fmt.Fprintf(stderr,
				"hint: some %s slots fall into off-hours 22:00-08:00 (may be non-working time); \n", typ)
			return
		}
	}
}

var CalendarFreebusy = common.Shortcut{
	Service:     "calendar",
	Command:     "+freebusy",
	Description: "Query free/busy for one or more users. Merges overlapping busy intervals and supports per-user free / common-free views for multi-user scheduling.",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.free_busy:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "start", Desc: "start time (ISO 8601, default: today)"},
		{Name: "end", Desc: "end time (ISO 8601, default: end of start day)"},
		{Name: flagFreebusyUserID, Type: "string_slice", Desc: "target user open_id(s); repeatable or comma-separated; default: current user; bot identity must provide at least one"},
		{Name: flagFreebusyType, Type: "string", Default: freebusyTypeBusy, Enum: []string{freebusyTypeBusy, freebusyTypeRawBusy, freebusyTypeFree, freebusyTypeCommonFree}, Desc: "output view: busy (default, per-user merged busy) | raw_busy (per-user upstream events with rsvp_status) | free (per-user free windows) | common_free (all-users common free)"},
		{Name: flagFreebusyMinDuration, Type: "string", Desc: "minimum length for free/common_free candidates (Go duration, e.g. 30m, 1h); ignored for busy / raw_busy"},
	},
	Tips: []string{
		"`--type busy` merges adjacent busy intervals, so item count != event count. For events/rsvp use `--type raw_busy` (own or others' calendars); for own attendees/details use `+get` / `+list-attendees`.",
		"Multi-user scheduling raw view: use `--type common_free --min-duration <dur>` and let the CLI compute the intersection instead of merging by hand.",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		timeMin, timeMax, _, _, err := parseFreebusyTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		userIDs, err := collectFreebusyUserIDs(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		typ, err := parseFreebusyType(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		body := map[string]interface{}{
			"time_min":         timeMin,
			"time_max":         timeMax,
			"user_ids":         userIDs,
			"need_rsvp_status": true,
		}
		d := common.NewDryRunAPI().POST(freebusyBatchPath).Body(body).Set("type", typ)
		if raw := strings.TrimSpace(runtime.Str(flagFreebusyMinDuration)); raw != "" {
			d.Set("min_duration", raw)
		}
		return d
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		if _, _, _, _, err := parseFreebusyTimeRange(runtime); err != nil {
			return err
		}
		if _, err := collectFreebusyUserIDs(runtime); err != nil {
			return err
		}
		if _, err := parseFreebusyType(runtime); err != nil {
			return err
		}
		if _, err := parseFreebusyMinDuration(runtime); err != nil {
			return err
		}
		warnCalendarTimezoneMismatch(runtime,
			calendarTimeInputRange{Flag: "start", Value: runtime.Str("start")},
			calendarTimeInputRange{Flag: "end", Value: runtime.Str("end")},
		)
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		timeMin, timeMax, startSec, endSec, err := parseFreebusyTimeRange(runtime)
		if err != nil {
			return err
		}
		userIDs, err := collectFreebusyUserIDs(runtime)
		if err != nil {
			return err
		}
		typ, err := parseFreebusyType(runtime)
		if err != nil {
			return err
		}
		minDur, err := parseFreebusyMinDuration(runtime)
		if err != nil {
			return err
		}

		winStart := time.Unix(startSec, 0)
		winEnd := time.Unix(endSec, 0)
		writeFreebusyMinDurationHints(runtime, typ, minDur, winStart, winEnd)

		data, err := runtime.CallAPITyped("POST", freebusyBatchPath, nil, map[string]interface{}{
			"time_min":         timeMin,
			"time_max":         timeMax,
			"user_ids":         userIDs,
			"need_rsvp_status": true,
		})
		if err != nil {
			return err
		}
		perUser := parseFreebusyBatchResponse(data, userIDs)
		mergedByUser := make(map[string][]*freebusyInterval, len(userIDs))
		for _, u := range userIDs {
			mergedByUser[u] = mergeBusyIntervals(rawToBusyIntervals(perUser[u]))
		}

		switch typ {
		case freebusyTypeBusy:
			users := make([]*freebusyUserBusy, 0, len(userIDs))
			total := 0
			for _, u := range userIDs {
				items := mergedByUser[u]
				if items == nil {
					items = []*freebusyInterval{}
				}
				users = append(users, &freebusyUserBusy{UserID: u, Busy: items})
				total += len(items)
			}
			out := map[string]interface{}{"users": users}
			runtime.OutFormat(out, &output.Meta{Count: total}, func(w io.Writer) {
				if total == 0 {
					fmt.Fprintln(w, "No busy periods in this time range.")
					return
				}
				for _, u := range users {
					fmt.Fprintf(w, "user %s\n", u.UserID)
					if len(u.Busy) == 0 {
						fmt.Fprintln(w, "  (no busy periods)")
						continue
					}
					rows := make([]map[string]interface{}, 0, len(u.Busy))
					for _, it := range u.Busy {
						rows = append(rows, map[string]interface{}{
							"start": it.StartTime,
							"end":   it.EndTime,
						})
					}
					output.PrintTable(w, rows)
				}
				fmt.Fprintf(w, "\n%d busy period(s) across %d user(s)\n", total, len(users))
			})
			return nil

		case freebusyTypeRawBusy:
			users := make([]*freebusyUserRawBusy, 0, len(userIDs))
			total := 0
			for _, u := range userIDs {
				items := sortRawItemsByStart(perUser[u])
				if items == nil {
					items = []*freebusyRawItem{}
				}
				users = append(users, &freebusyUserRawBusy{UserID: u, RawBusy: items})
				total += len(items)
			}
			out := map[string]interface{}{"users": users}
			runtime.OutFormat(out, &output.Meta{Count: total}, func(w io.Writer) {
				if total == 0 {
					fmt.Fprintln(w, "No events in this time range.")
					return
				}
				for _, u := range users {
					fmt.Fprintf(w, "user %s\n", u.UserID)
					if len(u.RawBusy) == 0 {
						fmt.Fprintln(w, "  (no events)")
						continue
					}
					rows := make([]map[string]interface{}, 0, len(u.RawBusy))
					for _, it := range u.RawBusy {
						rows = append(rows, map[string]interface{}{
							"start":       it.StartTime,
							"end":         it.EndTime,
							"rsvp_status": it.RSVPStatus,
						})
					}
					output.PrintTable(w, rows)
				}
				fmt.Fprintf(w, "\n%d event(s) across %d user(s)\n", total, len(users))
			})
			return nil

		case freebusyTypeFree:
			users := make([]*freebusyUserFree, 0, len(userIDs))
			total := 0
			var allFreeForHint []*freebusyFreeSlot
			for _, u := range userIDs {
				free := perUserFree(mergedByUser[u], winStart, winEnd, minDur)
				if free == nil {
					free = []*freebusyFreeSlot{}
				}
				users = append(users, &freebusyUserFree{UserID: u, Free: free})
				total += len(free)
				allFreeForHint = append(allFreeForHint, free...)
			}
			writeFreebusyOffHoursHint(runtime, freebusyTypeFree, allFreeForHint)
			out := map[string]interface{}{"users": users}
			runtime.OutFormat(out, &output.Meta{Count: total}, func(w io.Writer) {
				if total == 0 {
					fmt.Fprintln(w, "No free slots in this time range.")
					return
				}
				for _, u := range users {
					fmt.Fprintf(w, "user %s\n", u.UserID)
					if len(u.Free) == 0 {
						fmt.Fprintln(w, "  (no free slots)")
						continue
					}
					rows := make([]map[string]interface{}, 0, len(u.Free))
					for _, it := range u.Free {
						rows = append(rows, map[string]interface{}{
							"start":    it.StartTime,
							"end":      it.EndTime,
							"duration": it.Duration,
						})
					}
					output.PrintTable(w, rows)
				}
				fmt.Fprintf(w, "\n%d free slot(s) across %d user(s)\n", total, len(users))
			})
			return nil

		case freebusyTypeCommonFree:
			usersBusy := make([][]*freebusyInterval, 0, len(userIDs))
			for _, u := range userIDs {
				usersBusy = append(usersBusy, mergedByUser[u])
			}
			free := commonFree(usersBusy, winStart, winEnd, minDur)
			if free == nil {
				free = []*freebusyFreeSlot{}
			}
			writeFreebusyOffHoursHint(runtime, freebusyTypeCommonFree, free)
			out := map[string]interface{}{"common_free": free}
			runtime.OutFormat(out, &output.Meta{Count: len(free)}, func(w io.Writer) {
				if len(free) == 0 {
					fmt.Fprintln(w, "No common free slots in this time range.")
					return
				}
				rows := make([]map[string]interface{}, 0, len(free))
				for _, it := range free {
					rows = append(rows, map[string]interface{}{
						"start":    it.StartTime,
						"end":      it.EndTime,
						"duration": it.Duration,
					})
				}
				output.PrintTable(w, rows)
				fmt.Fprintf(w, "\n%d common free slot(s) across %d user(s)\n", len(free), len(userIDs))
			})
			return nil
		}
		return nil
	},
}
