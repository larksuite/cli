// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +meeting-chat-get — look up the meeting chat bound to a calendar
// event, so callers can reuse an existing chat (created pre-meeting) for
// post-meeting notification instead of creating a new one.

package calendar

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingChatGetLogPrefix = "[calendar +meeting-chat-get]"

// meetingChatGetPath renders the mget endpoint for a calendar. The server only
// accepts the caller's PRIMARY calendar here; a non-primary calendar_id is
// rejected at the top level (190002 with details), not as a per-event failure.
func meetingChatGetPath(calendarID string) string {
	return fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_meeting_chat",
		validate.EncodePathSegment(calendarID))
}

// CalendarMeetingChatGet looks up the meeting chat bound to a single calendar
// event. The underlying OAPI is a batch mget, but this shortcut deliberately
// exposes a single-event surface (matching the design) and surfaces the
// per-event fail_msg verbatim when the lookup misses.
var CalendarMeetingChatGet = common.Shortcut{
	Service:     "calendar",
	Command:     "+meeting-chat-get",
	Description: "Look up the meeting chat bound to a calendar event",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
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
			POST(meetingChatGetPath(calendarID)).
			Body(map[string]any{"event_ids": []string{eventID}}).
			Set("calendar_id", calendarID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarID == "" {
			calendarID = PrimaryCalendarIDStr
		}
		eventID := strings.TrimSpace(runtime.Str("event-id"))

		// Pre-probe (前置判断): mirror the create path. An unmaterialised
		// instance id resolves to 193001 on the underlying event lookup, and the
		// mget would report it as a `Not Found` per-event failure. Probe the
		// instance first; on 193001 query the recurring master `{uid}_0` instead
		// (the whole series shares one chat) and flag the downgrade in the hint.
		targetEventID := eventID
		fellBackToMaster := false
		if masterID, ok := recurringMasterEventID(eventID); ok {
			if _, err := fetchCalendarEvent(runtime, calendarID, eventID); err != nil {
				if !isEventNotFound(err) {
					return err
				}
				fmt.Fprintf(errOut, "%s instance %s not materialised; querying recurring master %s\n",
					meetingChatGetLogPrefix, eventID, masterID)
				targetEventID = masterID
				fellBackToMaster = true
			}
		}

		data, err := runtime.CallAPITyped("POST", meetingChatGetPath(calendarID), nil,
			map[string]any{"event_ids": []string{targetEventID}})
		if err != nil {
			return err
		}

		result := map[string]any{"event_id": targetEventID}
		if fellBackToMaster {
			result["hint"] = "fell back to the recurring series: the provided instance is not a materialised exception, so the meeting chat was looked up on the master event " + targetEventID
		}

		// A hit lands in meeting_chats[]; a miss lands in failed_event_ids[] with
		// a per-event fail_msg. The top-level code stays 0 in both cases, so we
		// surface the fail_msg on stdout rather than raising an error.
		if chat := findMeetingChat(data, targetEventID); chat != nil {
			result["found"] = true
			result["meeting_chat_id"] = common.GetString(chat, "meeting_chat_id")
		} else {
			result["found"] = false
			if failMsg := findMeetingChatFailMsg(data, targetEventID); failMsg != "" {
				result["fail_msg"] = failMsg
			}
		}

		runtime.Out(result, nil)
		return nil
	},
}

// findMeetingChat returns the meeting_chats[] entry matching eventID, or nil
// when the event was not returned as a hit.
func findMeetingChat(data map[string]interface{}, eventID string) map[string]interface{} {
	for _, raw := range common.GetSlice(data, "meeting_chats") {
		chat, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if common.GetString(chat, "event_id") == eventID {
			return chat
		}
	}
	return nil
}

// findMeetingChatFailMsg returns the failed_event_ids[] fail_msg for eventID,
// or "" when no per-event failure was reported.
func findMeetingChatFailMsg(data map[string]interface{}, eventID string) string {
	for _, raw := range common.GetSlice(data, "failed_event_ids") {
		fail, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if common.GetString(fail, "event_id") == eventID {
			return common.GetString(fail, "fail_msg")
		}
	}
	return ""
}
