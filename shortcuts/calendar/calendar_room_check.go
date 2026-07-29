// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +update room-availability pre-check helpers.
//
// Uses /open-apis/calendar/v4/freebusy/room_availability_check to warn the
// caller before an update either adds a new room attendee or shifts the time
// of a slot that already has a room reservation. --skip-room-check bypasses
// the check for callers that want to move fast.

package calendar

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	flagSkipRoomCheck = "skip-room-check"
	roomCheckPath     = "/open-apis/calendar/v4/freebusy/room_availability_check"
)

// roomAvailability mirrors a single room result from the API.
type roomAvailability struct {
	RoomID                string            `json:"room_id,omitempty"`
	RoomName              string            `json:"room_name,omitempty"`
	Status                string            `json:"status,omitempty"`
	UnavailableReasonType string            `json:"unavailable_reason_type,omitempty"`
	Strategy              *roomStrategy     `json:"room_strategy,omitempty"`
	Requisition           *roomRequisition  `json:"room_requisition,omitempty"`
	ApprovalInfo          *roomApprovalInfo `json:"room_approval_info,omitempty"`
}

// roomStrategy mirrors the room_strategy block returned by the API on
// unavailable rooms. Every field is optional: the server only fills in the
// entries relevant to the current unavailable_reason_type.
type roomStrategy struct {
	SingleMaxDuration             string `json:"single_max_duration,omitempty"`
	MaxAdvanceBookingTime         string `json:"max_advance_booking_time,omitempty"`
	DailyStartTime                string `json:"daily_start_time,omitempty"`
	DailyEndTime                  string `json:"daily_end_time,omitempty"`
	Timezone                      string `json:"timezone,omitempty"`
	DailyAdvanceWindowReleaseTime string `json:"daily_advance_window_release_time,omitempty"`
}

// roomRequisition mirrors room_requisition, returned by the API only when
// unavailable_reason_type == "during_requisition". Both fields are RFC3339
// strings and either may be empty if the server has no exact bound.
type roomRequisition struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
}

// roomApprovalInfo mirrors room_approval_info, returned when the room requires
// (or may require) an approval submission before it can be booked.
//
//   - ApprovalMode: "none" (no approval), "over_duration" (only when the
//     booking exceeds the threshold), or "all" (every booking needs approval).
//   - ApprovalDurationThreshold: seconds; only meaningful when
//     ApprovalMode == "over_duration". The server returns it as a numeric
//     string, matching the shape of the other duration fields.
//
// When the pre-check returns status == "need_approval" the caller renders a
// friendly reminder derived from these two fields plus the current event
// duration, so the agent knows whether to switch rooms/times or route the
// user through an approval flow.
type roomApprovalInfo struct {
	ApprovalMode              string `json:"approval_mode,omitempty"`
	ApprovalDurationThreshold string `json:"approval_duration_threshold,omitempty"`
}

// eventSnapshot carries only the fields room-check needs from the current
// event: existing room IDs, current start/end (unix seconds string), timezone,
// and rrule.
type eventSnapshot struct {
	RoomIDs   []string
	StartTs   string
	EndTs     string
	Timezone  string
	Recurrent string
}

// unavailableReasonHint maps API-declared unavailable reasons to a short
// English phrase suitable for embedding in the block message. Unknown or
// future reasons fall back to a single stable phrase so the CLI's blocked
// message stays predictable for agents that parse it.
func unavailableReasonHint(reason string) string {
	switch reason {
	case "reserved_by_other_event":
		return "already reserved by another event"
	case "past_time":
		return "cannot book a room in the past"
	case "beyond_advance_booking_window":
		return "beyond the room's advance-booking window"
	case "over_max_duration":
		return "exceeds the room's max single-booking duration"
	case "not_in_usable_time":
		return "outside the room's daily bookable window"
	case "during_requisition":
		return "the room is disabled during this time and cannot be booked"
	case "before_daily_advance_window_release":
		return "the target date is outside the room's currently unlocked advance-booking window; the window extends by one calendar day at the daily release time"
	case "recurring_exceed_approval_limit":
		return "recurring event duration exceeds the limit for booking this approval-required room — shorten the duration or pick a different room"
	default:
		return "currently unbookable"
	}
}

