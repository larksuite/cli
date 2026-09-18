// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var CalendarUpdate = common.Shortcut{
	Service:     "calendar",
	Command:     "+update",
	Description: "Update a calendar event and incrementally add or remove attendees",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:update"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "event-id", Desc: "event ID to update", Required: true},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "summary", Desc: "event title"},
		{Name: "description", Desc: "event description as Markdown (@file or - for stdin); the unified description field. Supports bold/italic/underline/strikethrough, links, headings (`#`..`###`), blockquotes (`>`), ordered/unordered lists, horizontal rules (`---`), GFM tables, and images (`![name](url)`; a remote URL is used as-is, and a local image path relative to and inside the current working directory is auto-uploaded to Lark drive and rendered inline — absolute/out-of-cwd paths are rejected). A Lark doc URL (bare or as a Markdown link) is auto-resolved to an inline doc-mention chip showing its title. Inside a GFM table cell, stack multiple lines with `<br>`; each line may itself be an ordered/unordered list item, image or styled text (e.g. `1. a<br>2. b`, `- x<br>- y`, `![p](url)<br>**bold**`). Passing an empty string clears the description.", Input: []string{common.File, common.Stdin}},
		{Name: "start", Desc: "new start time (ISO 8601); requires --end"},
		{Name: "end", Desc: "new end time (ISO 8601); requires --start"},
		{Name: "rrule", Desc: "recurrence rule (rfc5545)"},
		{Name: "add-attendee-ids", Desc: "attendee IDs to add, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{Name: "remove-attendee-ids", Desc: "attendee IDs to remove, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{
			Name: flagApplyTo,
			Enum: applyToValues,
			Desc: "recurring scope: single (this occurrence / exception only) | all (whole series and every exception) | this-and-following (truncate the series at this instance and create a new series carrying the requested edits). Required on recurring events; ignored on non-recurring events.",
		},
		{Name: "notify", Type: "bool", Default: "true", Desc: "send update notification to attendees"},
		{Name: flagSkipRoomCheck, Type: "bool", Default: "false", Hidden: true, Desc: "skip meeting-room availability precheck (default checks rooms whenever a new room is added or the time/rrule of a room-attached event changes)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateCalendarUpdate(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return dryRunCalendarUpdate(runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeCalendarUpdate(ctx, runtime)
	},
}

func validateCalendarUpdate(runtime *common.RuntimeContext) error {
	if err := rejectCalendarAutoBotFallback(runtime); err != nil {
		return err
	}
	for _, flag := range []string{"event-id", "summary", "description", "rrule", "calendar-id", "start", "end", "add-attendee-ids", "remove-attendee-ids"} {
		if val := runtime.Str(flag); val != "" {
			if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
				return err
			}
		}
	}

	if strings.TrimSpace(runtime.Str("event-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --event-id").WithParam("--event-id")
	}
	if _, _, err := buildCalendarUpdateEventData(runtime); err != nil {
		return err
	}
	if err := validateCalendarUpdateAttendees(runtime); err != nil {
		return err
	}
	if !hasCalendarUpdateOperation(runtime) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "nothing to update: specify at least one of --summary, --description, --start/--end, --rrule, --add-attendee-ids, or --remove-attendee-ids")
	}
	warnCalendarTimezoneMismatch(runtime,
		calendarTimeInputRange{Flag: "start", Value: runtime.Str("start")},
		calendarTimeInputRange{Flag: "end", Value: runtime.Str("end")},
	)
	return nil
}

func validateCalendarUpdateAttendees(runtime *common.RuntimeContext) error {
	addIDs, err := parseCalendarAttendeeIDs(runtime.Str("add-attendee-ids"))
	if err != nil {
		return withParam(err, "--add-attendee-ids")
	}
	removeIDs, err := parseCalendarAttendeeIDs(runtime.Str("remove-attendee-ids"))
	if err != nil {
		return withParam(err, "--remove-attendee-ids")
	}
	removeSet := make(map[string]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[id] = struct{}{}
	}
	for _, id := range addIDs {
		if _, ok := removeSet[id]; ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "attendee id %q appears in both --add-attendee-ids and --remove-attendee-ids", id)
		}
	}
	return nil
}

func hasCalendarUpdateOperation(runtime *common.RuntimeContext) bool {
	if len(runtime.Str("add-attendee-ids")) > 0 || len(runtime.Str("remove-attendee-ids")) > 0 {
		return true
	}
	body, hasEventFields, err := buildCalendarUpdateEventData(runtime)
	return err == nil && hasEventFields && len(body) > 0
}

