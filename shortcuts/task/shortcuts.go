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
	"strings"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

func inferTaskMemberType(id string) string {
	if strings.HasPrefix(strings.TrimSpace(id), "cli_") {
		return "app"
	}
	return "user"
}

func buildTaskMember(id, role string) map[string]interface{} {
	return map[string]interface{}{
		"id":   id,
		"role": role,
		"type": inferTaskMemberType(id),
	}
}

// parseTaskTime converts a flexible time string into the Task API due/start object format.
func parseTaskTime(timeStr string) (map[string]interface{}, error) {
	var msTs string
	timeStr = strings.TrimSpace(timeStr)

	// snapDay aligns to start-of-day or end-of-day based on hint.
	snapDay := func(t time.Time) time.Time {
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}

	if isRelativeTime(timeStr) {
		t, err := parseRelativeTime(timeStr)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(timeStr, "d") || strings.HasSuffix(timeStr, "w") {
			msTs = fmt.Sprintf("%d", snapDay(t).Unix()*1000)
		} else {
			msTs = fmt.Sprintf("%d", t.Unix()*1000)
		}
	} else {
		parsedTs, err := common.ParseTime(timeStr)
		if err != nil {
			return nil, err
		}
		var sec int64
		fmt.Sscanf(parsedTs, "%d", &sec)
		msTs = fmt.Sprintf("%d", sec*1000)
	}

	// Determine if it's an all-day event based on the input format
	isAllDay := false
	// YYYY-MM-DD or relative like +2d typically mean all-day
	if len(timeStr) == 10 && strings.Count(timeStr, "-") == 2 {
		isAllDay = true
	} else if strings.HasPrefix(timeStr, "+") && (strings.HasSuffix(timeStr, "d") || strings.HasSuffix(timeStr, "w")) {
		isAllDay = true
	}

	return map[string]interface{}{
		"timestamp":  msTs,
		"is_all_day": isAllDay,
	}, nil
}

// extractTasklistGuid extracts the GUID from an applink URL or returns the string if it's already an ID.
func extractTasklistGuid(input string) string {
	input = strings.TrimSpace(input)
	if strings.HasPrefix(input, "http") {
		u, err := url.Parse(input)
		if err == nil {
			guid := u.Query().Get("guid")
			if guid != "" {
				return guid
			}
		}
	}
	return input
}

func buildTaskCreateBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	body := make(map[string]interface{})

	// Handle generic JSON payload if provided
	if dataStr := runtime.Str("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &body); err != nil {
			return nil, fmt.Errorf("--data must be a valid JSON object: %v", err)
		}
	}

	// Explicit flags override generic data
	if summary := runtime.Str("summary"); summary != "" {
		body["summary"] = summary
	}

	if desc := runtime.Str("description"); desc != "" {
		body["description"] = desc
	}

	var members []map[string]interface{}
	if assignee := runtime.Str("assignee"); assignee != "" {
		members = append(members, buildTaskMember(assignee, "assignee"))
	}
	if follower := runtime.Str("follower"); follower != "" {
		members = append(members, buildTaskMember(follower, "follower"))
	}
	if len(members) > 0 {
		body["members"] = members
	}

	if tasklistId := runtime.Str("tasklist-id"); tasklistId != "" {
		guid := extractTasklistGuid(tasklistId)
		body["tasklists"] = []map[string]interface{}{
			{
				"tasklist_guid": guid,
			},
		}
	}

	if dueStr := runtime.Str("due"); dueStr != "" {
		dueObj, err := parseTaskTime(dueStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse due time: %v", err)
		}
		body["due"] = dueObj
	}

	if idempotencyKey := runtime.Str("idempotency-key"); idempotencyKey != "" {
		body["client_token"] = idempotencyKey
	}

	summary, _ := body["summary"].(string)
	if strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("task summary is required")
	}

	return body, nil
}

var CreateTask = common.Shortcut{
	Service:     "task",
	Command:     "+create",
	Description: "create a task",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "summary", Desc: "task title"},
		{Name: "description", Desc: "task description"},
		{Name: "assignee", Desc: "task assignee id added during create; use open_id (ou_xxx) when assignee is user, use app id (cli_xxx) when assignee is app"},
		{Name: "follower", Desc: "task follower id added during create; use open_id (ou_xxx) when follower is user, use app id (cli_xxx) when follower is app"},
		{Name: "due", Desc: "due date (ISO 8601 / date:YYYY-MM-DD / relative:+2d / ms timestamp)"},
		{Name: "tasklist-id", Desc: "tasklist id or applink URL"},
		{Name: "idempotency-key", Desc: "client token for idempotency"},
		{Name: "data", Desc: "JSON payload for creating task"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildTaskCreateBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/tasks").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body, err := buildTaskCreateBody(runtime)
		if err != nil {
			return WrapTaskError(ErrCodeTaskInvalidParams, err.Error(), "create task")
		}

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  http.MethodPost,
			ApiPath:     "/open-apis/task/v2/tasks",
			QueryParams: queryParams,
			Body:        body,
		})

		var result map[string]interface{}
		if err == nil {
			if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
				return fmt.Errorf("failed to parse response: %v", parseErr)
			}
		}

		data, err := HandleTaskApiResult(result, err, "create task")
		if err != nil {
			return err
		}

		task, _ := data["task"].(map[string]interface{})
		guid, _ := task["guid"].(string)
		urlVal, _ := task["url"].(string)
		urlVal = truncateTaskURL(urlVal)

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"guid": guid,
			"url":  urlVal,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Task created successfully!\n")
			fmt.Fprintf(w, "Summary: %s\n", body["summary"])
			if guid != "" {
				fmt.Fprintf(w, "Task ID: %s\n", guid)
			}
			if urlVal != "" {
				fmt.Fprintf(w, "Task URL: %s\n", urlVal)
			}
		})
		return nil
	},
}

// Shortcuts returns all shortcuts for task and tasklist domain.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		CreateTask,
		UpdateTask,
		SetAncestorTask,
		CommentTask,
		CompleteTask,
		ReopenTask,
		AssignTask,
		FollowersTask,
		ReminderTask,
		GetMyTasks,
		GetRelatedTasks,
		SearchTask,
		SubscribeTaskEvent,
		CreateTasklist,
		SearchTasklist,
		AddTaskToTasklist,
		MembersTasklist,
	}
}
