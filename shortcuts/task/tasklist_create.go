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
	"sync"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var CreateTasklist = common.Shortcut{
	Service:     "task",
	Command:     "+tasklist-create",
	Description: "create a tasklist and optionally add tasks",
	Risk:        "write",
	Scopes:      []string{"task:tasklist:write", "task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "name", Desc: "tasklist name", Required: true},
		{Name: "member", Desc: "comma-separated open_ids to add as editors"},
		{Name: "data", Desc: "JSON array of tasks to create within this tasklist"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildTasklistCreateBody(runtime)

		d := common.NewDryRunAPI().
			Desc("1. Create Tasklist").
			POST("/open-apis/task/v2/tasklists").
			Params(map[string]interface{}{"user_id_type": "open_id"}).
			Body(body)

		if dataStr := runtime.Str("data"); dataStr != "" {
			d.Desc("2. Create Tasks within the new tasklist (concurrently)")
		}

		return d
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildTasklistCreateBody(runtime)
		params := map[string]interface{}{"user_id_type": "open_id"}

		// Validate --data (client input) before any remote write, so a malformed
		// payload fails fast without creating an orphan tasklist.
		var tasks []map[string]interface{}
		if dataStr := runtime.Str("data"); dataStr != "" {
			if err := json.Unmarshal([]byte(dataStr), &tasks); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "failed to parse --data as JSON array: %v", err).WithParam("--data")
			}
		}

		data, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasklists", params, body)
		if err != nil {
			return err
		}

		tasklist, _ := data["tasklist"].(map[string]interface{})
		tasklistGuid, _ := tasklist["guid"].(string)
		tasklistName, _ := tasklist["name"].(string)
		tasklistUrl, _ := tasklist["url"].(string)
		tasklistUrl = truncateTaskURL(tasklistUrl)

		var createdTasks []map[string]interface{}
		var failedTasks []map[string]interface{}

		if len(tasks) > 0 {
			var wg sync.WaitGroup
			var mu sync.Mutex

			for i, taskDef := range tasks {
				wg.Add(1)
				go func(idx int, tDef map[string]interface{}) {
					defer func() {
						if r := recover(); r != nil {
							fmt.Fprintf(runtime.IO().ErrOut, "recovered in defer: %v\n", r)
						}
						wg.Done()
					}()

					// Add tasklist_guid to the task definition
					tDef["tasklists"] = []map[string]interface{}{
						{
							"tasklist_guid": tasklistGuid,
						},
					}

					// If assignee is provided as string, convert it to members
					if assignee, ok := tDef["assignee"].(string); ok {
						tDef["members"] = []map[string]interface{}{
							{
								"id":   assignee,
								"role": "assignee",
								"type": "user",
							},
						}
						delete(tDef, "assignee")
					}

					tData, tErr := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasks", params, tDef)

					mu.Lock()
					defer mu.Unlock()

					if tErr != nil {
						summary, _ := tDef["summary"].(string)
						failedTasks = append(failedTasks, buildTaskCreateFailure(idx, summary, tErr))
						return
					}

					if t, ok := tData["task"].(map[string]interface{}); ok {
						guid, _ := t["guid"].(string)
						urlVal, _ := t["url"].(string)
						urlVal = truncateTaskURL(urlVal)
						createdTasks = append(createdTasks, map[string]interface{}{
							"guid": guid,
							"url":  urlVal,
						})
					}
				}(i, taskDef)
			}
			wg.Wait()
		}

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"guid":          tasklistGuid,
			"url":           tasklistUrl,
			"created_tasks": createdTasks,
			"failed_tasks":  failedTasks,
		}

		pretty := func(w io.Writer) {
			fmt.Fprintf(w, "✅ Tasklist created successfully!\n")
			fmt.Fprintf(w, "Tasklist Name: %s\n", tasklistName)
			fmt.Fprintf(w, "Tasklist ID: %s\n", tasklistGuid)
			if tasklistUrl != "" {
				fmt.Fprintf(w, "Tasklist URL: %s\n", tasklistUrl)
			}

			if len(tasks) > 0 {
				fmt.Fprintln(w, strings.Repeat("-", 20))
				fmt.Fprintf(w, "Tasks created: %d/%d\n", len(createdTasks), len(tasks))
				for _, t := range createdTasks {
					guid, _ := t["guid"].(string)
					urlVal, _ := t["url"].(string)
					fmt.Fprintf(w, "  - ID: %s", guid)
					if urlVal != "" {
						fmt.Fprintf(w, ", URL: %s", urlVal)
					}
					fmt.Fprintln(w)
				}
				if len(failedTasks) > 0 {
					fmt.Fprintf(w, "\nFailed tasks:\n")
					for _, f := range failedTasks {
						fmt.Fprintf(w, "  - Index %v (%s): %s\n", f["index"], f["summary"], f["message"])
					}
				}
			}
		}

		// Sub-task creation failures surface as a non-zero exit. JSON/JQ callers
		// need an ok:false envelope, while pretty output should preserve the
		// command-specific human-readable summary.
		if len(failedTasks) > 0 {
			if runtime.Format == "pretty" && runtime.JqExpr == "" {
				runtime.OutFormat(outData, nil, pretty)
				return output.PartialFailure(output.ExitAPI)
			}
			return runtime.OutPartialFailure(outData, nil)
		}

		runtime.OutFormat(outData, nil, pretty)
		return nil
	},
}

func buildTaskCreateFailure(index int, summary string, err error) map[string]interface{} {
	failDetail := map[string]interface{}{
		"index":   index,
		"summary": summary,
	}
	if p, ok := errs.ProblemOf(err); ok {
		failDetail["type"] = string(p.Subtype)
		failDetail["code"] = p.Code
		failDetail["message"] = p.Message
		failDetail["hint"] = p.Hint
	} else {
		failDetail["type"] = "api_error"
		failDetail["message"] = err.Error()
	}
	return failDetail
}

func buildTasklistCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"name": runtime.Str("name"),
	}

	if memberStr := runtime.Str("member"); memberStr != "" {
		ids := strings.Split(memberStr, ",")
		var members []map[string]interface{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			members = append(members, map[string]interface{}{
				"id":   id,
				"role": "editor",
				"type": "user",
			})
		}
		body["members"] = members
	}

	return body
}
