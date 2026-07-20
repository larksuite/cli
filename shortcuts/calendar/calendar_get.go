// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +get — get a single calendar event detail by calendar_id and event_id

package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// calendarEventTime mirrors start_time / end_time in the API response.
type calendarEventTime struct {
	Date      string `json:"date,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
}

// calendarEventVChat mirrors the vchat block in the API response.
type calendarEventVChat struct {
	VCType      string `json:"vc_type,omitempty"`
	IconType    string `json:"icon_type,omitempty"`
	Description string `json:"description,omitempty"`
	MeetingURL  string `json:"meeting_url,omitempty"`
}

// calendarEventLocation mirrors the location block in the API response.
type calendarEventLocation struct {
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
	Latitude  float64 `json:"latitude,omitempty"`
	Longitude float64 `json:"longitude,omitempty"`
}

// calendarEventReminder mirrors a reminder entry.
type calendarEventReminder struct {
	Minutes int `json:"minutes"`
}

// calendarEventOrganizer mirrors event_organizer.
type calendarEventOrganizer struct {
	UserID      string `json:"user_id,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// calendarEventAttachment mirrors a single attachment entry.
type calendarEventAttachment struct {
	FileToken string `json:"file_token,omitempty"`
	FileSize  string `json:"file_size,omitempty"`
	Name      string `json:"name,omitempty"`
}

// calendarEventCheckInTime mirrors check_in_start_time / check_in_end_time.
type calendarEventCheckInTime struct {
	TimeType string `json:"time_type,omitempty"`
	Duration int    `json:"duration"`
}

// calendarEventCheckIn mirrors event_check_in.
type calendarEventCheckIn struct {
	EnableCheckIn       bool                      `json:"enable_check_in"`
	CheckInStartTime    *calendarEventCheckInTime `json:"check_in_start_time,omitempty"`
	CheckInEndTime      *calendarEventCheckInTime `json:"check_in_end_time,omitempty"`
	NeedNotifyAttendees bool                      `json:"need_notify_attendees"`
}

// calendarEvent mirrors the event object inside the API response.
type calendarEvent struct {
	EventID             string                    `json:"event_id,omitempty"`
	OrganizerCalendarID string                    `json:"organizer_calendar_id,omitempty"`
	Summary             string                    `json:"summary,omitempty"`
	Description         string                    `json:"description,omitempty"`
	StartTime           *calendarEventTime        `json:"start_time,omitempty"`
	EndTime             *calendarEventTime        `json:"end_time,omitempty"`
	VChat               *calendarEventVChat       `json:"vchat,omitempty"`
	Visibility          string                    `json:"visibility,omitempty"`
	AttendeeAbility     string                    `json:"attendee_ability,omitempty"`
	FreeBusyStatus      string                    `json:"free_busy_status,omitempty"`
	SelfRsvpStatus      string                    `json:"self_rsvp_status,omitempty"`
	Location            *calendarEventLocation    `json:"location,omitempty"`
	Color               int                       `json:"color,omitempty"`
	Reminders           []calendarEventReminder   `json:"reminders,omitempty"`
	Recurrence          string                    `json:"recurrence,omitempty"`
	Status              string                    `json:"status,omitempty"`
	IsException         bool                      `json:"is_exception,omitempty"`
	RecurringEventID    string                    `json:"recurring_event_id,omitempty"`
	CreateTime          string                    `json:"create_time,omitempty"`
	EventOrganizer      *calendarEventOrganizer   `json:"event_organizer,omitempty"`
	AppLink             string                    `json:"app_link,omitempty"`
	Attachments         []calendarEventAttachment `json:"attachments,omitempty"`
	EventCheckIn        *calendarEventCheckIn     `json:"event_check_in,omitempty"`
}

// parseCalendarEvent decodes the API response data into a typed calendarEvent.
func parseCalendarEvent(data map[string]any) (*calendarEvent, error) {
	rawEvent, ok := data["event"]
	if !ok || rawEvent == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event response missing 'event' field")
	}
	raw, err := json.Marshal(rawEvent)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event response: marshal failed: %s", err).WithCause(err)
	}
	var event calendarEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event response: unmarshal failed: %s", err).WithCause(err)
	}
	return &event, nil
}