// strategyDetail renders the human-readable suffix appended to the reason
// phrase for a given (reason, strategy) pair. It returns an empty string when
// no strategy data is available or when the fields relevant to this reason
// are missing / invalid, so callers can safely concatenate the result.
func strategyDetail(reason string, s *roomStrategy) string {
	if s == nil {
		return ""
	}
	switch reason {
	case "over_max_duration":
		if d := formatDurationSeconds(s.SingleMaxDuration); d != "" {
			return "the max single-booking duration is " + d
		}
	case "beyond_advance_booking_window":
		// The API returns max_advance_booking_time as RFC3339 already;
		// surface it verbatim so agents don't lose the exact instant.
		if t := strings.TrimSpace(s.MaxAdvanceBookingTime); t != "" {
			return "the latest bookable end time is " + t
		}
	case "not_in_usable_time":
		start := formatDaySeconds(s.DailyStartTime)
		end := formatDaySeconds(s.DailyEndTime)
		zone := roomZoneLabel(s.Timezone)
		switch {
		case start != "" && end != "":
			return fmt.Sprintf("the daily bookable window is %s - %s (%s)", start, end, zone)
		case start != "":
			return fmt.Sprintf("the daily bookable window starts at %s (%s)", start, zone)
		case end != "":
			return fmt.Sprintf("the daily bookable window ends at %s (%s)", end, zone)
		}
	case "before_daily_advance_window_release":
		if t := formatDaySeconds(s.DailyAdvanceWindowReleaseTime); t != "" {
			return fmt.Sprintf("the next unlock happens today at %s (%s), which advances the window by one day", t, roomZoneLabel(s.Timezone))
		}
	}
	return ""
}

// requisitionDetail renders the suffix describing the room's scheduled
// disable window for a `during_requisition` block. The API sends both bounds
// as RFC3339 already, so we surface them verbatim to keep the exact instant.
// Returns "" when both bounds are missing so the caller falls back to the
// generic "pick a different time or a different room" recovery hint.
func requisitionDetail(reason string, r *roomRequisition) string {
	if reason != "during_requisition" || r == nil {
		return ""
	}
	start := strings.TrimSpace(r.StartTime)
	end := strings.TrimSpace(r.EndTime)
	switch {
	case start != "" && end != "":
		return fmt.Sprintf("the disabled period is %s to %s", start, end)
	case start != "":
		return "the disabled period starts at " + start
	case end != "":
		return "the disabled period ends at " + end
	}
	return ""
}

// formatDurationSeconds renders a whole-second string like "10800" as a
// compact "H hours [M minutes]" phrase. Returns "" when the value is
// missing, non-numeric, or non-positive.
func formatDurationSeconds(raw string) string {
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sec <= 0 {
		return ""
	}
	d := time.Duration(sec) * time.Second
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%d hours %d minutes", h, m)
	case h > 0:
		return fmt.Sprintf("%d hours", h)
	case m > 0:
		return fmt.Sprintf("%d minutes", m)
	default:
		return fmt.Sprintf("%d seconds", sec)
	}
}

// formatDaySeconds renders a "seconds since midnight" string as "HH:MM".
// Returns "" when raw is missing, non-numeric, or outside [0, 24h). Seconds
// are truncated because the API only guarantees minute-level meaning for
// daily windows and release times.
func formatDaySeconds(raw string) string {
	sec, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || sec < 0 || sec >= 24*3600 {
		return ""
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	return fmt.Sprintf("%02d:%02d", h, m)
}

// roomZoneLabel renders the room's timezone as either a "GMT±X" string
// anchored to today (so DST is respected) when the IANA name resolves, or
// the IANA name itself as a fallback so agents always see the source of
// truth. Returns the local device timezone's label when raw is empty.
func roomZoneLabel(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return gmtOffsetLabel(time.Now())
	}
	loc, err := time.LoadLocation(raw)
	if err != nil {
		return raw
	}
	return gmtOffsetLabel(time.Now().In(loc))
}

