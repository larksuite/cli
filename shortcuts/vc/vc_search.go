// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultVCSearchPageSize = 15
	maxVCSearchPageSize     = 30
	maxVCSearchQueryLen     = 50
)

// toRFC3339 parses a time string via ParseTime (unix timestamp) and formats it as RFC3339.
func toRFC3339(input string, hint ...string) (string, error) {
	ts, err := common.ParseTime(input, hint...)
	if err != nil {
		return "", err
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp %q: %w", ts, err) //nolint:forbidigo // intermediate parse error; callers wrap it into a typed ValidationError
	}
	return time.Unix(sec, 0).Format(time.RFC3339), nil
}

// parseTimeRange validates --start/--end and returns RFC3339 formatted strings.
func parseTimeRange(runtime *common.RuntimeContext) (string, string, error) {
	start := strings.TrimSpace(runtime.Str("start"))
	end := strings.TrimSpace(runtime.Str("end"))
	if start == "" && end == "" {
		return "", "", nil
	}
	var startTime, endTime string
	if start != "" {
		parsed, err := toRFC3339(start)
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start: %v", err).WithParam("--start")
		}
		startTime = parsed
	}
	if end != "" {
		parsed, err := toRFC3339(end, "end")
		if err != nil {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--end: %v", err).WithParam("--end")
		}
		endTime = parsed
	}
	// validate start <= end
	if startTime != "" && endTime != "" {
		st, _ := time.Parse(time.RFC3339, startTime)
		et, _ := time.Parse(time.RFC3339, endTime)
		if st.After(et) {
			return "", "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--start (%s) is after --end (%s)", start, end).WithParam("--start")
		}
	}
	return startTime, endTime, nil
}

// uniqueIDs deduplicates a string slice while preserving order.
func uniqueIDs(ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ids))
	var out []string
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// buildTimeFilter returns a time range filter if start or end is non-empty.
func buildTimeFilter(startTime, endTime string) map[string]interface{} {
	if startTime == "" && endTime == "" {
		return nil
	}
	timeRange := map[string]interface{}{}
	if startTime != "" {
		timeRange["start_time"] = startTime
	}
	if endTime != "" {
		timeRange["end_time"] = endTime
	}
	return timeRange
}

// buildMeetingFilter assembles the meeting_filter object from flags and time range.
// Note: the API expects time range under "start_time" key in meeting_filter.
func buildMeetingFilter(participants, organizers, rooms []string, timeRange map[string]interface{}) map[string]interface{} {
	filter := map[string]interface{}{}
	if timeRange != nil {
		filter["start_time"] = timeRange
	}
	if len(participants) > 0 {
		filter["participant_ids"] = participants
	}
	if len(organizers) > 0 {
		filter["organizer_ids"] = organizers
	}
	if len(rooms) > 0 {
		filter["open_room_ids"] = rooms
	}
	if len(filter) == 0 {
		return nil
	}
	return filter
}

// buildSearchBody builds the request body for meeting search API.
func buildSearchBody(runtime *common.RuntimeContext, startTime, endTime string) map[string]interface{} {
	body := map[string]interface{}{}
	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		body["query"] = q
	}
	participants := uniqueIDs(common.SplitCSV(runtime.Str("participant-ids")))
	organizers := common.SplitCSV(runtime.Str("organizer-ids"))
	rooms := common.SplitCSV(runtime.Str("room-ids"))
	if filter := buildMeetingFilter(participants, organizers, rooms, buildTimeFilter(startTime, endTime)); filter != nil {
		body["meeting_filter"] = filter
	}
	return body
}

func buildSearchParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}
	pageToken := strings.TrimSpace(runtime.Str("page-token"))
	pageSize, _ := strconv.Atoi(strings.TrimSpace(runtime.Str("page-size")))
	if pageSize <= 0 {
		pageSize = defaultVCSearchPageSize
	}
	params["page_size"] = strconv.Itoa(pageSize)
	if pageToken != "" {
		params["page_token"] = pageToken
	}
	return params
}

