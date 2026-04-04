// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxInstanceViewSpanSeconds = 40 * 24 * 60 * 60
const minSplitWindowSeconds = 2 * 60 * 60

// Calendar API error codes.
const (
	larkErrCalendarTimeRangeExceeded = 193103 // instance_view query time range exceeds 40-day limit
	larkErrCalendarTooManyInstances  = 193104 // instance_view returns more than 1000 instances
)

func fetchInstanceViewRange(ctx context.Context, runtime *common.RuntimeContext, calendarId string, startTime, endTime int64, depth int) ([]map[string]interface{}, error) {
	if depth > 10 {
		return nil, output.Errorf(output.ExitInternal, "recursion_limit", "too many splits for instance_view")
	}
	if startTime > endTime {
		return nil, nil
	}
	span := endTime - startTime
	if span > maxInstanceViewSpanSeconds {
		mid := startTime + span/2
		left, err := fetchInstanceViewRange(ctx, runtime, calendarId, startTime, mid, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := fetchInstanceViewRange(ctx, runtime, calendarId, mid+1, endTime, depth+1)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	}

	result, err := runtime.RawAPI("GET",
		fmt.Sprintf("/open-apis/calendar/v4/calendars/%s/events/instance_view", validate.EncodePathSegment(calendarId)),
		map[string]interface{}{
			"start_time": fmt.Sprintf("%d", startTime),
			"end_time":   fmt.Sprintf("%d", endTime),
		}, nil)
	if err != nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "API call failed: %s", err)
	}

	resultMap, _ := result.(map[string]interface{})
	code, _ := util.ToFloat64(resultMap["code"])

	if code == 0 {
		data, _ := resultMap["data"].(map[string]interface{})
		items, _ := data["items"].([]interface{})
		var events []map[string]interface{}
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				events = append(events, m)
			}
		}
		return events, nil
	}

	// Error 193103: time range exceeds limit -> split
	if int(code) == larkErrCalendarTimeRangeExceeded {
		mid := startTime + span/2
		if mid <= startTime {
			return nil, output.Errorf(output.ExitAPI, "api_error", "query failed: time range exceeds 40-day limit, please narrow the range")
		}
		left, err := fetchInstanceViewRange(ctx, runtime, calendarId, startTime, mid, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := fetchInstanceViewRange(ctx, runtime, calendarId, mid+1, endTime, depth+1)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	}

	// Error 193104: too many instances -> split
	if int(code) == larkErrCalendarTooManyInstances {
		if span <= minSplitWindowSeconds {
			return nil, output.Errorf(output.ExitAPI, "api_error", "query failed: more than 1000 instances in the time range, please narrow the range")
		}
		mid := startTime + span/2
		left, err := fetchInstanceViewRange(ctx, runtime, calendarId, startTime, mid, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := fetchInstanceViewRange(ctx, runtime, calendarId, mid+1, endTime, depth+1)
		if err != nil {
			return nil, err
		}
		return append(left, right...), nil
	}

	msg, _ := resultMap["msg"].(string)
	return nil, output.ErrAPI(int(code), msg, resultMap["error"])
}

func dedupeAndSortItems(items []map[string]interface{}) []map[string]interface{} {
	seen := make(map[string]bool)
	var result []map[string]interface{}
	for _, e := range items {
		eventId, _ := e["event_id"].(string)
		startMap, _ := e["start_time"].(map[string]interface{})
		endMap, _ := e["end_time"].(map[string]interface{})
		startTs, _ := startMap["timestamp"].(string)
		endTs, _ := endMap["timestamp"].(string)
		key := eventId + "|" + startTs + "|" + endTs
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		si, _ := result[i]["start_time"].(map[string]interface{})
		sj, _ := result[j]["start_time"].(map[string]interface{})
		ti, _ := si["timestamp"].(string)
		tj, _ := sj["timestamp"].(string)
		ni, _ := strconv.ParseInt(ti, 10, 64)
		nj, _ := strconv.ParseInt(tj, 10, 64)
		return ni < nj
	})

	return result
}

