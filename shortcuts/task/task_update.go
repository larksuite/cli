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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var UpdateTask = common.Shortcut{
	Service:     "task",
	Command:     "+update",
	Description: "update task attributes",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task GUID or task applink URL (comma-separated for multiple)", Required: true},
		{Name: "summary", Desc: "task title"},
		{Name: "description", Desc: "task description"},
		{Name: "due", Desc: "due date (ISO 8601 / date:YYYY-MM-DD / relative:+2d / ms timestamp)"},
		{Name: "data", Desc: "JSON payload for task object"},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := parseTaskGUIDs(runtime.Str("task-id"))
		return err
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body, err := buildTaskUpdateBody(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		taskIDs, err := parseTaskGUIDs(runtime.Str("task-id"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		preview := common.NewDryRunAPI()
		for _, taskID := range taskIDs {
			preview.PATCH("/open-apis/task/v2/tasks/" + url.PathEscape(taskID)).
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Body(body)
		}
		return preview
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskIDs, err := parseTaskGUIDs(runtime.Str("task-id"))
		if err != nil {
			return err
		}

		body, err := buildTaskUpdateBody(runtime)
		if err != nil {
			// buildTaskUpdateBody already returns a typed validation error;
			// propagate it directly instead of re-wrapping as an API error.
			return err
		}

		var updatedTasks []map[string]interface{}

		for _, taskID := range taskIDs {
			params := map[string]interface{}{"user_id_type": "open_id"}
			data, err := callTaskAPITyped(runtime, http.MethodPatch, "/open-apis/task/v2/tasks/"+url.PathEscape(taskID), params, body)
			if err != nil {
				return err
			}

			taskObj, _ := data["task"].(map[string]interface{})
			if taskObj != nil {
				updatedTasks = append(updatedTasks, taskObj)
			}
		}

		updateFields, _ := body["update_fields"].([]string)
		var tasks []map[string]interface{}
		for _, task := range updatedTasks {
			guid, _ := task["guid"].(string)
			urlVal, _ := task["url"].(string)
			urlVal = truncateTaskURL(urlVal)
			confirmed := make(map[string]interface{})
			for _, field := range updateFields {
				if value, ok := task[field]; ok {
					confirmed[field] = value
				}
			}
			tasks = append(tasks, map[string]interface{}{
				"guid":      guid,
				"url":       urlVal,
				"confirmed": confirmed,
			})
		}
		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"updated_fields": updateFields,
			"tasks":          tasks,
		}

		runtime.OutFormat(outData, &output.Meta{Count: len(updatedTasks)}, func(w io.Writer) {
			for _, task := range updatedTasks {
				guid, _ := task["guid"].(string)
				summary, _ := task["summary"].(string)
				urlVal, _ := task["url"].(string)
				urlVal = truncateTaskURL(urlVal)
				fmt.Fprintf(w, "✅ Task updated successfully!\n")
				fmt.Fprintf(w, "Task ID: %s\n", guid)
				if summary != "" {
					fmt.Fprintf(w, "Summary: %s\n", summary)
				}
				if urlVal != "" {
					fmt.Fprintf(w, "Task URL: %s\n", urlVal)
				}
				fmt.Fprintln(w, strings.Repeat("-", 20))
			}
		})
		return nil
	},
}

func parseTaskGUIDs(input string) ([]string, error) {
	parts := strings.Split(input, ",")
	taskGUIDs := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		guid, err := parseTaskGUID(part)
		if err != nil {
			return nil, err
		}
		taskGUIDs = append(taskGUIDs, guid)
	}
	if len(taskGUIDs) == 0 {
		_, err := parseTaskGUID("")
		return nil, err
	}
	return taskGUIDs, nil
}

func buildTaskUpdateBody(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	taskObj := make(map[string]interface{})
	var updateFields []string

	if dataStr := runtime.Str("data"); dataStr != "" {
		if err := json.Unmarshal([]byte(dataStr), &taskObj); err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--data must be a valid JSON object: %v", err).WithParam("--data")
		}
		// If data is provided, assume keys are update fields
		for k := range taskObj {
			updateFields = append(updateFields, k)
		}
	}

	if summary := runtime.Str("summary"); summary != "" {
		taskObj["summary"] = summary
		if !contains(updateFields, "summary") {
			updateFields = append(updateFields, "summary")
		}
	}

	if desc := runtime.Str("description"); desc != "" {
		taskObj["description"] = desc
		if !contains(updateFields, "description") {
			updateFields = append(updateFields, "description")
		}
	}

	if dueStr := runtime.Str("due"); dueStr != "" {
		dueObj, err := parseTaskTime(dueStr)
		if err != nil {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "failed to parse due time: %v", err).WithParam("--due")
		}
		taskObj["due"] = dueObj
		if !contains(updateFields, "due") {
			updateFields = append(updateFields, "due")
		}
	}

	if len(updateFields) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "no fields to update")
	}

	return map[string]interface{}{
		"task":          taskObj,
		"update_fields": updateFields,
	}, nil
}
