// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var ReminderTask = common.Shortcut{
	Service:     "task",
	Command:     "+reminder",
	Description: "manage task reminders",
	Risk:        "write",
	Scopes:      []string{"task:task:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "task-id", Desc: "task id", Required: true},
		{Name: "set", Desc: "relative fire minutes to set (e.g. 15m, 1h, 1d)"},
		{Name: "remove", Type: "bool", Desc: "removes all existing reminders"},
	},

	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("set") == "" && !runtime.Bool("remove") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "must specify either --set or --remove")
		}
		if runtime.Str("set") != "" && runtime.Bool("remove") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot specify both --set and --remove")
		}
		return nil
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		d := common.NewDryRunAPI()
		taskId := url.PathEscape(runtime.Str("task-id"))

		if runtime.Bool("remove") {
			d.Desc("1. GET task to find existing reminder IDs").
				GET("/open-apis/task/v2/tasks/" + taskId).
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Desc("2. POST to remove_reminders with found IDs")
		} else if setStr := runtime.Str("set"); setStr != "" {
			d.Desc("1. GET task to check existing reminders").
				GET("/open-apis/task/v2/tasks/" + taskId).
				Params(map[string]interface{}{"user_id_type": "open_id"}).
				Desc("2. POST to remove_reminders if any exist").
				Desc("3. POST to add_reminders").
				POST("/open-apis/task/v2/tasks/" + taskId + "/add_reminders")
		}

		return d
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		taskId := url.PathEscape(runtime.Str("task-id"))
		params := map[string]interface{}{"user_id_type": "open_id"}

		// First, get the task to find existing reminders
		data, err := callTaskAPITyped(runtime, http.MethodGet, "/open-apis/task/v2/tasks/"+taskId, params, nil)
		if err != nil {
			return err
		}

		taskObj, _ := data["task"].(map[string]interface{})
		reminders, _ := taskObj["reminders"].([]interface{})

		if runtime.Bool("remove") {
			if len(reminders) == 0 {
				runtime.OutFormat(map[string]interface{}{"guid": taskId}, nil, func(w io.Writer) {
					fmt.Fprintln(w, "No existing reminders to remove.")
				})
				return nil
			}

			var reminderIds []string
			for _, r := range reminders {
				if rMap, ok := r.(map[string]interface{}); ok {
					if id, ok := rMap["id"].(string); ok {
						reminderIds = append(reminderIds, id)
					}
				}
			}

			if len(reminderIds) > 0 {
				body := map[string]interface{}{
					"reminder_ids": reminderIds,
				}
				if _, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasks/"+taskId+"/remove_reminders", params, body); err != nil {
					return err
				}
			}
		} else if setStr := runtime.Str("set"); setStr != "" {
			// Parse relative time string (e.g. 15m, 1h, 1d, or plain 30)
			var minutes int
			var parseErr error

			if strings.HasSuffix(setStr, "m") {
				minutes, parseErr = strconv.Atoi(strings.TrimSuffix(setStr, "m"))
			} else if strings.HasSuffix(setStr, "h") {
				h, e := strconv.Atoi(strings.TrimSuffix(setStr, "h"))
				if e == nil {
					minutes = h * 60
				}
				parseErr = e
			} else if strings.HasSuffix(setStr, "d") {
				d, e := strconv.Atoi(strings.TrimSuffix(setStr, "d"))
				if e == nil {
					minutes = d * 24 * 60
				}
				parseErr = e
			} else {
				// Default to minutes if no suffix
				minutes, parseErr = strconv.Atoi(setStr)
			}

			if parseErr != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", parseErr)
			}

			// If any reminders exist, remove them first
			if len(reminders) > 0 {
				var reminderIds []string
				for _, r := range reminders {
					if rMap, ok := r.(map[string]interface{}); ok {
						if id, ok := rMap["id"].(string); ok {
							reminderIds = append(reminderIds, id)
						}
					}
				}

				if len(reminderIds) > 0 {
					body := map[string]interface{}{
						"reminder_ids": reminderIds,
					}
					if _, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasks/"+taskId+"/remove_reminders", params, body); err != nil {
						return err
					}
				}
			}

			body := map[string]interface{}{
				"reminders": []map[string]interface{}{
					{
						"relative_fire_minute": minutes,
					},
				},
			}
			if _, err := callTaskAPITyped(runtime, http.MethodPost, "/open-apis/task/v2/tasks/"+taskId+"/add_reminders", params, body); err != nil {
				return err
			}
		}

		urlVal, _ := taskObj["url"].(string)
		urlVal = truncateTaskURL(urlVal)

		// Standardized write output: return resource identifiers
		outData := map[string]interface{}{
			"guid": taskId,
			"url":  urlVal,
		}

		runtime.OutFormat(outData, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Task reminders updated successfully!\n")
			fmt.Fprintf(w, "Task ID: %s\n", taskId)
			if urlVal != "" {
				fmt.Fprintf(w, "Task URL: %s\n", urlVal)
			}
		})
		return nil
	},
}