// parseTimeRange parses --start/--end into Unix seconds.
func parseTimeRange(runtime *common.RuntimeContext) (int64, int64, error) {
	startInput, endInput := resolveStartEnd(runtime)

	startTime, err := common.ParseTime(startInput)
	if err != nil {
		return 0, 0, output.ErrValidation("--start: %v", err)
	}
	endTime, err := common.ParseTime(endInput, "end")
	if err != nil {
		return 0, 0, output.ErrValidation("--end: %v", err)
	}

	startInt, err := strconv.ParseInt(startTime, 10, 64)
	if err != nil {
		return 0, 0, output.ErrValidation("invalid start time: %v", err)
	}
	endInt, err := strconv.ParseInt(endTime, 10, 64)
	if err != nil {
		return 0, 0, output.ErrValidation("invalid end time: %v", err)
	}

	return startInt, endInt, nil
}

var CalendarAgenda = common.Shortcut{
	Service:     "calendar",
	Command:     "+agenda",
	Description: "View calendar agenda (defaults to today)",
	Risk:        "read",
	Scopes:      []string{"calendar:calendar.event:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "start", Desc: "start time (ISO 8601, default: start of today)"},
		{Name: "end", Desc: "end time (ISO 8601, default: end of start day)"},
		{Name: "calendar-id", Desc: "calendar ID (default: primary)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return rejectCalendarAutoBotFallback(runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		startInt, endInt, err := parseTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
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
			GET("/open-apis/calendar/v4/calendars/:calendar_id/events/instance_view").
			Params(map[string]interface{}{"start_time": fmt.Sprintf("%d", startInt), "end_time": fmt.Sprintf("%d", endInt)}).
			Set("calendar_id", calendarId)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		startInt, endInt, err := parseTimeRange(runtime)
		if err != nil {
			return err
		}
		calendarId := strings.TrimSpace(runtime.Str("calendar-id"))
		if calendarId == "" {
			calendarId = PrimaryCalendarIDStr
		}

		items, err := fetchInstanceViewRange(ctx, runtime, calendarId, startInt, endInt, 0)
		if err != nil {
			return err
		}
		visible := dedupeAndSortItems(items)

		// Filter cancelled
		filtered := make([]map[string]interface{}, 0)
		for _, e := range visible {
			status, _ := e["status"].(string)
			if status != "cancelled" {
				delete(e, "status")
				delete(e, "attendees")

				// Replace timestamp with datetime (RFC3339, device timezone)
				if startMap, ok := e["start_time"].(map[string]interface{}); ok {
					if tsStr, ok := startMap["timestamp"].(string); ok && tsStr != "" {
						if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
							startMap["datetime"] = time.Unix(ts, 0).Local().Format(time.RFC3339)
							delete(startMap, "timestamp")
						}
					}
				}
				if endMap, ok := e["end_time"].(map[string]interface{}); ok {
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

				filtered = append(filtered, e)
			}
		}

		runtime.OutFormat(filtered, &output.Meta{Count: len(filtered)}, func(w io.Writer) {
			if len(filtered) == 0 {
				fmt.Fprintln(w, "No events in this time range.")
				return
			}

			var rows []map[string]interface{}
			for _, e := range filtered {
				summary, _ := e["summary"].(string)
				if summary == "" {
					summary = "(untitled)"
				}
				summary = common.TruncateStr(summary, 40)
				startMap, _ := e["start_time"].(map[string]interface{})
				endMap, _ := e["end_time"].(map[string]interface{})
				startStr, _ := startMap["datetime"].(string)
				if startStr == "" {
					startStr, _ = startMap["date"].(string)
				}
				endStr, _ := endMap["datetime"].(string)
				if endStr == "" {
					endStr, _ = endMap["date"].(string)
				}
				freeBusyStatus, _ := e["free_busy_status"].(string)
				selfRsvpStatus, _ := e["self_rsvp_status"].(string)
				eventId, _ := e["event_id"].(string)
				rows = append(rows, map[string]interface{}{
					"event_id":         eventId,
					"summary":          summary,
					"start":            startStr,
					"end":              endStr,
					"free_busy_status": freeBusyStatus,
					"self_rsvp_status": selfRsvpStatus,
				})
			}
			output.PrintTable(w, rows)
			fmt.Fprintf(w, "\n%d event(s) total\n", len(filtered))
		})
		return nil
	},
}