// gmtOffsetLabel formats t's zone offset as "GMT+8" / "GMT-5:30" / "GMT".
// Minute-precision is included only when the offset has a non-zero minute
// component so the common whole-hour case stays terse.
func gmtOffsetLabel(t time.Time) string {
	_, offsetSec := t.Zone()
	if offsetSec == 0 {
		return "GMT"
	}
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	h := offsetSec / 3600
	m := (offsetSec % 3600) / 60
	if m == 0 {
		return fmt.Sprintf("GMT%s%d", sign, h)
	}
	return fmt.Sprintf("GMT%s%d:%02d", sign, h, m)
}

// collectAttendeeRoomIDs extracts omm_ prefixed IDs from a comma-separated
// flag value. Empty / whitespace input returns nil.
func collectAttendeeRoomIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var rooms []string
	seen := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		id := strings.TrimSpace(part)
		if !strings.HasPrefix(id, "omm_") {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		rooms = append(rooms, id)
	}
	return rooms
}

// fetchEventSnapshot GETs the event with attendees so we can read the current
// start / end / recurrence and the room IDs already booked on the event. It is
// best-effort: any error bubbles up so the caller can降级放行 by warning.
//
// One retry is baked in: a `{uid}_{original_time}` event_id refers to a
// specific instance of a recurring series, but until that instance is edited
// and materialised as an exception, the server only knows the master
// (`{uid}_0`) and answers 193001 (event not found). We detect that shape and
// re-issue the GET against the master so the room-check pipeline still has a
// snapshot to work with.
func fetchEventSnapshot(_ context.Context, runtime *common.RuntimeContext, calendarID, eventID string) (*eventSnapshot, error) {
	data, err := callEventGet(runtime, calendarID, eventID)
	if err != nil {
		if masterID, ok := recurringMasterEventID(eventID); ok && isEventNotFound(err) {
			data, err = callEventGet(runtime, calendarID, masterID)
		}
		if err != nil {
			return nil, err
		}
	}
	event, _ := data["event"].(map[string]interface{})
	if event == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event response missing 'event' field")
	}
	snap := &eventSnapshot{}
	if start, _ := event["start_time"].(map[string]interface{}); start != nil {
		if ts, _ := start["timestamp"].(string); ts != "" {
			snap.StartTs = ts
		}
		if tz, _ := start["timezone"].(string); tz != "" {
			snap.Timezone = tz
		}
	}
	if end, _ := event["end_time"].(map[string]interface{}); end != nil {
		if ts, _ := end["timestamp"].(string); ts != "" {
			snap.EndTs = ts
		}
		if snap.Timezone == "" {
			if tz, _ := end["timezone"].(string); tz != "" {
				snap.Timezone = tz
			}
		}
	}
	if r, _ := event["recurrence"].(string); r != "" {
		snap.Recurrent = r
	}
	attendees, _ := event["attendees"].([]interface{})
	seen := map[string]struct{}{}
	for _, raw := range attendees {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != "resource" {
			continue
		}
		id, _ := m["room_id"].(string)
		if id == "" {
			continue
		}
		if status, _ := m["rsvp_status"].(string); status == "removed" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		snap.RoomIDs = append(snap.RoomIDs, id)
	}
	return snap, nil
}

// callEventGet issues the calendar event GET used by fetchEventSnapshot. It
// is factored out so the 193001 fallback can re-issue the request against
// the master event without duplicating the params / path plumbing.
func callEventGet(runtime *common.RuntimeContext, calendarID, eventID string) (map[string]interface{}, error) {
	path := fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
		validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID))
	params := map[string]interface{}{
		"user_id_type":     "open_id",
		"need_attendee":    true,
		"max_attendee_num": 20,
	}
	return runtime.CallAPITyped("GET", path, params, nil)
}