func meetingSearchDisplayInfo(item map[string]interface{}) string {
	if displayInfo := common.GetString(item, "display_info"); displayInfo != "" {
		return displayInfo
	}
	return ""
}

func meetingSearchDescription(item map[string]interface{}) string {
	if meta, ok := item["meta_data"].(map[string]interface{}); ok {
		if desc := common.GetString(meta, "description"); desc != "" {
			return desc
		}
	}
	return ""
}

// VCSearch searches historical meeting records with filters.
var VCSearch = common.Shortcut{
	Service:     "vc",
	Command:     "+search",
	Description: "Search meeting records by keyword, time range, participant, organizer, or meeting room (requires at least one filter)",
	Risk:        "read",
	Scopes:      []string{"vc:meeting.search:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "start", Desc: "start time (ISO 8601 or YYYY-MM-DD, e.g. 2026-03-24T00:00+08:00)"},
		{Name: "end", Desc: "end time (ISO 8601 or YYYY-MM-DD, e.g. 2026-03-25)"},
		{Name: "organizer-ids", Desc: "organizer open_id list, comma-separated"},
		{Name: "participant-ids", Desc: "participant open_id list, comma-separated"},
		{Name: "room-ids", Desc: "room_id list, comma-separated"},
		{Name: "page-token", Desc: "page token for next page"},
		{Name: "page-size", Default: "15", Desc: "page size, 1-30 (default 15)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, _, err := parseTimeRange(runtime); err != nil {
			return err
		}
		if q := strings.TrimSpace(runtime.Str("query")); q != "" && utf8.RuneCountInString(q) > maxVCSearchQueryLen {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--query: length must be between 1 and 50 characters").WithParam("--query")
		}
		if _, err := common.ValidatePageSizeTyped(runtime, "page-size", defaultVCSearchPageSize, 1, maxVCSearchPageSize); err != nil {
			return err
		}
		for _, flag := range []string{"query", "start", "end", "organizer-ids", "participant-ids", "room-ids"} {
			if strings.TrimSpace(runtime.Str(flag)) != "" {
				return nil
			}
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "specify at least one of --query, --start, --end, --organizer-ids, --participant-ids, or --room-ids")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		startTime, endTime, err := parseTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		params := buildSearchParams(runtime)
		dryRun := common.NewDryRunAPI().
			POST("/open-apis/vc/v1/meetings/search")
		if len(params) > 0 {
			dryRun.Params(params)
		}
		return dryRun.Body(buildSearchBody(runtime, startTime, endTime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		startTime, endTime, err := parseTimeRange(runtime)
		if err != nil {
			return err
		}
		data, err := runtime.CallAPITyped("POST", "/open-apis/vc/v1/meetings/search", buildSearchParams(runtime), buildSearchBody(runtime, startTime, endTime))
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}
		items := common.GetSlice(data, "items")
		outData := map[string]interface{}{
			"items":      items,
			"total":      data["total"],
			"has_more":   data["has_more"],
			"page_token": data["page_token"],
		}
		hasMore, _ := data["has_more"].(bool)
		runtime.OutFormat(outData, &output.Meta{Count: len(items)}, func(w io.Writer) {
			if len(items) == 0 {
				fmt.Fprintln(w, "No meetings.")
				return
			}
			var rows []map[string]interface{}
			common.EachMap(items, func(item map[string]interface{}) {
				rows = append(rows, map[string]interface{}{
					"id":           fmt.Sprintf("%v", item["id"]),
					"display_info": common.TruncateStr(meetingSearchDisplayInfo(item), 40),
					"meta_data":    common.TruncateStr(meetingSearchDescription(item), 80),
				})
			})
			output.PrintTable(w, rows)
		})
		// 非 json 格式下追加分页提示（json 格式已包含 has_more/page_token 字段）
		if hasMore && runtime.Format != "json" && runtime.Format != "" {
			pt, _ := data["page_token"].(string)
			fmt.Fprintf(runtime.IO().Out, "\n(more available, page_token: %s)\n", pt)
		}
		return nil
	},
}
