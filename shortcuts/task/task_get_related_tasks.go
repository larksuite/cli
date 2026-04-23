// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	relatedTasksDefaultPageLimit = 20
	relatedTasksMaxPageLimit     = 40
	relatedTasksPageSize         = 100
)

var GetRelatedTasks = common.Shortcut{
	Service:     "task",
	Command:     "+get-related-tasks",
	Description: "list tasks related to me",
	Risk:        "read",
	Scopes:      []string{"task:task:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "include-complete", Type: "bool", Desc: "default true; set false to return only incomplete tasks"},
		{Name: "page-all", Type: "bool", Desc: "automatically paginate through all pages (max 40)"},
		{Name: "page-limit", Type: "int", Default: "20", Desc: "max page limit (default 20, max 40)"},
		{Name: "page-token", Desc: "page token / updated_at cursor in microseconds"},
		{Name: "created-by-me", Type: "bool", Desc: "client-side filter to tasks created by me; pagination still follows upstream related-task pages"},
		{Name: "followed-by-me", Type: "bool", Desc: "client-side filter to tasks followed by me; pagination still follows upstream related-task pages"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params := map[string]interface{}{
			"user_id_type": "open_id",
			"page_size":    relatedTasksPageSize,
		}
		if runtime.Cmd.Flags().Changed("include-complete") && !runtime.Bool("include-complete") {
			params["completed"] = false
		}
		if pageToken := runtime.Str("page-token"); pageToken != "" {
			params["page_token"] = pageToken
		}
		return common.NewDryRunAPI().
			GET("/open-apis/task/v2/task_v2/list_related_task").
			Params(params)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")
		queryParams.Set("page_size", fmt.Sprintf("%d", relatedTasksPageSize))
		if runtime.Cmd.Flags().Changed("include-complete") && !runtime.Bool("include-complete") {
			queryParams.Set("completed", "false")
		}
		if pageToken := runtime.Str("page-token"); pageToken != "" {
			queryParams.Set("page_token", pageToken)
		}

		pageLimit := runtime.Int("page-limit")
		if pageLimit <= 0 {
			pageLimit = relatedTasksDefaultPageLimit
		}
		if runtime.Bool("page-all") {
			pageLimit = relatedTasksMaxPageLimit
		}
		if pageLimit > relatedTasksMaxPageLimit {
			pageLimit = relatedTasksMaxPageLimit
		}

		var allItems []interface{}
		var lastPageToken string
		var lastHasMore bool
		for page := 0; page < pageLimit; page++ {
			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod:  http.MethodGet,
				ApiPath:     "/open-apis/task/v2/task_v2/list_related_task",
				QueryParams: queryParams,
			})
			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					return WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse related tasks")
				}
			}
			data, err := HandleTaskApiResult(result, err, "list related tasks")
			if err != nil {
				return err
			}

			items, _ := data["items"].([]interface{})
			allItems = append(allItems, items...)
			lastHasMore, _ = data["has_more"].(bool)
			lastPageToken, _ = data["page_token"].(string)
			if !lastHasMore || lastPageToken == "" {
				break
			}
			queryParams.Set("page_token", lastPageToken)
		}

		userOpenID := runtime.UserOpenId()
		filtered := make([]map[string]interface{}, 0, len(allItems))
		for _, item := range allItems {
			task, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if runtime.Bool("created-by-me") {
				creator, _ := task["creator"].(map[string]interface{})
				if creatorID, _ := creator["id"].(string); creatorID != userOpenID {
					continue
				}
			}
			if runtime.Bool("followed-by-me") && !taskFollowedBy(task, userOpenID) {
				continue
			}
			filtered = append(filtered, outputRelatedTask(task))
		}

		outData := map[string]interface{}{
			"items":      filtered,
			"page_token": lastPageToken,
			"has_more":   lastHasMore,
		}
		runtime.OutFormat(outData, &output.Meta{Count: len(filtered)}, func(w io.Writer) {
			if len(filtered) == 0 {
				fmt.Fprintln(w, "No related tasks found.")
				return
			}
			io.WriteString(w, renderRelatedTasksPretty(filtered, lastHasMore, lastPageToken))
		})
		return nil
	},
}

func taskFollowedBy(task map[string]interface{}, userOpenID string) bool {
	members, _ := task["members"].([]interface{})
	for _, member := range members {
		memberObj, _ := member.(map[string]interface{})
		role, _ := memberObj["role"].(string)
		id, _ := memberObj["id"].(string)
		if strings.EqualFold(role, "follower") && id == userOpenID {
			return true
		}
	}
	return false
}
