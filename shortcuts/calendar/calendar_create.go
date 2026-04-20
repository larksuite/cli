// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func buildEventData(runtime *common.RuntimeContext, startTs, endTs string) map[string]interface{} {
	eventData := map[string]interface{}{
		"summary":          runtime.Str("summary"),
		"description":      runtime.Str("description"),
		"start_time":       map[string]string{"timestamp": startTs},
		"end_time":         map[string]string{"timestamp": endTs},
		"attendee_ability": "can_modify_event",
		"free_busy_status": "busy",
		"vchat":            map[string]string{"vc_type": "vc"},
		"reminders": []map[string]int{
			{"minutes": 5},
		},
	}
	if rrule := runtime.Str("rrule"); rrule != "" {
		eventData["recurrence"] = rrule
	}
	return eventData
}

func parseAttendees(attendeesStr string, currentUserId string) ([]map[string]string, error) {
	if attendeesStr == "" && currentUserId == "" {
		return nil, nil
	}
	ids := strings.Split(attendeesStr, ",")
	uniqueIds := make(map[string]bool)
	if currentUserId != "" {
		uniqueIds[currentUserId] = true
	}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			uniqueIds[id] = true
		}
	}
	var attendees []map[string]string
	for id := range uniqueIds {
		switch {
		case strings.HasPrefix(id, "oc_"):
			attendees = append(attendees, map[string]string{"type": "chat", "chat_id": id})
		case strings.HasPrefix(id, "omm_"):
			attendees = append(attendees, map[string]string{"type": "resource", "room_id": id})
		case strings.HasPrefix(id, "ou_"):
			attendees = append(attendees, map[string]string{"type": "user", "user_id": id})
		default:
			return nil, fmt.Errorf("unsupported attendee id format: %s", id)
		}
	}
	return attendees, nil
}

