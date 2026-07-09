// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +meeting — get meeting info for calendar events via mget_instance_relation_info

package calendar

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const meetingLogPrefix = "[calendar +meeting]"

// mgetInstanceRelationRequestBody is the request body for mget_instance_relation_info API.
type mgetInstanceRelationRequestBody struct {
	InstanceIDs            []string `json:"instance_ids"`
	NeedMeetingInstanceIDs bool     `json:"need_meeting_instance_ids"`
	NeedMeetingNotes       bool     `json:"need_meeting_notes"`
	NeedAIMeetingNotes     bool     `json:"need_ai_meeting_notes"`
}

// meetingInfoItem represents a single event's meeting info in the output.
type meetingInfoItem struct {
	EventID     string `json:"event_id"`
	MeetingID   string `json:"meeting_id,omitempty"`
	MeetingNote string `json:"meeting_note,omitempty"`
	Error       string `json:"error,omitempty"`
	Hint        string `json:"hint,omitempty"`
}

// translateFailMsg converts API fail_msg to a user-friendly error message.
func translateFailMsg(failMsg string) string {
	switch failMsg {
	case "No Permission":
		return "no read permission for this calendar event (not a participant of the event)"
	case "Not Found":
		return "event not found on the specified calendar (event ID may be incorrect or does not belong to this calendar)"
	default:
		return failMsg
	}
}

// fetchEventMeetingInfo queries mget_instance_relation_info for a single event instance.
func fetchEventMeetingInfo(ctx context.Context, runtime *common.RuntimeContext, instanceID, calendarID string) *meetingInfoItem {
	body := &mgetInstanceRelationRequestBody{
		InstanceIDs:            []string{instanceID},
		NeedMeetingInstanceIDs: true,
		NeedMeetingNotes:       true,
		NeedAIMeetingNotes:     false,
	}

	data, err := runtime.CallAPITyped("POST",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", validate.EncodePathSegment(calendarID)),
		nil, body)
	if err != nil {
		msg := unwrapCalendarAPIError(err)
		if msg == "" {
			msg = err.Error()
		}
		return &meetingInfoItem{EventID: instanceID, Error: msg}
	}

	// Check for failed instance IDs first
	if failedIDs, _ := data["failed_instance_ids"].([]any); len(failedIDs) > 0 {
		for _, raw := range failedIDs {
			if failInfo, ok := raw.(map[string]any); ok {
				if failID, _ := failInfo["instance_id"].(string); failID == instanceID {
					failMsg, _ := failInfo["fail_msg"].(string)
					return &meetingInfoItem{EventID: instanceID, Error: translateFailMsg(failMsg)}
				}
			}
		}
	}

	infos, _ := data["instance_relation_infos"].([]any)
	if len(infos) == 0 {
		return &meetingInfoItem{EventID: instanceID, Error: "no event relation info found"}
	}

	info, _ := infos[0].(map[string]any)
	result := &meetingInfoItem{EventID: instanceID}

	// Extract meeting_id (return first if multiple) — API returns string
	if rawIDs, _ := info["meeting_instance_ids"].([]any); len(rawIDs) > 0 {
		if id, ok := rawIDs[0].(string); ok && id != "" {
			result.MeetingID = id
		}
	}

	// Extract meeting_note (return first if multiple)
	if notes, _ := info["meeting_notes"].([]any); len(notes) > 0 {
		if note, ok := notes[0].(string); ok && note != "" {
			result.MeetingNote = note
		}
	}

	// Add hints for empty resources (independent checks)
	var emptyFields []string
	if result.MeetingID == "" {
		emptyFields = append(emptyFields, "meeting_id")
	}
	if result.MeetingNote == "" {
		emptyFields = append(emptyFields, "meeting_note")
	}
	if len(emptyFields) > 0 {
		result.Hint = fmt.Sprintf("%s not found for this event", strings.Join(emptyFields, ", "))
	}

	return result
}

// CalendarMeeting gets meeting info for calendar events.
var CalendarMeeting = common.Shortcut{
	Service:     "calendar",
	Command:     "+meeting",
	Description: "Get meeting info for calendar events (meeting_id, meeting_note)",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "event-ids", Desc: "calendar event instance IDs, comma-separated for batch", Required: true},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		ids := common.SplitCSV(runtime.Str("event-ids"))
		const maxBatchSize = 50
		if len(ids) > maxBatchSize {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--event-ids: too many IDs (%d), maximum is %d", len(ids), maxBatchSize).WithParam("--event-ids")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarID := runtime.Str("calendar-id")
		if calendarID == "" {
			calendarID = "<primary>"
		}
		return common.NewDryRunAPI().
			POST(fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/mget_instance_relation_info", calendarID)).
			Set("event_ids", common.SplitCSV(runtime.Str("event-ids"))).
			Set("calendar_id", calendarID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		errOut := runtime.IO().ErrOut
		instanceIDs := common.SplitCSV(runtime.Str("event-ids"))
		calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarID == "" {
			calendarID = PrimaryCalendarIDStr
		}

		results := make([]*meetingInfoItem, 0, len(instanceIDs))
		fmt.Fprintf(errOut, "%s querying %d event_id(s)\n", meetingLogPrefix, len(instanceIDs))
		for _, id := range instanceIDs {
			if err := ctx.Err(); err != nil {
				return err
			}
			fmt.Fprintf(errOut, "%s querying event_id=%s ...\n", meetingLogPrefix, id)
			results = append(results, fetchEventMeetingInfo(ctx, runtime, id, calendarID))
		}

		successCount := 0
		for _, r := range results {
			if r.Error == "" {
				successCount++
			}
		}
		fmt.Fprintf(errOut, "%s done: %d total, %d succeeded, %d failed\n", meetingLogPrefix, len(results), successCount, len(results)-successCount)

		if successCount == 0 && len(results) > 0 {
			return runtime.OutPartialFailure(map[string]any{"meetings": results}, &output.Meta{Count: len(results)})
		}

		outData := map[string]any{"meetings": results}
		runtime.OutFormat(outData, &output.Meta{Count: len(results)}, func(w io.Writer) {
			if len(results) == 0 {
				fmt.Fprintln(w, "No events.")
				return
			}
			var rows []map[string]interface{}
			for _, r := range results {
				row := map[string]interface{}{"event_id": r.EventID}
				if r.Error != "" {
					row["status"] = "FAIL"
					row["error"] = r.Error
				} else {
					row["status"] = "OK"
					if r.MeetingID != "" {
						row["meeting_id"] = r.MeetingID
					}
					if r.MeetingNote != "" {
						row["meeting_note"] = r.MeetingNote
					}
					if r.Hint != "" {
						row["hint"] = r.Hint
					}
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d event(s), %d succeeded, %d failed\n", len(results), successCount, len(results)-successCount)
		})
		return nil
	},
}
