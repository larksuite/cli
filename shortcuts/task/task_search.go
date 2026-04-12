// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	taskSearchDefaultPageLimit = 20
	taskSearchMaxPageLimit     = 40
)

var SearchTask = common.Shortcut{
	Service:     "task",
	Command:     "+search",
	Description: "search tasks",
	Risk:        "read",
	Scopes:      []string{"task:task:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (max 40)"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max page limit (default 20, max 40)"},
		{Name: "page-token", Desc: "page token"},
		{Name: "creator", Desc: "creator open_ids, comma-separated"},
		{Name: "assignee", Desc: "assignee open_ids, comma-separated"},
		{Name: "completed", Type: "bool", Desc: "set true for completed or false for incomplete tasks"},
		{Name: "due", Desc: "due time range: start,end (supports ISO/date/relative/ms)"},
		{Name: "follower", Desc: "follower open_ids, comma-separated"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildTaskSearchBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/tasks/search").
			Body(body).
			Desc("Then GET /open-apis/task/v2/tasks/:guid for each search hit to render standard output")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildTaskSearchBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildTaskSearchBody(runtime)
		if err != nil {
			return err
		}

		pageLimit := runtime.Int("page-limit")
		if pageLimit <= 0 {
			pageLimit = taskSearchDefaultPageLimit
		}
		if runtime.Bool("page-all") {
			pageLimit = taskSearchMaxPageLimit
		}
		if pageLimit > taskSearchMaxPageLimit {
			pageLimit = taskSearchMaxPageLimit
		}

		var rawItems []interface{}
		var lastPageToken string
		var lastHasMore bool
		currentBody := body
		for page := 0; page < pageLimit; page++ {
			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod: http.MethodPost,
				ApiPath:    "/open-apis/task/v2/tasks/search",
				Body:       currentBody,
			})
			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse task search")
				}
			}
			data, err := HandleTaskApiResult(result, err, "search tasks")
			if err != nil {
				return err
			}
			items, _ := data["items"].([]interface{})
			rawItems = append(rawItems, items...)
			lastHasMore, _ = data["has_more"].(bool)
			lastPageToken, _ = data["page_token"].(string)
			if !lastHasMore || lastPageToken == "" {
				break
			}
			currentBody["page_token"] = lastPageToken
		}

		enriched := make([]map[string]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			itemMap, _ := item.(map[string]interface{})
			taskID, _ := itemMap["id"].(string)
			if taskID == "" {
				continue
			}

			task, err := getTaskDetail(runtime, taskID)
			if err != nil {
				metaData, _ := itemMap["meta_data"].(map[string]interface{})
				appLink, _ := metaData["app_link"].(string)
				enriched = append(enriched, map[string]interface{}{
					"guid": taskID,
					"url":  truncateTaskURL(appLink),
				})
				continue
			}
			enriched = append(enriched, outputTaskSummary(task))
		}

		outData := map[string]interface{}{
			"items":      enriched,
			"page_token": lastPageToken,
			"has_more":   lastHasMore,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(enriched)}, func(w io.Writer) {
			if len(enriched) == 0 {
				fmt.Fprintln(w, "No tasks found.")
				return
			}
			for i, item := range enriched {
				fmt.Fprintf(w, "[%d] %v\n", i+1, item["summary"])
				fmt.Fprintf(w, "    GUID: %v\n", item["guid"])
				if created, _ := item["created_at"].(string); created != "" {
					fmt.Fprintf(w, "    Created: %s\n", created)
				}
				if dueAt, _ := item["due_at"].(string); dueAt != "" {
					fmt.Fprintf(w, "    Due: %s\n", dueAt)
				}
				if urlVal, _ := item["url"].(string); urlVal != "" {
					fmt.Fprintf(w, "    URL: %s\n", urlVal)
				}
				fmt.Fprintln(w)
			}
			if lastHasMore && lastPageToken != "" {
				fmt.Fprintf(w, "Next page token: %s\n", lastPageToken)
			}
		})
		return nil
	},
}

func buildTaskSearchBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	filter := map[string]interface{}{}

	if ids := splitAndTrimCSV(runtime.Str("creator")); len(ids) > 0 {
		filter["creator_ids"] = ids
	}
	if ids := splitAndTrimCSV(runtime.Str("assignee")); len(ids) > 0 {
		filter["assignee_ids"] = ids
	}
	if ids := splitAndTrimCSV(runtime.Str("follower")); len(ids) > 0 {
		filter["follower_ids"] = ids
	}
	if runtime.Cmd.Flags().Changed("completed") {
		filter["is_completed"] = runtime.Bool("completed")
	}
	if dueRange := runtime.Str("due"); dueRange != "" {
		start, end, err := parseTimeRangeRFC3339(dueRange)
		if err != nil {
			return nil, WrapTaskError(ErrCodeTaskInvalidParams, fmt.Sprintf("invalid due: %v", err), "build task search")
		}
		if dueFilter := buildTimeRangeFilter("due_time", start, end); dueFilter != nil {
			mergeIntoFilter(filter, dueFilter)
		}
	}
	if err := requireSearchFilter(runtime.Str("query"), filter, "build task search"); err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"query": runtime.Str("query"),
	}
	if len(filter) > 0 {
		body["filter"] = filter
	}
	if pageToken := runtime.Str("page-token"); pageToken != "" {
		body["page_token"] = pageToken
	}
	return body, nil
}

func getTaskDetail(runtime *common.RuntimeContext, taskID string) (map[string]interface{}, error) {
	queryParams := make(larkcore.QueryParams)
	queryParams.Set("user_id_type", "open_id")

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/task/v2/tasks/" + url.PathEscape(taskID),
		QueryParams: queryParams,
	})
	var result map[string]interface{}
	if err == nil {
		if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
			return nil, WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse task detail response: %v", parseErr), "parse task detail")
		}
	}
	data, err := HandleTaskApiResult(result, err, "get task detail "+taskID)
	if err != nil {
		return nil, err
	}
	task, _ := data["task"].(map[string]interface{})
	if task == nil {
		return nil, WrapTaskError(ErrCodeTaskInternalError, "task detail response missing task object", "get task detail")
	}
	return task, nil
}
