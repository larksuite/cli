// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/larksuite/cli/shortcuts/common"
)

// CompleteTask marks a task as complete and skips the PATCH call if already completed.
var CompleteTask = common.Shortcut{
	Service:     "task",
	Command:     "+complete",
	Description: "mark a task as complete",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task id", Required: true},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildCompleteBody()
		taskId := url.PathEscape(runtime.Str("task-id"))
		return common.NewDryRunAPI().
			GET("/open-apis/task/v2/tasks/" + taskId).
			Desc("get current task status").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			PATCH("/open-apis/task/v2/tasks/" + taskId).
			Desc("complete task if not completed").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskId := url.PathEscape(runtime.Str("task-id"))

		params := map[string]interface{}{"user_id_type": "open_id"}

		var data map[string]interface{}

		// 1. Get current task status
		getData, err := callTaskAPITyped(runtime, http.MethodGet, "/open-apis/task/v2/tasks/"+taskId, params, nil)
		if err != nil {
			return err
		}

		taskData, _ := getData["task"].(map[string]interface{})
		completedAtStr, _ := taskData["completed_at"].(string)

		// 2. If already completed, directly return success
		if completedAtStr != "" && completedAtStr != "0" {
			data = getData
		} else {
			// 3. Complete the task
			body := buildCompleteBody()
			data, err = callTaskAPITyped(runtime, http.MethodPatch, "/open-apis/task/v2/tasks/"+taskId, params, body)
			if err != nil {
				return err
			}
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
			summary, _ := task["summary"].(string)
			fmt.Fprintf(w, "✅ Task completed successfully!\n")
			if guid != "" {
				fmt.Fprintf(w, "Task ID: %s\n", guid)
			}
			if summary != "" {
				fmt.Fprintf(w, "Summary: %s\n", summary)
			}
			if urlVal != "" {
				fmt.Fprintf(w, "Task URL: %s\n", urlVal)
			}
		})
		return nil
	},
}

func buildCompleteBody() map[string]interface{} {
	completedAt := fmt.Sprintf("%d", time.Now().Unix()*1000)
	return map[string]interface{}{
		"task": map[string]interface{}{
			"completed_at": completedAt,
		},
		"update_fields": []string{"completed_at"},
	}
}
