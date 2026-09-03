// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +meeting-chat-create — create the meeting chat bound to a calendar
// event, so downstream IM commands can push post-meeting artifacts (minutes,
// notes, docs) into a chat that already contains every participant.

package calendar

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingChatCreateLogPrefix = "[calendar +meeting-chat-create]"

// meetingChatCreatePath renders the create endpoint for a given calendar and
// event. The server binds the whole recurring series (past / future /
// exceptions) to one shared meeting chat regardless of which instance-shaped id
// is used as the anchor, so callers do not need to normalise to the master
// themselves — except for the unmaterialised-instance case handled by the
// pre-probe below.
func meetingChatCreatePath(calendarID, eventID string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/%s/meeting_chat",
		validate.EncodePathSegment(calendarID), validate.EncodePathSegment(eventID))
}

// CalendarMeetingChatCreate creates the meeting chat for a calendar event.
var CalendarMeetingChatCreate = common.Shortcut{
	Service:     "calendar",
	Command:     "+meeting-chat-create",
	Description: "Create the meeting chat bound to a calendar event",
	Risk:        "write",
	Scopes:      []string{"calendar:calendar.event:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "event-id", Desc: "calendar event ID", Required: true},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		eventID := strings.TrimSpace(runtime.Str("event-id"))
		if eventID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--event-id is required").WithParam("--event-id")
		}
		if err := common.RejectDangerousCharsTyped("--event-id", eventID); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarID == "" {
			calendarID = PrimaryCalendarIDStr
		}
		eventID := strings.TrimSpace(runtime.Str("event-id"))
		return common.NewDryRunAPI().
			POST(meetingChatCreatePath(calendarID, eventID)).
			Body(map[string]any{}).
			Set("calendar_id", calendarID).
			Set("event_id", eventID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarID == "" {
			calendarID = PrimaryCalendarIDStr
		}
		eventID := strings.TrimSpace(runtime.Str("event-id"))

		// Pre-probe (前置判断): an instance-shaped id (`{uid}_{original_time>0}`)
		// may be a virtual occurrence that the server has never materialised. If
		// so, the create call reports 193001 (event not found) because the
		// per-instance record does not exist. We probe the instance first and, on
		// 193001, fall back to the master `{uid}_0` — which the server accepts and
		// binds to the entire series. A materialised exception is left untouched.
		targetEventID := eventID
		fellBackToMaster := false
		if masterID, ok := recurringMasterEventID(eventID); ok {
			if _, err := fetchCalendarEvent(runtime, calendarID, eventID); err != nil {
				if !isEventNotFound(err) {
					return err
				}
				fmt.Fprintf(errOut, "%s instance %s not materialised; falling back to recurring master %s\n",
					meetingChatCreateLogPrefix, eventID, masterID)
				targetEventID = masterID
				fellBackToMaster = true
			}
		}

		data, err := runtime.CallAPITyped("POST", meetingChatCreatePath(calendarID, targetEventID), nil, map[string]any{})
		if err != nil {
			return mapMeetingChatCreateError(err)
		}

		result := map[string]any{
			"meeting_chat_id": common.GetString(data, "meeting_chat_id"),
			"event_id_used":   targetEventID,
		}
		if fellBackToMaster {
			result["hint"] = "fell back to the recurring series: the provided instance is not a materialised exception, so the meeting chat was created on the master event " + targetEventID + " and is shared across the entire recurring series"
		}
		runtime.Out(result, nil)
		return nil
	},
}

// mapMeetingChatCreateError translates the create-specific business codes into
// typed errors carrying an actionable hint, while preserving the underlying
// APIError as the cause. Codes without a specialised message pass through
// unchanged so their server-supplied detail survives.
func mapMeetingChatCreateError(err error) error {
	if err == nil {
		return nil
	}
	var ae *errs.APIError
	if !errors.As(err, &ae) {
		return err
	}
	switch ae.Code {
	case 195109:
		return errs.NewAPIError(errs.SubtypeFailedPrecondition,
			"calendar event does not support meeting chat creation").
			WithCode(ae.Code).
			WithHint("meeting chat requires: the event lives on your PRIMARY calendar with WRITER access, has at least 2 attendees, and the attendee list is visible to guests (guests_can_see_other_guests). Check these on the event, then retry.").
			WithLogID(ae.LogID).
			WithCause(err)
	}
	return err
}
