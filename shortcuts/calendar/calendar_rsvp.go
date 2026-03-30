// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var validRsvpStatuses = map[string]bool{
	"accept":    true,
	"decline":   true,
	"tentative": true,
}

var CalendarRsvp = common.Shortcut{
	Service:     "calendar",
	Command:     "+rsvp",
	Description: "Reply to a calendar event invitation (accept, decline, or tentative)",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:reply"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "event-id", Desc: "event ID (from +agenda output)", Required: true},
		{Name: "status", Desc: "RSVP status: accept | decline | tentative", Required: true},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		status := strings.ToLower(strings.TrimSpace(runtime.Str("status")))
		if !validRsvpStatuses[status] {
			return common.FlagErrorf("--status must be one of: accept, decline, tentative")
		}
		if runtime.Str("event-id") == "" {
			return common.FlagErrorf("--event-id is required")
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
		return d.
			POST(fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/reply", calendarId, runtime.Str("event-id"))).
			Body(map[string]interface{}{"rsvp_status": strings.ToLower(strings.TrimSpace(runtime.Str("status")))}).
			Set("calendar_id", calendarId)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}
		eventId := strings.TrimSpace(runtime.Str("event-id"))
		status := strings.ToLower(strings.TrimSpace(runtime.Str("status")))

		_, err := runtime.CallAPI("POST",
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
			"event_id":    eventId,
			"rsvp_status": status,
			"message":     fmt.Sprintf("Successfully %sed the event", status),
		}, nil)
		return nil
	},
}
