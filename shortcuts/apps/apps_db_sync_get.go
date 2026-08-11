// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsDBSyncGet gets one Base data sync task.
var AppsDBSyncGet = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-get",
	Description: "Get a Base data sync task",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-sync-get --app-id <app_id> --task-id <task_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app id", Required: true},
		{Name: "task-id", Desc: "data sync task id", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if _, err := requireDBSyncTaskID(rctx.Str("task-id")); err != nil {
			return err
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		taskID, _ := requireDBSyncTaskID(rctx.Str("task-id"))
		return common.NewDryRunAPI().
			GET(appDbSyncTaskPath(appID)).
			Desc("Get Base data sync task").
			Params(map[string]interface{}{"task_id": taskID})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		taskID, err := requireDBSyncTaskID(rctx.Str("task-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appDbSyncTaskPath(appID), map[string]interface{}{"task_id": taskID}, nil)
		if err != nil {
			return withDBSyncHint(err, "verify --task-id; list tasks with `lark-cli apps +db-sync-list --app-id "+appID+"`")
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderDBSyncGetPretty(w, data)
		})
		return nil
	},
}

func requireDBSyncTaskID(raw string) (string, error) {
	taskID := strings.TrimSpace(raw)
	if taskID == "" {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
	}
	return taskID, nil
}

func renderDBSyncGetPretty(w io.Writer, data map[string]interface{}) {
	source := common.GetMap(data, "source", "table")
	target := common.GetMap(data, "target", "table")
	pairs := [][2]string{
		{"task_id", dashIfEmpty(common.GetString(data, "task_id"))},
		{"mode", dashIfEmpty(common.GetString(data, "mode"))},
		{"status", dashIfEmpty(common.GetString(data, "status"))},
		{"source", dashIfEmpty(common.GetString(source, "name"))},
		{"target", dashIfEmpty(common.GetString(target, "name"))},
		{"created_at", dashIfEmpty(common.GetString(data, "created_at"))},
	}
	if common.GetString(data, "mode") == "batch" {
		pairs = append(pairs,
			[2]string{"schema_only", dbSyncBool(data["schema_only"])},
			[2]string{"statistics", dbSyncSummary(data["statistics"])},
		)
	}
	if common.GetString(data, "mode") == "streaming" {
		pairs = append(pairs, [2]string{"last_synced_at", dashIfEmpty(common.GetString(data, "last_synced_at"))})
	}
	renderKeyValuePairs(w, pairs)
	renderDBSyncWarnings(w, common.GetSlice(data, "warnings"))
}

func renderDBSyncWarnings(w io.Writer, warnings []interface{}) {
	if len(warnings) == 0 {
		return
	}
	io.WriteString(w, "warnings:\n")
	for _, item := range warnings {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		fmt.Fprintf(w, "- code=%s message=%s target_table=%s hint=%s\n",
			dashIfEmpty(common.GetString(m, "code")),
			dashIfEmpty(common.GetString(m, "message")),
			dashIfEmpty(common.GetString(m, "target_table")),
			dashIfEmpty(common.GetString(m, "hint")),
		)
	}
}

func dbSyncBool(raw interface{}) string {
	b, ok := raw.(bool)
	if !ok {
		return "—"
	}
	if b {
		return "true"
	}
	return "false"
}

func dbSyncSummary(raw interface{}) string {
	m, ok := raw.(map[string]interface{})
	if !ok || len(m) == 0 {
		return "—"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return strings.Join(parts, " ")
}
