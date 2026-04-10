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
	tasklistSearchDefaultPageLimit = 20
	tasklistSearchMaxPageLimit     = 40
)

var SearchTasklist = common.Shortcut{
	Service:     "task",
	Command:     "+tasklist-search",
	Description: "search tasklists",
	Risk:        "read",
	Scopes:      []string{"task:tasklist:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "query", Desc: "search keyword"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (max 40)"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max page limit (default 20, max 40)"},
		{Name: "page-token", Desc: "page token"},
		{Name: "creator", Desc: "creator open_ids, comma-separated"},
		{Name: "create-time", Desc: "create time range: start,end (supports ISO/date/relative/ms)"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildTasklistSearchBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/tasklists/search").
			Body(body).
			Desc("Then GET /open-apis/task/v2/tasklists/:guid for each search hit to render standard output")
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := buildTasklistSearchBody(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildTasklistSearchBody(runtime)
		if err != nil {
			return err
		}

		pageLimit := runtime.Int("page-limit")
		if pageLimit <= 0 {
			pageLimit = tasklistSearchDefaultPageLimit
		}
		if runtime.Bool("page-all") {
			pageLimit = tasklistSearchMaxPageLimit
		}
		if pageLimit > tasklistSearchMaxPageLimit {
			pageLimit = tasklistSearchMaxPageLimit
		}

		var rawItems []interface{}
		var lastPageToken string
		var lastHasMore bool
		currentBody := body
		for page := 0; page < pageLimit; page++ {
			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod: http.MethodPost,
				ApiPath:    "/open-apis/task/v2/tasklists/search",
				Body:       currentBody,
			})
			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse tasklist search")
				}
			}
			data, err := HandleTaskApiResult(result, err, "search tasklists")
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

		tasklists := make([]map[string]interface{}, 0, len(rawItems))
		for _, item := range rawItems {
			itemMap, _ := item.(map[string]interface{})
			tasklistID, _ := itemMap["id"].(string)
			if tasklistID == "" {
				continue
			}

			tasklist, err := getTasklistDetail(runtime, tasklistID)
			if err != nil {
				// Keep a stable identifier and avoid rendering "<nil>" in pretty output.
				tasklists = append(tasklists, map[string]interface{}{
					"guid": tasklistID,
					"name": fmt.Sprintf("(unknown tasklist: %s)", tasklistID),
				})
				continue
			}
			urlVal, _ := tasklist["url"].(string)
			urlVal = truncateTaskURL(urlVal)
			tasklists = append(tasklists, map[string]interface{}{
				"guid":    tasklist["guid"],
				"name":    tasklist["name"],
				"url":     urlVal,
				"creator": tasklist["creator"],
			})
		}

		outData := map[string]interface{}{
			"items":      tasklists,
			"page_token": lastPageToken,
			"has_more":   lastHasMore,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(tasklists)}, func(w io.Writer) {
			if len(tasklists) == 0 {
				fmt.Fprintln(w, "No tasklists found.")
				return
			}
			for i, tasklist := range tasklists {
				fmt.Fprintf(w, "[%d] %v\n", i+1, tasklist["name"])
				fmt.Fprintf(w, "    GUID: %v\n", tasklist["guid"])
				if urlVal, _ := tasklist["url"].(string); urlVal != "" {
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

func buildTasklistSearchBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	filter := map[string]interface{}{}
	if ids := splitAndTrimCSV(runtime.Str("creator")); len(ids) > 0 {
		filter["user_id"] = ids
	}
	if createTime := runtime.Str("create-time"); createTime != "" {
		start, end, err := parseTimeRangeRFC3339(createTime)
		if err != nil {
			return nil, WrapTaskError(ErrCodeTaskInvalidParams, fmt.Sprintf("invalid create-time: %v", err), "build tasklist search")
		}
		if timeFilter := buildTimeRangeFilter("create_time", start, end); timeFilter != nil {
			mergeIntoFilter(filter, timeFilter)
		}
	}
	if err := requireSearchFilter(runtime.Str("query"), filter, "build tasklist search"); err != nil {
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

func getTasklistDetail(runtime *common.RuntimeContext, tasklistID string) (map[string]interface{}, error) {
	queryParams := make(larkcore.QueryParams)
	queryParams.Set("user_id_type", "open_id")

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod:  http.MethodGet,
		ApiPath:     "/open-apis/task/v2/tasklists/" + url.PathEscape(tasklistID),
		QueryParams: queryParams,
	})
	var result map[string]interface{}
	if err == nil {
		if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
			return nil, WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse tasklist detail response: %v", parseErr), "parse tasklist detail")
		}
	}
	data, err := HandleTaskApiResult(result, err, "get tasklist detail "+tasklistID)
	if err != nil {
		return nil, err
	}
	tasklist, _ := data["tasklist"].(map[string]interface{})
	if tasklist == nil {
		return nil, WrapTaskError(ErrCodeTaskInternalError, "tasklist detail response missing tasklist object", "get tasklist detail")
	}
	return tasklist, nil
}