// recurringMasterEventID inspects a calendar event_id shaped like
// `{uid}_{original_time}` and returns `{uid}_0` when original_time is a
// positive integer, plus true so callers know a fallback is worth trying.
// Any other shape (missing underscore, non-numeric suffix, already `_0`, or
// suffix `0` / negative) returns "", false so we don't retry pointlessly.
func recurringMasterEventID(eventID string) (string, bool) {
	idx := strings.LastIndex(eventID, "_")
	if idx <= 0 || idx == len(eventID)-1 {
		return "", false
	}
	uid := eventID[:idx]
	suffix := eventID[idx+1:]
	n, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil || n <= 0 {
		return "", false
	}
	return uid + "_0", true
}

// isEventNotFound returns true when err is a calendar 193001 (event not
// found) API error. Kept in this file rather than shared with
// unwrapCalendarAPIError because that helper returns a user-facing hint —
// here we only need the classification, not the copy.
func isEventNotFound(err error) bool {
	if err == nil {
		return false
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		return false
	}
	return ae.Code == 193001
}

// roomCheckPlan bundles the resolved inputs for the pre-check API call.
type roomCheckPlan struct {
	RoomIDs       []string
	StartTs       string
	EndTs         string
	StartTimezone string
	Rrule         string
}

// resolveRoomCheckPlan works out which rooms to check and the target time
// window. It applies the降级放行 policy: if the event snapshot fails to load
// but we can proceed with only user-provided inputs (i.e., time changed and a
// new room is added), the pre-check still runs against those. Otherwise it
// warns and returns (nil, nil) so the caller skips the check.
//
// Returns (nil, nil) when no check is warranted.
func resolveRoomCheckPlan(ctx context.Context, runtime *common.RuntimeContext, calendarID, eventID string, newStartTs, newEndTs string, timeChanged, rruleChanged bool) (*roomCheckPlan, error) {
	newRooms := collectAttendeeRoomIDs(runtime.Str("add-attendee-ids"))
	removeSet := map[string]struct{}{}
	for _, id := range collectAttendeeRoomIDs(runtime.Str("remove-attendee-ids")) {
		removeSet[id] = struct{}{}
	}

	// Fast path: only trigger the check when it can find something to look at.
	// - New room attendees → always check.
	// - Time or rrule change → check existing rooms if any.
	if len(newRooms) == 0 && !timeChanged && !rruleChanged {
		return nil, nil
	}

	newRrule := strings.TrimSpace(runtime.Str("rrule"))

	// If we don't need existing rooms and have both start/end, skip the GET.
	needSnapshot := timeChanged || rruleChanged || !timeChanged && len(newRooms) > 0

	var snap *eventSnapshot
	if needSnapshot {
		var err error
		snap, err = fetchEventSnapshot(ctx, runtime, calendarID, eventID)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut,
				"[calendar +update] warning: failed to fetch current event for room-availability check (%v); precheck runs only against user-supplied inputs — pass --%s to silence\n",
				err, flagSkipRoomCheck)
			snap = nil
		}
	}

	plan := &roomCheckPlan{
		StartTs: newStartTs,
		EndTs:   newEndTs,
		Rrule:   newRrule,
	}
	if plan.StartTs == "" && snap != nil {
		plan.StartTs = snap.StartTs
	}
	if plan.EndTs == "" && snap != nil {
		plan.EndTs = snap.EndTs
	}
	if plan.Rrule == "" && snap != nil {
		plan.Rrule = snap.Recurrent
	}
	if snap != nil {
		plan.StartTimezone = snap.Timezone
	}

	seen := map[string]struct{}{}
	addRoom := func(id string) {
		if id == "" {
			return
		}
		if _, ok := removeSet[id]; ok {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		plan.RoomIDs = append(plan.RoomIDs, id)
	}
	for _, id := range newRooms {
		addRoom(id)
	}
	if snap != nil && (timeChanged || rruleChanged) {
		for _, id := range snap.RoomIDs {
			addRoom(id)
		}
	}

	if len(plan.RoomIDs) == 0 {
		return nil, nil
	}
	// Without a target window the server has no basis to check anything;
	// prefer degrading gracefully to blocking legitimate updates.
	if plan.StartTs == "" || plan.EndTs == "" {
		fmt.Fprintf(runtime.IO().ErrOut,
			"[calendar +update] warning: room-availability check skipped because start/end could not be resolved; pass --%s to silence\n",
			flagSkipRoomCheck)
		return nil, nil
	}
	return plan, nil
}

