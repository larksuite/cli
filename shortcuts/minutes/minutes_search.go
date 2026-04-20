// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	defaultMinutesSearchPageSize = 15
	maxMinutesSearchPageSize     = 30
	maxMinutesSearchQueryLen     = 50
)

// parseTimeRange normalizes --start and --end into RFC3339 timestamps.
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
			return "", "", output.ErrValidation("--start: %v", err)
		}
		startTime = parsed
	}
	if end != "" {
		parsed, err := toRFC3339(end, "end")
		if err != nil {
			return "", "", output.ErrValidation("--end: %v", err)
		}
		endTime = parsed
	}
	if startTime != "" && endTime != "" {
		st, err := time.Parse(time.RFC3339, startTime)
		if err != nil {
			return "", "", fmt.Errorf("parse normalized --start: %w", err)
		}
		et, err := time.Parse(time.RFC3339, endTime)
		if err != nil {
			return "", "", fmt.Errorf("parse normalized --end: %w", err)
		}
		if st.After(et) {
			return "", "", output.ErrValidation("--start (%s) is after --end (%s)", start, end)
		}
	}
	return startTime, endTime, nil
}

// toRFC3339 converts a supported CLI time input into an RFC3339 timestamp.
func toRFC3339(input string, hint ...string) (string, error) {
	ts, err := common.ParseTime(input, hint...)
	if err != nil {
		return "", err
	}
	sec, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp %q: %w", ts, err)
	}
	return time.Unix(sec, 0).Format(time.RFC3339), nil
}

// resolveUserIDs expands special user identifiers and removes duplicates.
func resolveUserIDs(flagName string, ids []string, runtime *common.RuntimeContext) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	currentUserID := runtime.UserOpenId()
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.EqualFold(id, "me") {
			if currentUserID == "" {
				return nil, output.ErrValidation("%s: \"me\" requires a logged-in user with a resolvable open_id", flagName)
			}
			id = currentUserID
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// buildTimeFilter builds the create_time filter block for the API request.
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

// buildMinutesSearchFilter builds the filter object for the API request body.
func buildMinutesSearchFilter(runtime *common.RuntimeContext, startTime, endTime string) (map[string]interface{}, error) {
	filter := map[string]interface{}{}

	ownerIDs, err := resolveUserIDs("--owner-ids", common.SplitCSV(runtime.Str("owner-ids")), runtime)
	if err != nil {
		return nil, err
	}
	if len(ownerIDs) > 0 {
		filter["owner_ids"] = ownerIDs
	}

	participantIDs, err := resolveUserIDs("--participant-ids", common.SplitCSV(runtime.Str("participant-ids")), runtime)
	if err != nil {
		return nil, err
	}
	if len(participantIDs) > 0 {
		filter["participant_ids"] = participantIDs
	}

	if timeRange := buildTimeFilter(startTime, endTime); timeRange != nil {
		filter["create_time"] = timeRange
	}

	if len(filter) == 0 {
		return nil, nil
	}
	return filter, nil
}

// buildMinutesSearchBody builds the POST body for the minutes search API.
func buildMinutesSearchBody(runtime *common.RuntimeContext, startTime, endTime string) (map[string]interface{}, error) {
	body := map[string]interface{}{}

	if q := strings.TrimSpace(runtime.Str("query")); q != "" {
		body["query"] = q
	}

	filter, err := buildMinutesSearchFilter(runtime, startTime, endTime)
	if err != nil {
		return nil, err
	}
	if filter != nil {
		body["filter"] = filter
	}

	return body, nil
}

// buildMinutesSearchParams builds the query parameters for the search request.
func buildMinutesSearchParams(runtime *common.RuntimeContext) map[string]interface{} {
	params := map[string]interface{}{}

	pageSize := strings.TrimSpace(runtime.Str("page-size"))
	if pageSize == "" {
		pageSize = fmt.Sprintf("%d", defaultMinutesSearchPageSize)
	}
	params["page_size"] = pageSize

	if pageToken := strings.TrimSpace(runtime.Str("page-token")); pageToken != "" {
		params["page_token"] = pageToken
	}

	return params
}

// minuteSearchItems extracts the result items from the API response payload.
func minuteSearchItems(data map[string]interface{}) []interface{} {
	return common.GetSlice(data, "items")
}

// minuteSearchToken extracts the minute token from a search result item.
func minuteSearchToken(item map[string]interface{}) string {
	return common.GetString(item, "token")
}

// minuteSearchDisplayInfo extracts the display_info field from a search result item.
func minuteSearchDisplayInfo(item map[string]interface{}) string {
	return common.GetString(item, "display_info")
}

// minuteSearchDescription extracts the description field from a search result item.
func minuteSearchDescription(item map[string]interface{}) string {
	meta := common.GetMap(item, "meta_data")
	return common.GetString(meta, "description")
}

// minuteSearchAppLink extracts the app link from a search result item.
func minuteSearchAppLink(item map[string]interface{}) string {
	meta := common.GetMap(item, "meta_data")
	return common.GetString(meta, "app_link")
}

// minuteSearchAvatar extracts the avatar URL from a search result item.
func minuteSearchAvatar(item map[string]interface{}) string {
	meta := common.GetMap(item, "meta_data")
	return common.GetString(meta, "avatar")
}

// buildMinuteSearchRows converts API items into pretty output rows.
func buildMinuteSearchRows(items []interface{}) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]interface{})
		if item == nil {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"token":        minuteSearchToken(item),
			"display_info": common.TruncateStr(minuteSearchDisplayInfo(item), 40),
			"description":  common.TruncateStr(minuteSearchDescription(item), 40),
			"app_link":     common.TruncateStr(minuteSearchAppLink(item), 80),
			"avatar":       common.TruncateStr(minuteSearchAvatar(item), 80),
		})
	}
	return rows
}

