// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/larksuite/cli/shortcuts/common"
)

var ReopenTask = common.Shortcut{
	Service:     "task",
	Command:     "+reopen",
	Description: "reopen a completed task",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task id", Required: true},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildReopenBody()
		taskId := url.PathEscape(runtime.Str("task-id"))
		return common.NewDryRunAPI().
			PATCH("/open-apis/task/v2/tasks/" + taskId).
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskId := url.PathEscape(runtime.Str("task-id"))
		body := buildReopenBody()

		params := map[string]interface{}{"user_id_type": "open_id"}
		data, err := callTaskAPITyped(runtime, http.MethodPatch, "/open-apis/task/v2/tasks/"+taskId, params, body)
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
			summary, _ := task["summary"].(string)
			fmt.Fprintf(w, "✅ Task reopened successfully!\n")
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

func buildReopenBody() map[string]interface{} {
	return map[string]interface{}{
		"task": map[string]interface{}{
			"completed_at": "0",
		},
		"update_fields": []string{"completed_at"},
	}
}
