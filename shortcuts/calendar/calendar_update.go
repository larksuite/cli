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

	"github.com/larksuite/cli/internal/output"
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
		{Name: "description", Desc: "event description"},
		{Name: "start", Desc: "new start time (ISO 8601); requires --end"},
		{Name: "end", Desc: "new end time (ISO 8601); requires --start"},
		{Name: "rrule", Desc: "recurrence rule (rfc5545)"},
		{Name: "add-attendee-ids", Desc: "attendee IDs to add, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{Name: "remove-attendee-ids", Desc: "attendee IDs to remove, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{Name: "notify", Type: "bool", Default: "true", Desc: "send update notification to attendees"},
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
			if err := common.RejectDangerousChars("--"+flag, val); err != nil {
				return output.ErrValidation(err.Error())
			}
		}
	}

	if strings.TrimSpace(runtime.Str("event-id")) == "" {
		return common.FlagErrorf("specify --event-id")
	}
	if _, _, err := buildCalendarUpdateEventData(runtime); err != nil {
		return err
	}
	if err := validateCalendarUpdateAttendees(runtime); err != nil {
		return err
	}
	if !hasCalendarUpdateOperation(runtime) {
		return common.FlagErrorf("nothing to update: specify at least one of --summary, --description, --start/--end, --rrule, --add-attendee-ids, or --remove-attendee-ids")
	}
	return nil
}

func validateCalendarUpdateAttendees(runtime *common.RuntimeContext) error {
	addIDs, err := parseCalendarAttendeeIDs(runtime.Str("add-attendee-ids"))
	if err != nil {
		return err
	}
	removeIDs, err := parseCalendarAttendeeIDs(runtime.Str("remove-attendee-ids"))
	if err != nil {
		return err
	}
	removeSet := make(map[string]struct{}, len(removeIDs))
	for _, id := range removeIDs {
		removeSet[id] = struct{}{}
	}
	for _, id := range addIDs {
		if _, ok := removeSet[id]; ok {
			return output.ErrValidation("attendee id %q appears in both --add-attendee-ids and --remove-attendee-ids", id)
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

	for _, field := range []string{"summary", "description"} {
		if runtime.Cmd.Flags().Changed(field) {
			body[field] = runtime.Str(field)
			hasFields = true
		}
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
		return nil, false, common.FlagErrorf("--start and --end must be specified together when updating event time")
	}
	if startChanged {
		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return nil, false, common.FlagErrorf("--start: %v", err)
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return nil, false, common.FlagErrorf("--end: %v", err)
		}
		s, err := strconv.ParseInt(startTs, 10, 64)
		if err != nil {
			return nil, false, common.FlagErrorf("invalid start time: %v", err)
		}
		e, err := strconv.ParseInt(endTs, 10, 64)
		if err != nil {
			return nil, false, common.FlagErrorf("invalid end time: %v", err)
		}
		if e <= s {
			return nil, false, common.FlagErrorf("end time must be after start time")
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
			return nil, output.ErrValidation("invalid attendee id format %q: should start with 'ou_', 'oc_', or 'omm_'", id)
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
			return nil, output.ErrValidation("invalid attendee id format %q: should start with 'ou_', 'oc_', or 'omm_'", id)
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

func executeCalendarUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	calendarID, eventID := calendarUpdateIDs(runtime)
	if eventID == "" {
		return output.ErrValidation("specify --event-id")
	}

	body, hasEventFields, err := buildCalendarUpdateEventData(runtime)
	if err != nil {
		return err
	}

	completed := []string{}
	event := map[string]interface{}{}
	if hasEventFields {
		data, err := runtime.CallAPI("PATCH", calendarUpdateEventPath(calendarID, eventID), map[string]interface{}{"user_id_type": "open_id"}, body)
		err = wrapPredefinedError(err)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "failed to update event %s: %v", eventID, err)
		}
		if v, _ := data["event"].(map[string]interface{}); v != nil {
			event = v
		}
		completed = append(completed, "event")
	}

	removedCount := 0
	if removeStr := runtime.Str("remove-attendee-ids"); strings.TrimSpace(removeStr) != "" {
		deleteIDs, err := attendeeDeleteIDs(removeStr)
		if err != nil {
			return err
		}
		_, err = runtime.CallAPI("POST", calendarUpdateAttendeesPath(calendarID, eventID)+"/batch_delete",
			map[string]interface{}{"user_id_type": "open_id"},
			map[string]interface{}{"delete_ids": deleteIDs, "need_notification": runtime.Bool("notify")})
		err = wrapPredefinedError(err)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "failed to remove attendees from event %s after completed steps %v: %v", eventID, completed, err)
		}
		removedCount = len(deleteIDs)
		completed = append(completed, "remove_attendees")
	}

	addedCount := 0
	if addStr := runtime.Str("add-attendee-ids"); strings.TrimSpace(addStr) != "" {
		attendees, err := parseAttendees(addStr, "")
		if err != nil {
			return output.ErrValidation("invalid attendee id: %v", err)
		}
		_, err = runtime.CallAPI("POST", calendarUpdateAttendeesPath(calendarID, eventID),
			map[string]interface{}{"user_id_type": "open_id"},
			map[string]interface{}{"attendees": attendees, "need_notification": runtime.Bool("notify")})
		err = wrapPredefinedError(err)
		if err != nil {
			return output.Errorf(output.ExitAPI, "api_error", "failed to add attendees to event %s after completed steps %v: %v", eventID, completed, err)
		}
		addedCount = len(attendees)
	}

	result := calendarUpdateResult(eventID, event, addedCount, removedCount)
	runtime.OutFormat(result, nil, func(w io.Writer) {
		output.PrintTable(w, []map[string]interface{}{result})
		fmt.Fprintln(w, "\nEvent updated successfully")
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
	if description, _ := event["description"].(string); description != "" {
		result["description"] = description
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