// buildCalendarEventOutput converts the typed event into the output map and
// applies the four transformation rules:
//  1. create_time -> RFC3339
//  2. start_time / end_time timestamp -> datetime (RFC3339), drop timestamp
//  3. flatten event into the top-level result
//  4. when status != "cancelled", drop status (and adjust all-day end date)
func buildCalendarEventOutput(event *calendarEvent) (map[string]interface{}, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event marshal failed: %s", err).WithCause(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "calendar event unmarshal failed: %s", err).WithCause(err)
	}

	if ctStr, ok := out["create_time"].(string); ok && ctStr != "" {
		if ts, err := strconv.ParseInt(ctStr, 10, 64); err == nil {
			out["create_time"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
		}
	}

	if startMap, ok := out["start_time"].(map[string]interface{}); ok {
		if tsStr, ok := startMap["timestamp"].(string); ok && tsStr != "" {
			if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
				startMap["datetime"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
				delete(startMap, "timestamp")
			}
		}
	}
	if endMap, ok := out["end_time"].(map[string]interface{}); ok {
		if tsStr, ok := endMap["timestamp"].(string); ok && tsStr != "" {
			if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
				endMap["datetime"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
				delete(endMap, "timestamp")
			}
		}
		// All-day event: end date is exclusive in the API; rewind by 1s and reformat.
		if dt, _ := endMap["datetime"].(string); dt == "" {
			if dateStr, ok := endMap["date"].(string); ok && dateStr != "" {
				if t, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC); err == nil {
					endMap["date"] = t.Add(-1 * time.Second).Format("2006-01-02")
				}
			}
		}
	}

	if status, _ := out["status"].(string); status != "cancelled" {
		delete(out, "status")
	}

	return out, nil
}

// CalendarGet gets a single calendar event detail.
var CalendarGet = common.Shortcut{
	Service:     "calendar",
	Command:     "+get",
	Description: "Get a single calendar event detail by calendar-id and event-id",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "event-id", Desc: "event ID", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		for _, flag := range []string{"calendar-id", "event-id"} {
			if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
				if err := common.RejectDangerousCharsTyped("--"+flag, val); err != nil {
					return err
				}
			}
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		if eventId == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "event-id cannot be empty").WithParam("--event-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		d := common.NewDryRunAPI()
		switch calendarId {
		case "":
			d.Desc("(calendar-id omitted) Will use primary calendar")
			calendarId = "<primary>"
		case "primary":
			calendarId = "<primary>"
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		return d.
			GET("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id").
			Set("calendar_id", calendarId).
			Set("event_id", eventId)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))

		data, err := runtime.CallAPITyped("GET",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s",
				validate.EncodePathSegment(calendarId),
				validate.EncodePathSegment(eventId)),
			nil, nil)
		if err != nil {
			return err
		}

		event, err := parseCalendarEvent(data)
		if err != nil {
			return err
		}

		out, err := buildCalendarEventOutput(event)
		if err != nil {
			return err
		}

		runtime.OutFormat(out, nil, func(w io.Writer) {
			summary, _ := out["summary"].(string)
			if summary == "" {
				summary = "(untitled)"
			}
			startMap, _ := out["start_time"].(map[string]interface{})
			endMap, _ := out["end_time"].(map[string]interface{})
			startStr, _ := startMap["datetime"].(string)
			if startStr == "" {
				startStr, _ = startMap["date"].(string)
			}
			endStr, _ := endMap["datetime"].(string)
			if endStr == "" {
				endStr, _ = endMap["date"].(string)
			}
			eventIdOut, _ := out["event_id"].(string)
			freeBusyStatus, _ := out["free_busy_status"].(string)
			selfRsvpStatus, _ := out["self_rsvp_status"].(string)
			row := map[string]interface{}{
				"event_id":         eventIdOut,
				"summary":          summary,
				"start":            startStr,
				"end":              endStr,
				"free_busy_status": freeBusyStatus,
				"self_rsvp_status": selfRsvpStatus,
			}
			output.PrintTable(w, []map[string]interface{}{row})
			fmt.Fprintln(w)
		})
		return nil
	},
}
