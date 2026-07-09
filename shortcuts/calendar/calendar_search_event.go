// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// calendar +search-event — search calendar events by keyword, time range, and attendees

package calendar

import (
	"context"
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

const (
	defaultSearchEventPageSize = 20
	maxSearchEventPageSize     = 30
)

// searchEventTimeRange represents the time range filter for search_event API.
type searchEventTimeRange struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
}

// searchEventFilter represents the filter object for the search_event API request.
type searchEventFilter struct {
	AttendeeUserIDs []string              `json:"attendee_user_ids,omitempty"`
	AttendeeChatIDs []string              `json:"attendee_chat_ids,omitempty"`
	MeetingRoomIDs  []string              `json:"meeting_room_ids,omitempty"`
	TimeRange       *searchEventTimeRange `json:"time_range,omitempty"`
}

// searchEventRequestBody is the request body for the search_event API.
type searchEventRequestBody struct {
	Query  string             `json:"query"`
	Filter *searchEventFilter `json:"filter,omitempty"`
}

// searchEventTimeInfo represents start/end time info in the search result.
type searchEventTimeInfo struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"date_time,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

// searchEventItem represents a single event in the search result output.
type searchEventItem struct {
	EventID  string               `json:"event_id"`
	Summary  string               `json:"summary"`
	Start    *searchEventTimeInfo `json:"start,omitempty"`
	End      *searchEventTimeInfo `json:"end,omitempty"`
	IsAllDay bool                 `json:"is_all_day,omitempty"`
	AppLink  string               `json:"app_link,omitempty"`
}

// searchEventOutput is the structured output for +search-event.
type searchEventOutput struct {
	CalendarID string            `json:"calendar_id"`
	Items      []searchEventItem `json:"items"`
	HasMore    bool              `json:"has_more"`
	PageToken  string            `json:"page_token"`
}

// parseSearchEventTimeRange parses --start / --end into RFC3339 strings.
// When only one side is provided, the other defaults to the same day's
// boundary (start → end-of-day, end → start-of-day).
func parseSearchEventTimeRange(runtime *common.RuntimeContext) (string, string, error) {
	startInput := strings.TrimSpace(runtime.Str("start"))
	endInput := strings.TrimSpace(runtime.Str("end"))
	if startInput == "" && endInput == "" {
		return "", "", nil
	}

	var startSec, endSec int64

	if startInput != "" {
		ts, err := common.ParseTime(startInput)
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
		}
		startSec, _ = strconv.ParseInt(ts, 10, 64)
	}
	if endInput != "" {
		ts, err := common.ParseTime(endInput, "end")
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
		}
		endSec, _ = strconv.ParseInt(ts, 10, 64)
	}

	if startInput == "" {
		t := time.Unix(endSec, 0).In(time.Local)
		startSec = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
	}
	if endInput == "" {
		t := time.Unix(startSec, 0).In(time.Local)
		endSec = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, t.Location()).Unix()
	}

	if startSec > endSec {
		return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start must be before --end").WithParam("--start")
	}

	return time.Unix(startSec, 0).Format(time.RFC3339), time.Unix(endSec, 0).Format(time.RFC3339), nil
}

// buildSearchEventFilter builds the filter object for the search_event API.
func buildSearchEventFilter(runtime *common.RuntimeContext, startTime, endTime string) *searchEventFilter {
	attendeeIDs := common.SplitCSV(runtime.Str("attendee-ids"))

	var userIDs, chatIDs, roomIDs []string
	for _, id := range attendeeIDs {
		switch {
		case strings.HasPrefix(id, "ou_"):
			userIDs = append(userIDs, id)
		case strings.HasPrefix(id, "oc_"):
			chatIDs = append(chatIDs, id)
		case strings.HasPrefix(id, "omm_"):
			roomIDs = append(roomIDs, id)
		default:
			userIDs = append(userIDs, id)
		}
	}

	var tr *searchEventTimeRange
	if startTime != "" || endTime != "" {
		tr = &searchEventTimeRange{StartTime: startTime, EndTime: endTime}
	}

	if len(userIDs) == 0 && len(chatIDs) == 0 && len(roomIDs) == 0 && tr == nil {
		return nil
	}
	return &searchEventFilter{
		AttendeeUserIDs: userIDs,
		AttendeeChatIDs: chatIDs,
		MeetingRoomIDs:  roomIDs,
		TimeRange:       tr,
	}
}

// extractTimeInfo extracts time info from a meta_data start/end map.
func extractTimeInfo(m map[string]any) *searchEventTimeInfo {
	if m == nil {
		return nil
	}
	info := &searchEventTimeInfo{}
	if v, ok := m["date"].(string); ok && v != "" {
		info.Date = v
	}
	if v, ok := m["date_time"].(string); ok && v != "" {
		info.DateTime = v
	}
	if v, ok := m["timezone"].(string); ok && v != "" {
		info.Timezone = v
	}
	if info.Date == "" && info.DateTime == "" {
		return nil
	}
	return info
}