// roomCheckPlanDurationSec returns the current booking duration in whole
// seconds derived from the resolved plan's Unix-second window, or 0 when
// either bound is missing or unparseable. Used to compare against
// approval_duration_threshold when the API asks for approval.
func roomCheckPlanDurationSec(plan *roomCheckPlan) int64 {
	if plan == nil {
		return 0
	}
	start, err := strconv.ParseInt(strings.TrimSpace(plan.StartTs), 10, 64)
	if err != nil {
		return 0
	}
	end, err := strconv.ParseInt(strings.TrimSpace(plan.EndTs), 10, 64)
	if err != nil {
		return 0
	}
	if end <= start {
		return 0
	}
	return end - start
}

// buildRoomCheckBody assembles the request body for room_availability_check.
// The pre-check API expects start/end as RFC3339 timestamps; we take the
// Unix-second strings used elsewhere in the update flow and render them in
// the event's own timezone when available, falling back to the local device
// timezone so agents on different machines still produce a valid request.
// start_timezone is an IANA name (e.g. "Asia/Shanghai") copied from the event
// snapshot; it is omitted when unknown so the server can fall back to its own
// default.
func buildRoomCheckBody(calendarID, eventID string, plan *roomCheckPlan) map[string]interface{} {
	loc := time.Local
	if plan.StartTimezone != "" {
		if l, err := time.LoadLocation(plan.StartTimezone); err == nil {
			loc = l
		}
	}
	body := map[string]interface{}{
		"calendar_id": calendarID,
		"event_id":    eventID,
		"start_time":  formatRoomCheckTime(plan.StartTs, loc),
		"end_time":    formatRoomCheckTime(plan.EndTs, loc),
		"room_ids":    plan.RoomIDs,
	}
	if plan.StartTimezone != "" {
		body["start_timezone"] = plan.StartTimezone
	}
	if plan.Rrule != "" {
		body["event_rrule"] = plan.Rrule
	}
	return body
}

// formatRoomCheckTime renders a Unix-second string as RFC3339 in loc.
// Non-numeric input is returned unchanged so anomalies stay visible instead
// of being silently rewritten to the epoch.
func formatRoomCheckTime(unixStr string, loc *time.Location) string {
	sec, err := strconv.ParseInt(strings.TrimSpace(unixStr), 10, 64)
	if err != nil {
		return unixStr
	}
	return time.Unix(sec, 0).In(loc).Format(time.RFC3339)
}

// callRoomAvailabilityCheck posts the availability request and returns per-room
// results.
func callRoomAvailabilityCheck(runtime *common.RuntimeContext, body map[string]interface{}) ([]roomAvailability, error) {
	data, err := runtime.CallAPITyped("POST", roomCheckPath, nil, body)
	if err != nil {
		return nil, err
	}
	rawList, _ := data["room_availabilitys"].([]interface{})
	out := make([]roomAvailability, 0, len(rawList))
	for _, raw := range rawList {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		item := roomAvailability{}
		if v, ok := m["room_id"].(string); ok {
			item.RoomID = v
		}
		if v, ok := m["room_name"].(string); ok {
			item.RoomName = v
		}
		if v, ok := m["status"].(string); ok {
			item.Status = v
		}
		if v, ok := m["unavailable_reason_type"].(string); ok {
			item.UnavailableReasonType = v
		}
		if strat, ok := m["room_strategy"].(map[string]interface{}); ok {
			item.Strategy = parseRoomStrategy(strat)
		}
		if req, ok := m["room_requisition"].(map[string]interface{}); ok {
			item.Requisition = parseRoomRequisition(req)
		}
		if info, ok := m["room_approval_info"].(map[string]interface{}); ok {
			item.ApprovalInfo = parseRoomApprovalInfo(info)
		}
		out = append(out, item)
	}
	return out, nil
}