var CalendarCreate = common.Shortcut{
	Service:     "calendar",
	Command:     "+create",
	Description: "Create a calendar event and optionally invite attendees",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:create", "calendar:calendar.event:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "summary", Desc: "event title"},
		{Name: "start", Desc: "start time (ISO 8601)", Required: true},
		{Name: "end", Desc: "end time (ISO 8601)", Required: true},
		{Name: "description", Desc: "event description"},
		{Name: "attendee-ids", Desc: "attendee IDs, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "rrule", Desc: "recurrence rule (rfc5545)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		for _, flag := range []string{"summary", "description", "rrule", "calendar-id"} {
			if val := runtime.Str(flag); val != "" {
				if err := common.RejectDangerousChars("--"+flag, val); err != nil {
					return output.ErrValidation(err.Error())
				}
			}
		}

		if attendeesStr := runtime.Str("attendee-ids"); attendeesStr != "" {
			for _, id := range strings.Split(attendeesStr, ",") {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if !strings.HasPrefix(id, "ou_") && !strings.HasPrefix(id, "oc_") && !strings.HasPrefix(id, "omm_") {
					return output.ErrValidation("invalid attendee id format %q: should start with 'ou_', 'oc_', or 'omm_'", id)
				}
			}
		}

		if runtime.Str("start") == "" {
			return common.FlagErrorf("specify --start (e.g. '2026-03-12T14:00+08:00')")
		}
		if runtime.Str("end") == "" {
			return common.FlagErrorf("specify --end (e.g. '2026-03-12T15:00+08:00')")
		}
		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return common.FlagErrorf("--start: %v", err)
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return common.FlagErrorf("--end: %v", err)
		}
		s, err := strconv.ParseInt(startTs, 10, 64)
		if err != nil {
			return common.FlagErrorf("invalid start time: %v", err)
		}
		e, err := strconv.ParseInt(endTs, 10, 64)
		if err != nil {
			return common.FlagErrorf("invalid end time: %v", err)
		}
		if e <= s {
			return common.FlagErrorf("end time must be after start time")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarId := runtime.Str("calendar-id")
		d := common.NewDryRunAPI()
		switch calendarId {
		case "":
			d.Desc("(calendar-id omitted) Will use primary calendar")
			calendarId = "<primary>"
		case "primary":
			calendarId = "<primary>"
		}
		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", fmt.Sprintf("--start: %v", err))
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return common.NewDryRunAPI().Set("error", fmt.Sprintf("--end: %v", err))
		}
		eventData := buildEventData(runtime, startTs, endTs)
		attendeesStr := runtime.Str("attendee-ids")
		if attendeesStr != "" {
			// Note: dry-run doesn't network resolve the current user's open_id.
			attendees, err := parseAttendees(attendeesStr, "")
			if err != nil {
				return common.NewDryRunAPI().Set("error", err.Error())
			}

			d.Desc("2-step: create event → add attendees (auto-rollback on failure)").
				POST("/open-apis/calendar/v4/calendars/:calendar_id/events").
				Desc("[1/2] Create event").
				Body(eventData).
				POST("/open-apis/calendar/v4/calendars/:calendar_id/events/<event_id>/attendees").
				Desc("[2/2] Add attendees (on failure: auto-delete event)").
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(map[string]interface{}{"attendees": attendees, "need_notification": true})
		} else {
			d.POST("/open-apis/calendar/v4/calendars/:calendar_id/events").
				Body(eventData)
		}
		return d.Set("calendar_id", calendarId)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}

		startTs, err := common.ParseTime(runtime.Str("start"))
		if err != nil {
			return output.ErrValidation("--start: %v", err)
		}
		endTs, err := common.ParseTime(runtime.Str("end"), "end")
		if err != nil {
			return output.ErrValidation("--end: %v", err)
		}

		eventData := buildEventData(runtime, startTs, endTs)

		// Create event
		data, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events", validate.EncodePathSegment(calendarId)),
			nil, eventData)
		if err != nil {
			return err
		}
		event, _ := data["event"].(map[string]interface{})
		eventId, _ := event["event_id"].(string)
		if eventId == "" {
			return output.Errorf(output.ExitAPI, "api_error", "failed to create event: no event_id returned")
		}

		// Add attendees if specified
		if attendeesStr := runtime.Str("attendee-ids"); attendeesStr != "" {
			currentUserId := ""
			if !runtime.IsBot() {
				currentUserId = runtime.UserOpenId()
			}
			attendees, err := parseAttendees(attendeesStr, currentUserId)
			if err != nil {
				return output.ErrValidation("invalid attendee id: %v", err)
			}

			_, err = runtime.CallAPI("POST",
				fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/attendees", validate.EncodePathSegment(calendarId), validate.EncodePathSegment(eventId)),
				map[string]interface{}{"user_id_type": "open_id"},
				map[string]interface{}{
					"attendees":         attendees,
					"need_notification": true,
				})
			if err != nil {
				// Rollback: delete the event
				_, rollbackErr := runtime.RawAPI("DELETE",
					fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s", validate.EncodePathSegment(calendarId), validate.EncodePathSegment(eventId)),
					map[string]interface{}{"need_notification": false}, nil)
				if rollbackErr != nil {
					return output.Errorf(output.ExitAPI, "api_error", "failed to add attendees: %v; rollback also failed, orphan event_id=%s needs manual cleanup", rollbackErr, eventId)
				}
				return output.Errorf(output.ExitAPI, "api_error", "failed to add attendees: %v; event rolled back successfully", err)
			}
		}

		startMap, _ := event["start_time"].(map[string]interface{})
		endMap, _ := event["end_time"].(map[string]interface{})

		// Replace timestamp with datetime (RFC3339, device timezone)
		if startMap != nil {
			if tsStr, ok := startMap["timestamp"].(string); ok && tsStr != "" {
				if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
					startMap["datetime"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
					delete(startMap, "timestamp")
				}
			}
		}
		if endMap != nil {
			if tsStr, ok := endMap["timestamp"].(string); ok && tsStr != "" {
				if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
					endMap["datetime"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
					delete(endMap, "timestamp")
				}
			}
			// If datetime is empty (all-day event), adjust date: date -> timestamp(00:00:00 UTC) -> -1s -> date
			if dt, _ := endMap["datetime"].(string); dt == "" {
				if dateStr, ok := endMap["date"].(string); ok && dateStr != "" {
					if t, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC); err == nil {
						endMap["date"] = t.Add(-1 * time.Second).Format("2006-01-02")
					}
				}
			}
		}

		var startStr, endStr string
		if startMap != nil {
			startStr, _ = startMap["datetime"].(string)
			if startStr == "" {
				startStr, _ = startMap["date"].(string)
			}
		}
		if endMap != nil {
			endStr, _ = endMap["datetime"].(string)
			if endStr == "" {
				endStr, _ = endMap["date"].(string)
			}
		}

		runtime.Out(map[string]interface{}{
			"event_id": eventId,
			"summary":  event["summary"],
			"start":    startStr,
			"end":      endStr,
		}, nil)
		return nil
	},
}
