// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/shortcuts/common"
)

// GetMyTasks lists tasks assigned to the current user.
var GetMyTasks = common.Shortcut{
	Service:     "task",
	Command:     "+get-my-tasks",
	Description: "List tasks assigned to me",
	Risk:        "read",
	Scopes:      []string{"task:task:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "query", Desc: "search for tasks by summary (exact match first, then partial match)"},
		{Name: "complete", Type: "bool", Desc: "if true, query completed tasks; default is false"},
		{Name: "created_at", Desc: "query tasks created after this time (date/relative/ms)"},
		{Name: "due-start", Desc: "query tasks with due date after this time (date/relative/ms)"},
		{Name: "due-end", Desc: "query tasks with due date before this time (date/relative/ms)"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (max 40)"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max page limit (default 20, max 40 with --page-all)"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()

		params := map[string]interface{}{
			"type":         "my_tasks",
			"user_id_type": "open_id",
			"page_size":    50,
		}
		if runtime.Cmd.Flags().Changed("complete") {
			params["completed"] = runtime.Bool("complete")
		}

		return d.GET("/open-apis/task/v2/tasks").Params(params)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		startTime := time.Now()

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("type", "my_tasks")
		queryParams.Set("user_id_type", "open_id")
		queryParams.Set("page_size", "50")
		if runtime.Cmd.Flags().Changed("complete") {
			if runtime.Bool("complete") {
				queryParams.Set("completed", "true")
			} else {
				queryParams.Set("completed", "false")
			}
		}

		// parse time flags to ms timestamp if provided
		var createdAfterMs, dueStartMs, dueEndMs int64
		if createdStr := runtime.Str("created_at"); createdStr != "" {
			tStr, err := parseTimeFlagSec(createdStr, "start")
			if err != nil {
				return WrapTaskError(ErrCodeTaskInvalidParams, fmt.Sprintf("invalid created_at: %v", err), "parse created_at")
			}
			createdAfterMs, _ = strconv.ParseInt(tStr, 10, 64)
			createdAfterMs *= 1000 // Convert sec to ms
		}

		if dueStartStr := runtime.Str("due-start"); dueStartStr != "" {
			tStr, err := parseTimeFlagSec(dueStartStr, "start")
			if err != nil {
				return WrapTaskError(ErrCodeTaskInvalidParams, fmt.Sprintf("invalid due-start: %v", err), "parse due-start")
			}
			dueStartMs, _ = strconv.ParseInt(tStr, 10, 64)
			dueStartMs *= 1000
		}

		if dueEndStr := runtime.Str("due-end"); dueEndStr != "" {
			tStr, err := parseTimeFlagSec(dueEndStr, "end")
			if err != nil {
				return WrapTaskError(ErrCodeTaskInvalidParams, fmt.Sprintf("invalid due-end: %v", err), "parse due-end")
			}
			dueEndMs, _ = strconv.ParseInt(tStr, 10, 64)
			dueEndMs *= 1000
		}

		var allItems []interface{}
		var lastPageToken string
		var lastHasMore bool
		pageCount := 0
		pageLimit := runtime.Int("page-limit")
		if runtime.Bool("page-all") {
			pageLimit = 40
		}

		for {
			pageCount++
			apiReq := &larkcore.ApiReq{
				HttpMethod:  "GET",
				ApiPath:     "/open-apis/task/v2/tasks",
				QueryParams: queryParams,
			}

			apiResp, err := runtime.DoAPI(apiReq)

			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse my tasks")
				}
			}

			data, err := HandleTaskApiResult(result, err, "list tasks")
			if err != nil {
				return err
			}

			itemsRaw, _ := data["items"].([]interface{})
			allItems = append(allItems, itemsRaw...)

			hasMore, _ := data["has_more"].(bool)
			lastHasMore = hasMore
			lastPageToken, _ = data["page_token"].(string)

			if !hasMore || lastPageToken == "" {
				break
			}

			if pageCount >= pageLimit {
				break
			}

			// Set page_token for next iteration
			queryParams.Set("page_token", lastPageToken)
		}

		var filteredItems []map[string]interface{}

		for _, itemRaw := range allItems {
			item, ok := itemRaw.(map[string]interface{})
			if !ok {
				continue
			}

			// Apply client-side filtering for created_at, due-start, due-end
			// because the API might not support these filters natively in GET /v2/tasks
			if createdAfterMs > 0 {
				createdAtStr, _ := item["created_at"].(string)
				createdAtMs, _ := strconv.ParseInt(createdAtStr, 10, 64)
				if createdAtMs < createdAfterMs {
					continue
				}
			}

			if dueStartMs > 0 || dueEndMs > 0 {
				dueObj, _ := item["due"].(map[string]interface{})
				if dueObj == nil {
					// If due filtering is requested but task has no due date, filter it out
					continue
				}
				dueTimeStr, _ := dueObj["timestamp"].(string)
				dueTimeMs, _ := strconv.ParseInt(dueTimeStr, 10, 64)

				if dueStartMs > 0 && dueTimeMs < dueStartMs {
					continue
				}
				if dueEndMs > 0 && dueTimeMs > dueEndMs {
					continue
				}
			}

			filteredItems = append(filteredItems, item)
		}

		// Apply query filtering if provided
		if query := runtime.Str("query"); query != "" {
			var exactMatches []map[string]interface{}
			var partialMatches []map[string]interface{}
			for _, item := range filteredItems {
				summary, _ := item["summary"].(string)
				if summary == query {
					exactMatches = append(exactMatches, item)
				} else if strings.Contains(summary, query) {
					partialMatches = append(partialMatches, item)
				}
			}

			if len(exactMatches) > 0 {
				filteredItems = exactMatches
			} else {
				filteredItems = partialMatches
			}
		}

		var outputItems []interface{}
		for _, item := range filteredItems {
			urlVal, _ := item["url"].(string)
			urlVal = truncateTaskURL(urlVal)
			outputItem := map[string]interface{}{
				"guid":    item["guid"],
				"summary": item["summary"],
				"url":     urlVal,
			}
			if createdAtStr, ok := item["created_at"].(string); ok {
				if ts, err := strconv.ParseInt(createdAtStr, 10, 64); err == nil {
					outputItem["created_at"] = time.UnixMilli(ts).Local().Format(time.RFC3339)
				}
			}
			if dueObj, ok := item["due"].(map[string]interface{}); ok {
				if tsStr, ok := dueObj["timestamp"].(string); ok {
					if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
						outputItem["due_at"] = time.UnixMilli(ts).Local().Format(time.RFC3339)
					}
				}
			}
			outputItems = append(outputItems, outputItem)
		}

		outData := map[string]interface{}{
			"items":      outputItems,
			"page_token": lastPageToken,
			"has_more":   lastHasMore,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			if len(filteredItems) == 0 {
				fmt.Fprintln(w, "No tasks found.")
				return
			}

			for i, item := range filteredItems {
				guid, _ := item["guid"].(string)
				summary, _ := item["summary"].(string)
				urlVal, _ := item["url"].(string)
				urlVal = truncateTaskURL(urlVal)

				var dueTimeStr string
				if dueObj, ok := item["due"].(map[string]interface{}); ok {
					if tsStr, ok := dueObj["timestamp"].(string); ok {
						if ts, err := strconv.ParseInt(tsStr, 10, 64); err == nil {
							dueTimeStr = time.UnixMilli(ts).Local().Format("2006-01-02 15:04")
						}
					}
				}

				var createdDateStr string
				if createdStr, ok := item["created_at"].(string); ok {
					if ts, err := strconv.ParseInt(createdStr, 10, 64); err == nil {
						createdDateStr = time.UnixMilli(ts).Local().Format("2006-01-02")
					}
				}

				fmt.Fprintf(w, "[%d] %s\n", i+1, summary)
				fmt.Fprintf(w, "    ID: %s\n", guid)
				if urlVal != "" {
					fmt.Fprintf(w, "    URL: %s\n", urlVal)
				}
				if dueTimeStr != "" {
					fmt.Fprintf(w, "    Due: %s\n", dueTimeStr)
				}
				if createdDateStr != "" {
					fmt.Fprintf(w, "    Created: %s\n", createdDateStr)
				}
				fmt.Fprintln(w)
			}

			if lastHasMore && lastPageToken != "" && !runtime.Cmd.Flags().Changed("page-limit") && !runtime.Cmd.Flags().Changed("page-all") {
				fmt.Fprintf(w, "\n[Warning] Too many tasks! Stopped after fetching %d pages.\n", pageLimit)
			}

			fmt.Fprintf(w, "\nTotal execution time: %v\n", time.Since(startTime))
		})

		return nil
	},
}