// parseRoomStrategy extracts the optional strategy fields from a raw API
// map. Missing / non-string values are dropped so callers only see what the
// server actually sent.
func parseRoomStrategy(m map[string]interface{}) *roomStrategy {
	s := &roomStrategy{}
	if v, ok := m["single_max_duration"].(string); ok {
		s.SingleMaxDuration = v
	}
	if v, ok := m["max_advance_booking_time"].(string); ok {
		s.MaxAdvanceBookingTime = v
	}
	if v, ok := m["daily_start_time"].(string); ok {
		s.DailyStartTime = v
	}
	if v, ok := m["daily_end_time"].(string); ok {
		s.DailyEndTime = v
	}
	if v, ok := m["timezone"].(string); ok {
		s.Timezone = v
	}
	if v, ok := m["daily_advance_window_release_time"].(string); ok {
		s.DailyAdvanceWindowReleaseTime = v
	}
	return s
}

// parseRoomRequisition extracts the optional room_requisition block from a
// raw API map. Missing / non-string values are dropped.
func parseRoomRequisition(m map[string]interface{}) *roomRequisition {
	r := &roomRequisition{}
	if v, ok := m["start_time"].(string); ok {
		r.StartTime = v
	}
	if v, ok := m["end_time"].(string); ok {
		r.EndTime = v
	}
	return r
}

// parseRoomApprovalInfo extracts the optional room_approval_info block from a
// raw API map. Missing / non-string values are dropped.
func parseRoomApprovalInfo(m map[string]interface{}) *roomApprovalInfo {
	info := &roomApprovalInfo{}
	if v, ok := m["approval_mode"].(string); ok {
		info.ApprovalMode = v
	}
	if v, ok := m["approval_duration_threshold"].(string); ok {
		info.ApprovalDurationThreshold = v
	}
	return info
}

// approvalReasonHint composes the per-line phrase for a `need_approval`
// status. The API returns `room_approval_info` with:
//
//   - "all"           → every reservation on this room must be approved.
//   - "over_duration" → only bookings longer than approval_duration_threshold
//     need approval. The current event duration (eventDurationSec) is compared
//     against the threshold so agents can see exactly why approval is being
//     asked for — and, when the current duration is below the threshold, the
//     message points at the "shorten it" recovery path.
//   - anything else   → generic reminder so unknown modes still surface.
//
// This function only produces the per-room fragment. The shared recovery
// clause (attendees-create, client fallback, shorten, pick another room) is
// appended once by blockOnUnavailableRooms into `.WithHint(...)` so a message
// with several approval-required rooms doesn't repeat the same recovery
// paragraph on every line.
func approvalReasonHint(info *roomApprovalInfo, eventDurationSec int64) string {
	mode := ""
	if info != nil {
		mode = strings.TrimSpace(info.ApprovalMode)
	}
	switch mode {
	case "all":
		return "this room requires approval for every reservation"
	case "over_duration":
		threshold, _ := strconv.ParseInt(strings.TrimSpace(info.ApprovalDurationThreshold), 10, 64)
		if threshold <= 0 {
			// Server said approval-by-duration but didn't give a threshold —
			// keep the mode label so agents don't lose the classification.
			return "this room requires approval when the booking exceeds a duration threshold"
		}
		thresholdPhrase := formatDurationSeconds(info.ApprovalDurationThreshold)
		if thresholdPhrase == "" {
			thresholdPhrase = fmt.Sprintf("%d seconds", threshold)
		}
		base := fmt.Sprintf("this room requires approval when the booking exceeds %s", thresholdPhrase)
		if eventDurationSec > 0 {
			currentPhrase := formatDurationSeconds(strconv.FormatInt(eventDurationSec, 10))
			if currentPhrase == "" {
				currentPhrase = fmt.Sprintf("%d seconds", eventDurationSec)
			}
			if eventDurationSec >= threshold {
				base += fmt.Sprintf(" (current duration is %s)", currentPhrase)
			} else {
				// Server flagged approval but our duration reads as below the
				// threshold — surface both so the agent can reconcile rather
				// than guess.
				base += fmt.Sprintf(" (current duration reads as %s; server still flagged approval)", currentPhrase)
			}
		}
		return base
	default:
		return "this room requires approval before it can be booked"
	}
}