func buildCalendarUpdateEventData(runtime *common.RuntimeContext) (map[string]interface{}, bool, error) {
	body := map[string]interface{}{}
	hasFields := false

	if runtime.Cmd.Flags().Changed("summary") {
		body["summary"] = runtime.Str("summary")
		hasFields = true
	}
	if runtime.Cmd.Flags().Changed("description") {
		body["description_rich"] = runtime.Str("description")
		hasFields = true
	}
	if runtime.Cmd.Flags().Changed("rrule") {
		rrule := strings.TrimSpace(runtime.Str("rrule"))
		if rrule != "" {
			body["recurrence"] = rrule
			hasFields = true
		}
	}

	startChanged := runtime.Cmd.Flags().Changed("start")
	endChanged := runtime.Cmd.Flags().Changed("end")
	if startChanged != endChanged {
		return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start and --end must be specified together when updating event time")
	}
	if startChanged {
		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
		}
		s, err := strconv.ParseInt(startTs, 10, 64)
		if err != nil {
			return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid start time: %v", err).WithParam("--start")
		}
		e, err := strconv.ParseInt(endTs, 10, 64)
		if err != nil {
			return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid end time: %v", err).WithParam("--end")
		}
		if e <= s {
			return nil, false, errs.NewValidationError(errs.SubtypeInvalidArgument, "end time must be after start time")
		}
		body["start_time"] = map[string]string{"timestamp": startTs}
		body["end_time"] = map[string]string{"timestamp": endTs}
		hasFields = true
	}

	if hasFields {
		body["need_notification"] = runtime.Bool("notify")
	}
	return body, hasFields, nil
}

