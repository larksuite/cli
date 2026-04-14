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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var AddTaskToTasklist = common.Shortcut{
	Service:     "task",
	Command:     "+tasklist-task-add",
	Description: "add tasks to a tasklist",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "tasklist-id", Desc: "tasklist id", Required: true},
		{Name: "task-id", Desc: "task id (comma-separated for multiple)", Required: true},
		{Name: "section-guid", Desc: "section guid"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		taskIds := strings.Split(runtime.Str("task-id"), ",")
		taskId := url.PathEscape(strings.TrimSpace(taskIds[0]))

		body := map[string]interface{}{
			"tasklist_guid": extractTasklistGuid(runtime.Str("tasklist-id")),
		}

		if sectionGuid := strings.TrimSpace(runtime.Str("section-guid")); sectionGuid != "" {
			body["section_guid"] = sectionGuid
		}

		return common.NewDryRunAPI().
			POST("/open-apis/task/v2/tasks/" + taskId + "/add_tasklist").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		tasklistGuid := extractTasklistGuid(runtime.Str("tasklist-id"))
		taskIds := strings.Split(runtime.Str("task-id"), ",")

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", "open_id")

		body := map[string]interface{}{
			"tasklist_guid": tasklistGuid,
		}

		if sectionGuid := strings.TrimSpace(runtime.Str("section-guid")); sectionGuid != "" {
			body["section_guid"] = sectionGuid
		}

		var successful []map[string]interface{}
		var failed []map[string]interface{}

		for _, taskId := range taskIds {
			taskId = strings.TrimSpace(taskId)
			if taskId == "" {
				continue
			}

			apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
				HttpMethod:  http.MethodPost,
				ApiPath:     "/open-apis/task/v2/tasks/" + url.PathEscape(taskId) + "/add_tasklist",
				QueryParams: queryParams,
				Body:        body,
			})

			var result map[string]interface{}
			if err == nil {
				if parseErr := json.Unmarshal(apiResp.RawBody, &result); parseErr != nil {
					err = WrapTaskError(ErrCodeTaskInternalError, fmt.Sprintf("failed to parse response: %v", parseErr), "parse add task response")
				}
			}

			data, err := HandleTaskApiResult(result, err, "add task to tasklist")
			if err != nil {
				failDetail := map[string]interface{}{
					"guid": taskId,
				}
				if exitErr, ok := err.(*output.ExitError); ok && exitErr.Detail != nil {
					failDetail["type"] = exitErr.Detail.Type
					failDetail["code"] = exitErr.Detail.Code
					failDetail["message"] = exitErr.Detail.Message
					failDetail["hint"] = exitErr.Detail.Hint
				} else {
					failDetail["type"] = "api_error"
					failDetail["message"] = err.Error()
				}
				failed = append(failed, failDetail)
			} else {
				task, _ := data["task"].(map[string]interface{})
				guid, _ := task["guid"].(string)
				taskUrl, _ := task["url"].(string)
				taskUrl = truncateTaskURL(taskUrl)
				successful = append(successful, map[string]interface{}{
					"guid": guid,
					"url":  taskUrl,
				})
			}
		}

		// Standardized write output: return resource identifiers
		resultData := map[string]interface{}{
			"successful_tasks": successful,
			"failed_tasks":     failed,
			"tasklist_guid":    tasklistGuid,
		}

		runtime.OutFormat(resultData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Tasks added to tasklist %s!\n", tasklistGuid)
			fmt.Fprintf(w, "Successful: %d, Failed: %d\n", len(successful), len(failed))

			if len(successful) > 0 {
				fmt.Fprintln(w, "Successful Tasks:")
				for _, t := range successful {
					guid, _ := t["guid"].(string)
					taskUrl, _ := t["url"].(string)
					fmt.Fprintf(w, "  - ID: %s", guid)
					if taskUrl != "" {
						fmt.Fprintf(w, ", URL: %s", taskUrl)
					}
					fmt.Fprintln(w)
				}
			}

			if len(failed) > 0 {
				fmt.Fprintln(w, "Failed Tasks:")
				for _, f := range failed {
					fmt.Fprintf(w, "  - %s: %s\n", f["guid"], f["message"])
				}
			}
		})
		return nil
	},
}
