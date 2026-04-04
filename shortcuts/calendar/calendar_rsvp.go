// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var CalendarRsvp = common.Shortcut{
	Service:     "calendar",
	Command:     "+rsvp",
	Description: "Reply to a calendar event (accept/decline/tentative)",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:reply"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   false,
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "event-id", Desc: "event ID", Required: true},
		{Name: "rsvp-status", Desc: "reply status", Required: true, Enum: []string{"accept", "decline", "tentative"}},
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
		status := strings.TrimSpace(runtime.Str("rsvp-status"))

		return d.
			POST("/open-apis/calendar/v4/calendars/:calendar_id/events/:event_id/reply").
			Body(map[string]interface{}{"rsvp_status": status}).
			Set("calendar_id", calendarId).
			Set("event_id", eventId)
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		for _, flag := range []string{"calendar-id", "event-id", "rsvp-status"} {
			if val := strings.TrimSpace(runtime.Str(flag)); val != "" {
				if err := common.RejectDangerousChars("--"+flag, val); err != nil {
					return output.ErrValidation(err.Error())
				}
			}
		}

		eventId := strings.TrimSpace(runtime.Str("event-id"))
		if eventId == "" {
			return output.ErrValidation("event-id cannot be empty")
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		status := strings.TrimSpace(runtime.Str("rsvp-status"))

		_, err := runtime.DoAPIJSON("POST",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/reply",
				validate.EncodePathSegment(calendarId),
				validate.EncodePathSegment(eventId)),
			nil,
			map[string]interface{}{
				"rsvp_status": status,
			})
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"calendar_id": calendarId,
			"event_id":    eventId,
			"rsvp_status": status,
		}, nil)
		return nil
	},
}