func parseCalendarAttendeeIDs(attendeesStr string) ([]string, error) {
	if strings.TrimSpace(attendeesStr) == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var ids []string
	for _, raw := range strings.Split(attendeesStr, ",") {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if !strings.HasPrefix(id, "ou_") && !strings.HasPrefix(id, "oc_") && !strings.HasPrefix(id, "omm_") {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid attendee id format %q: should start with 'ou_', 'oc_', or 'omm_'", id)
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func attendeeDeleteIDs(attendeesStr string) ([]map[string]string, error) {
	ids, err := parseCalendarAttendeeIDs(attendeesStr)
	if err != nil {
		return nil, err
	}
	deleteIDs := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		switch {
		case strings.HasPrefix(id, "oc_"):
			deleteIDs = append(deleteIDs, map[string]string{"type": "chat", "chat_id": id})
		case strings.HasPrefix(id, "omm_"):
			deleteIDs = append(deleteIDs, map[string]string{"type": "resource", "room_id": id})
		case strings.HasPrefix(id, "ou_"):
			deleteIDs = append(deleteIDs, map[string]string{"type": "user", "user_id": id})
		default:
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid attendee id format %q: should start with 'ou_', 'oc_', or 'omm_'", id).WithParam("--remove-attendee-ids")
		}
	}
	return deleteIDs, nil
}

func calendarUpdateIDs(runtime *common.RuntimeContext) (calendarID string, eventID string) {
	calendarID = strings.TrimSpace(runtime.Str("calendar-id"))
	if calendarID == "" {
		calendarID = PrimaryCalendarIDStr
	}
	eventID = strings.TrimSpace(runtime.Str("event-id"))
	return calendarID, eventID
}

func calendarUpdateEventPath(calendarID, eventID string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s", validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID))
}

func calendarUpdateAttendeesPath(calendarID, eventID string) string {
	return calendarUpdateEventPath(calendarID, eventID) + "/attendees"
}

// runRoomAvailabilityPrecheck checks any room affected by this update (new
// room attendees, or existing rooms when the time/rrule shifts) against the
// server before the PATCH is issued. It returns nil to allow the update to
// proceed and a typed error to block it. Called only when --skip-room-check
// is false.
func runRoomAvailabilityPrecheck(ctx context.Context, runtime *common.RuntimeContext, calendarID, eventID string, body map[string]interface{}) error {
	timeChanged := runtime.Cmd.Flags().Changed("start") && runtime.Cmd.Flags().Changed("end")
	rruleChanged := runtime.Cmd.Flags().Changed("rrule")

	var newStartTs, newEndTs string
	if timeChanged {
		if m, _ := body["start_time"].(map[string]string); m != nil {
			newStartTs = m["timestamp"]
		}
		if m, _ := body["end_time"].(map[string]string); m != nil {
			newEndTs = m["timestamp"]
		}
	}

	plan, err := resolveRoomCheckPlan(ctx, runtime, calendarID, eventID, newStartTs, newEndTs, timeChanged, rruleChanged)
	if err != nil {
		return err
	}
	if plan == nil {
		return nil
	}
	results, err := callRoomAvailabilityCheck(runtime, buildRoomCheckBody(calendarID, eventID, plan))
	if err != nil {
		// Degrade gracefully: warn on stderr and let the update proceed so the
		// pre-check API doesn't gate legitimate updates when it hiccups. For
		// 190014 (invalid_parameters) surface the server-supplied field-level
		// detail so agents can see why the precheck refused.
		msg := unwrapCalendarAPIError(err)
		if msg == "" {
			msg = err.Error()
		}
		fmt.Fprintf(runtime.IO().ErrOut,
			"[calendar +update] warning: room availability check failed (%s); proceeding with update — pass --%s to silence\n",
			msg, flagSkipRoomCheck)
		return nil
	}
	return blockOnUnavailableRooms(results, roomCheckPlanDurationSec(plan))
}

func dryRunCalendarUpdate(runtime *common.RuntimeContext) *common.DryRunAPI {
	calendarID, eventID := calendarUpdateIDs(runtime)
	displayCalendarID := calendarID
	if displayCalendarID == "" || displayCalendarID == "primary" {
		displayCalendarID = "<primary>"
	}

	body, hasEventFields, err := buildCalendarUpdateEventData(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}

	d := common.NewDryRunAPI().Set("calendar_id", displayCalendarID).Set("event_id", eventID)
	if scope := strings.TrimSpace(runtime.Str(flagApplyTo)); scope != "" {
		d.Set("apply_to", scope)
		switch scope {
		case applyToAll:
			d.Desc("recurring --apply-to=all: PATCH every exception first (only user-set fields), then the master; if --start/--end changed, exceptions are deleted first instead")
		case applyToThisAndFollowing:
			d.Desc("recurring --apply-to=this-and-following: delete exceptions on/after pivot → truncate master rrule with UNTIL → POST a new event carrying the requested edits (inherits master defaults for anything not overridden)")
		}
	}
	opCount := 0
	if hasEventFields {
		opCount++
	}
	if strings.TrimSpace(runtime.Str("remove-attendee-ids")) != "" {
		opCount++
	}
	if strings.TrimSpace(runtime.Str("add-attendee-ids")) != "" {
		opCount++
	}
	if opCount > 1 {
		d.Desc("multi-step update: event fields, attendee removal, and attendee addition run in order when requested")
	}
	steps := 0

	if !runtime.Bool(flagSkipRoomCheck) {
		newRooms := collectAttendeeRoomIDs(runtime.Str("add-attendee-ids"))
		timeChanged := runtime.Cmd.Flags().Changed("start") && runtime.Cmd.Flags().Changed("end")
		rruleChanged := runtime.Cmd.Flags().Changed("rrule")
		if len(newRooms) > 0 || timeChanged || rruleChanged {
			steps++
			desc := fmt.Sprintf("[%d] Pre-check meeting room availability (default; pass --%s to skip)", steps, flagSkipRoomCheck)
			previewBody := map[string]interface{}{
				"calendar_id":    displayCalendarID,
				"event_id":       eventID,
				"room_ids":       newRooms,
				"start_timezone": "<inherited from event>",
			}
			if start, _ := body["start_time"].(map[string]string); start != nil {
				previewBody["start_time"] = formatRoomCheckTime(start["timestamp"], time.Local)
			}
			if end, _ := body["end_time"].(map[string]string); end != nil {
				previewBody["end_time"] = formatRoomCheckTime(end["timestamp"], time.Local)
			}
			if rrule, _ := body["recurrence"].(string); rrule != "" {
				previewBody["event_rrule"] = rrule
			}
			d.POST(roomCheckPath).Desc(desc).Body(previewBody)
		}
	}

	if hasEventFields {
		steps++
		d.PATCH("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id").
			Desc(fmt.Sprintf("[%d] Update event fields", steps)).
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	}
	if removeStr := runtime.Str("remove-attendee-ids"); strings.TrimSpace(removeStr) != "" {
		deleteIDs, err := attendeeDeleteIDs(removeStr)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		steps++
		d.POST("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id/attendees/batch_delete").
			Desc(fmt.Sprintf("[%d] Remove attendees", steps)).
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(map[string]interface{}{"delete_ids": deleteIDs, "need_notification": runtime.Bool("notify")})
	}
	if addStr := runtime.Str("add-attendee-ids"); strings.TrimSpace(addStr) != "" {
		attendees, err := parseAttendees(addStr, "")
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		steps++
		d.POST("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id/attendees").
			Desc(fmt.Sprintf("[%d] Add attendees", steps)).
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(map[string]interface{}{"attendees": attendees, "need_notification": runtime.Bool("notify")})
	}
	return d
}

func executeCalendarUpdate(ctx context.Context, runtime *common.RuntimeContext) error {
	calendarID, eventID := calendarUpdateIDs(runtime)
	if eventID == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify --event-id").WithParam("--event-id")
	}

	if runtime.Cmd.Flags().Changed("description") {
		if err := resolveDescriptionImages(runtime, calendarID); err != nil {
			return err
		}
	}

	current, err := resolveCalendarEventOrMaster(runtime, calendarID, eventID)
	if err != nil {
		return err
	}
	kind := classifyRecurringEvent(current)
	scope, err := validateApplyTo(runtime, kind, eventID)
	if err != nil {
		return err
	}
	switch scope {
	case applyToSingle:
		return executeCalendarUpdateSingle(ctx, runtime, calendarID, eventID)
	case applyToAll:
		return executeCalendarUpdateAll(ctx, runtime, calendarID, current, eventID)
	case applyToThisAndFollowing:
		return executeCalendarUpdateThisAndFollowing(ctx, runtime, calendarID, current, eventID)
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "unhandled apply-to scope %q", scope)
}

// executeCalendarUpdateSingle mirrors the historical +update behavior when
// scope resolves to "single" (normal event or one recurring instance /
// exception). Delegates to applyUpdateToEvent so the master, every exception
// and single-event paths share the exact same PATCH + attendee-batch logic.
func executeCalendarUpdateSingle(ctx context.Context, runtime *common.RuntimeContext, calendarID, eventID string) error {
	if !runtime.Bool(flagSkipRoomCheck) {
		body, _, err := buildCalendarUpdateEventData(runtime)
		if err != nil {
			return err
		}
		if err := runRoomAvailabilityPrecheck(ctx, runtime, calendarID, eventID, body); err != nil {
			return err
		}
	}

	event, addedCount, removedCount, err := applyUpdateToEvent(runtime, calendarID, eventID, false)
	if err != nil {
		return withStepContext(err, "failed to update event %s", eventID)
	}

	result := map[string]interface{}{
		"calendar_id":   calendarID,
		"apply_to":      applyToSingle,
		"updated_event": calendarUpdateResult(eventID, event, addedCount, removedCount),
	}
	runtime.OutFormat(result, nil, func(w io.Writer) {
		writeUpdatePretty(w, result, eventID, applyToSingle, "updated_event", "Updated event", "Exceptions updated")
	})
	return nil
}

func calendarUpdateResult(eventID string, event map[string]interface{}, addedCount, removedCount int) map[string]interface{} {
	result := map[string]interface{}{
		"event_id":                eventID,
		"attendees_added_count":   addedCount,
		"attendees_removed_count": removedCount,
	}
	if summary, _ := event["summary"].(string); summary != "" {
		result["summary"] = summary
	}
	if rich, _ := event["description_rich"].(string); rich != "" {
		result["description"] = rich
	} else if plain, _ := event["description"].(string); plain != "" {
		result["description"] = plain
	}
	if start := formatCalendarEventTime(event["start_time"]); start != "" {
		result["start"] = start
	}
	if end := formatCalendarEventTime(event["end_time"]); end != "" {
		result["end"] = end
	}
	return result
}

func formatCalendarEventTime(v interface{}) string {
	m, _ := v.(map[string]interface{})
	if m == nil {
		return ""
	}
	if tsStr, _ := m["timestamp"].(string); tsStr != "" {
		if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
			return time.Unix(ts, 0).Local().Format(time.RFC3339)
		}
	}
	if dt, _ := m["datetime"].(string); dt != "" {
		return dt
	}
	if date, _ := m["date"].(string); date != "" {
		return date
	}
	return ""
}

// eventTimeAsMap projects a typed calendarEventTime back into the loose map
// shape formatCalendarEventTime expects, so callers with a parsed event can
// reuse the same rendering path as callers holding a raw response map.
func eventTimeAsMap(t *calendarEventTime) map[string]interface{} {
	if t == nil {
		return nil
	}
	m := map[string]interface{}{}
	if t.Date != "" {
		m["date"] = t.Date
	}
	if t.Timestamp != "" {
		m["timestamp"] = t.Timestamp
	}
	if t.Timezone != "" {
		m["timezone"] = t.Timezone
	}
	return m
}