// CalendarSearchEvent searches calendar events by keyword, time range, and attendees.
var CalendarSearchEvent = common.Shortcut{
	Service:     "calendar",
	Command:     "+search-event",
	Description: "Search calendar events by keyword, time range, and attendees",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
		{Name: "query", Desc: "search keyword"},
		{Name: "attendee-ids", Desc: "attendee IDs, comma-separated (supports user ou_, chat oc_, room omm_)"},
		{Name: "start", Desc: "search time range start (ISO 8601 or YYYY-MM-DD)"},
		{Name: "end", Desc: "search time range end (ISO 8601 or YYYY-MM-DD)"},
		{Name: "page-token", Desc: "page token for next page"},
		{Name: "page-size", Default: "20", Desc: "page size, 1-30 (default 20)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if err := rejectCalendarAutoBotFallback(runtime); err != nil {
			return err
		}
		if _, _, err := parseSearchEventTimeRange(runtime); err != nil {
			return err
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", defaultSearchEventPageSize, 1, maxSearchEventPageSize); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		calendarID := runtime.Str("calendar-id")
		if calendarID == "" {
			calendarID = "<primary>"
		}
		return common.NewDryRunAPI().
			POST(fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/search_event", calendarID)).
			Set("calendar_id", calendarID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		calendarID := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarID == "" {
			calendarID = PrimaryCalendarIDStr
		}

		startTime, endTime, err := parseSearchEventTimeRange(runtime)
		if err != nil {
			return err
		}

		// Build request body — always send query (even if empty)
		body := &searchEventRequestBody{
			Query: strings.TrimSpace(runtime.Str("query")),
		}
		if filter := buildSearchEventFilter(runtime, startTime, endTime); filter != nil {
			body.Filter = filter
		}

		// Build query params
		params := map[string]any{}
		pageSize, _ := strconv.Atoi(strings.TrimSpace(runtime.Str("page-size")))
		if pageSize <= 0 {
			pageSize = defaultSearchEventPageSize
		}
		params["page_size"] = strconv.Itoa(pageSize)
		if pt := strings.TrimSpace(runtime.Str("page-token")); pt != "" {
			params["page_token"] = pt
		}

		data, err := runtime.CallAPITyped("POST",
			fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/search_event", validate.EncodePathSegment(calendarID)),
			params, body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]any{}
		}

		items := common.GetSlice(data, "items")
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)

		// Transform items to structured output
		outItems := make([]searchEventItem, 0, len(items))
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			if item == nil {
				continue
			}
			meta, _ := item["meta_data"].(map[string]any)
			out := searchEventItem{}
			if meta != nil {
				if v, ok := meta["event_id"].(string); ok {
					out.EventID = v
				}
				if v, ok := meta["summary"].(string); ok {
					out.Summary = v
				}
				if v, ok := meta["is_all_day"].(bool); ok {
					out.IsAllDay = v
				}
				if v, ok := meta["app_link"].(string); ok {
					out.AppLink = v
				}
				if start, ok := meta["start"].(map[string]any); ok {
					out.Start = extractTimeInfo(start)
				}
				if end, ok := meta["end"].(map[string]any); ok {
					out.End = extractTimeInfo(end)
				}
			}
			outItems = append(outItems, out)
		}

		outData := searchEventOutput{
			CalendarID: calendarID,
			Items:      outItems,
			HasMore:    hasMore,
			PageToken:  pageToken,
		}

		runtime.OutFormat(outData, &output.Meta{Count: len(outItems)}, func(w io.Writer) {
			if len(outItems) == 0 {
				fmt.Fprintln(w, "No events found.")
				return
			}
			var rows []map[string]interface{}
			for _, item := range outItems {
				row := map[string]interface{}{
					"event_id": item.EventID,
					"summary":  common.TruncateStr(item.Summary, 40),
				}
				if item.Start != nil {
					if item.Start.DateTime != "" {
						row["start"] = item.Start.DateTime
					} else if item.Start.Date != "" {
						row["start"] = item.Start.Date
					}
				}
				if item.End != nil {
					if item.End.DateTime != "" {
						row["end"] = item.End.DateTime
					} else if item.End.Date != "" {
						row["end"] = item.End.Date
					}
				}
				if item.IsAllDay {
					row["is_all_day"] = true
				}
				rows = append(rows, row)
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d event(s) found\n", len(outItems))
		})

		if hasMore && runtime.Format != "json" && runtime.Format != "" {
			fmt.Fprintf(runtime.IO().Out, "\n(more available, page_token: %s)\n", pageToken)
		}
		return nil
	},
}