// MinutesSearch searches minutes by keyword, owners, participants, and time range.
var MinutesSearch = common.Shortcut{
	Service:     "minutes",
	Command:     "+search",
	Description: "Search minutes by keyword, owners, participants, and time range",
	Risk:        "read",
	Scopes:      []string{"minutes:minutes.search:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "owner-ids", Desc: "owner open_id list, comma-separated (use \"me\" for current user)"},
		{Name: "participant-ids", Desc: "participant open_id list, comma-separated (use \"me\" for current user)"},
		{Name: "start", Desc: "time lower bound (ISO 8601 or YYYY-MM-DD)"},
		{Name: "end", Desc: "time upper bound (ISO 8601 or YYYY-MM-DD)"},
		{Name: "page-token", Desc: "page token for next page"},
		{Name: "page-size", Default: "15", Desc: "page size, 1-30 (default 15)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, _, err := parseTimeRange(runtime); err != nil {
			return err
		}
		if q := strings.TrimSpace(runtime.Str("query")); q != "" && utf8.RuneCountInString(q) > maxMinutesSearchQueryLen {
			return output.ErrValidation("--query: length must be between 1 and 50 characters")
		}
		if _, err := common.ValidatePageSize(runtime, "page-size", defaultMinutesSearchPageSize, 1, maxMinutesSearchPageSize); err != nil {
			return err
		}
		ownerIDs, err := resolveUserIDs("--owner-ids", common.SplitCSV(runtime.Str("owner-ids")), runtime)
		if err != nil {
			return err
		}
		for _, id := range ownerIDs {
			if _, err := common.ValidateUserID(id); err != nil {
				return err
			}
		}
		participantIDs, err := resolveUserIDs("--participant-ids", common.SplitCSV(runtime.Str("participant-ids")), runtime)
		if err != nil {
			return err
		}
		for _, id := range participantIDs {
			if _, err := common.ValidateUserID(id); err != nil {
				return err
			}
		}
		for _, flag := range []string{"query", "owner-ids", "participant-ids", "start", "end"} {
			if strings.TrimSpace(runtime.Str(flag)) != "" {
				return nil
			}
		}
		return common.FlagErrorf("specify at least one of --query, --owner-ids, --participant-ids, --start, or --end")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		startTime, endTime, err := parseTimeRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		params := buildMinutesSearchParams(runtime)
		body, err := buildMinutesSearchBody(runtime, startTime, endTime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dryRun := common.NewDryRunAPI().
			POST("/open-apis/minutes/v1/minutes/search")
		if len(params) > 0 {
			dryRun.Params(params)
		}
		return dryRun.Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		startTime, endTime, err := parseTimeRange(runtime)
		if err != nil {
			return err
		}
		body, err := buildMinutesSearchBody(runtime, startTime, endTime)
		if err != nil {
			return err
		}

		data, err := runtime.CallAPI(http.MethodPost, "/open-apis/minutes/v1/minutes/search", buildMinutesSearchParams(runtime), body)
		if err != nil {
			return err
		}
		if data == nil {
			data = map[string]interface{}{}
		}

		items := minuteSearchItems(data)
		hasMore, _ := data["has_more"].(bool)
		pageToken, _ := data["page_token"].(string)
		rows := buildMinuteSearchRows(items)

		outData := map[string]interface{}{
			"items":      items,
			"total":      data["total"],
			"has_more":   data["has_more"],
			"page_token": data["page_token"],
		}

		runtime.OutFormat(outData, &output.Meta{Count: len(rows)}, func(w io.Writer) {
			if len(rows) == 0 {
				fmt.Fprintln(w, "No minutes.")
				return
			}
			output.PrintTable(w, rows)
		})
		if hasMore && runtime.Format != "json" && runtime.Format != "" {
			fmt.Fprintf(runtime.IO().Out, "\n(more available, page_token: %s)\n", pageToken)
		}
		return nil
	},
}