// roomLabel renders the room identifier for the block message. When the API
// returns a human-readable name it becomes `<room_id>[<room_name>]`; a blank
// name (or an entirely blank id, defensive) degrades to whichever is present
// so agents can still address the room. The room_id is kept as the primary
// identifier because callers act on it programmatically. Square brackets are
// used (rather than parentheses) so a room name that itself contains
// parentheses — e.g. "Room A (west wing)" — doesn't produce ambiguous nesting
// like `omm_1(Room A (west wing))`.
func roomLabel(id, name string) string {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	switch {
	case id != "" && name != "":
		return fmt.Sprintf("%s[%s]", id, name)
	case id != "":
		return id
	default:
		return name
	}
}

// blockOnUnavailableRooms returns a typed validation error when any room in
// results is unavailable or requires approval, or nil when everything is
// bookable. The error text carries per-room reasons plus the retry command
// hint from the PRD. When the API returns a room_strategy for a blocked room,
// the relevant limit (max duration, latest bookable time, daily window, or
// daily release time) is appended after the reason so agents can relay it to
// the user without making a follow-up request. For a `during_requisition`
// block, the disabled period (from room_requisition) is appended if available;
// a "pick a different time or a different room" recovery clause is always
// appended so the message reads coherently whether or not exact bounds are
// known.
//
// `need_approval` results are treated as blocking (the CLI cannot submit an
// approval on the user's behalf, so silently PATCHing would surprise the
// user). The line uses room_approval_info + eventDurationSec to explain the
// mode ("all" / "over_duration"), the threshold, and — for over_duration —
// how the current booking compares. The shared "how do I actually recover
// from approval" clause is folded into the hint once (not per line), so
// several approval-required rooms don't repeat the same paragraph.
func blockOnUnavailableRooms(results []roomAvailability, eventDurationSec int64) error {
	var blocked []roomAvailability
	for _, r := range results {
		if r.Status != "available" {
			blocked = append(blocked, r)
		}
	}
	if len(blocked) == 0 {
		return nil
	}
	var lines []string
	hasNeedApproval := false
	for _, r := range blocked {
		var reason string
		switch r.Status {
		case "need_approval":
			hasNeedApproval = true
			reason = approvalReasonHint(r.ApprovalInfo, eventDurationSec)
		default:
			reason = unavailableReasonHint(r.UnavailableReasonType)
		}
		line := fmt.Sprintf("%s: %s", roomLabel(r.RoomID, r.RoomName), reason)
		if detail := strategyDetail(r.UnavailableReasonType, r.Strategy); detail != "" {
			line += ", " + detail
		}
		if detail := requisitionDetail(r.UnavailableReasonType, r.Requisition); detail != "" {
			line += ", " + detail
		}
		if r.UnavailableReasonType == "during_requisition" {
			line += "; pick a different time or a different room"
		}
		lines = append(lines, line)
	}
	msg := "meeting room booking will fail after this event change:\n  " + strings.Join(lines, "\n  ")
	hint := fmt.Sprintf("do NOT auto-retry: relay the room IDs and reasons above to the user and get explicit confirmation before re-running with --%s.",
		flagSkipRoomCheck)
	if hasNeedApproval {
		hint += " Rooms flagged need_approval: the CLI cannot submit approvals; DO NOT auto-run any recovery — ask the user first, then pick one: (a) newly added room → after the user confirms and provides `approval_reason`, run `lark-cli calendar event.attendees create --as user`; (b) time/rrule change re-triggers approval on an existing room → ask the user to update through the client; (c) shorten the meeting below the threshold or pick a different room."
	}
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s", msg).WithHint("%s", hint)
}
