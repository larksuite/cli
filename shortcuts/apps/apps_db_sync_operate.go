// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbSyncOperateHint = "verify --task-id with `lark-cli apps +db-sync-get --app-id <app_id> --task-id <task_id>`; enable, disable, and delete only apply to streaming tasks"

// AppsDBSyncEnable enables a disabled streaming Base data sync task.
var AppsDBSyncEnable = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-enable",
	Description: "Enable a streaming Base data sync task",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +db-sync-enable --app-id <app_id> --task-id streaming_123",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags:     dbSyncTaskFlags(),
	Validate:  validateDBSyncTaskFlags,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		taskID := strings.TrimSpace(rctx.Str("task-id"))
		return common.NewDryRunAPI().
			POST(appDbSyncActionPath(appID, "enable")).
			Desc("Enable a streaming Base data sync task").
			Body(map[string]interface{}{"task_id": taskID})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runDBSyncTaskAction(rctx, "enable", "Enabled")
	},
}

// AppsDBSyncDisable disables an active streaming Base data sync task.
var AppsDBSyncDisable = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-disable",
	Description: "Disable a streaming Base data sync task",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +db-sync-disable --app-id <app_id> --task-id streaming_123",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags:     dbSyncTaskFlags(),
	Validate:  validateDBSyncTaskFlags,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		taskID := strings.TrimSpace(rctx.Str("task-id"))
		return common.NewDryRunAPI().
			POST(appDbSyncActionPath(appID, "disable")).
			Desc("Disable a streaming Base data sync task").
			Body(map[string]interface{}{"task_id": taskID})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		return runDBSyncTaskAction(rctx, "disable", "Disabled")
	},
}

// AppsDBSyncDelete stops and deletes a streaming Base data sync task while keeping target data.
var AppsDBSyncDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-delete",
	Description: "Delete a streaming Base data sync task while keeping target data",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-sync-delete --app-id <app_id> --task-id streaming_123 --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags:     dbSyncTaskFlags(),
	Validate:  validateDBSyncTaskFlags,
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		taskID := strings.TrimSpace(rctx.Str("task-id"))
		return common.NewDryRunAPI().
			POST(appDbSyncDeletePath(appID)).
			Desc("Delete a streaming Base data sync task").
			Body(map[string]interface{}{"task_id": taskID})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		taskID := strings.TrimSpace(rctx.Str("task-id"))
		data, err := rctx.CallAPITyped("POST", appDbSyncDeletePath(appID), nil, map[string]interface{}{"task_id": taskID})
		if err != nil {
			return withDBSyncHint(err, dbSyncOperateHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Deleted sync task %s\n", dashIfEmpty(common.GetString(data, "task_id")))
		})
		return nil
	},
}

func dbSyncTaskFlags() []common.Flag {
	return []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "task-id", Desc: "sync task id returned by +db-sync-create or +db-sync-list, such as streaming_123", Required: true},
	}
}

func validateDBSyncTaskFlags(ctx context.Context, rctx *common.RuntimeContext) error {
	if _, err := requireAppID(rctx.Str("app-id")); err != nil {
		return err
	}
	if strings.TrimSpace(rctx.Str("task-id")) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
	}
	return nil
}

func runDBSyncTaskAction(rctx *common.RuntimeContext, action, verb string) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	taskID := strings.TrimSpace(rctx.Str("task-id"))
	data, err := rctx.CallAPITyped("POST", appDbSyncActionPath(appID, action), nil, map[string]interface{}{"task_id": taskID})
	if err != nil {
		return withDBSyncHint(err, dbSyncOperateHint)
	}
	rctx.OutFormat(data, nil, func(w io.Writer) {
		taskID := common.GetString(data, "task_id")
		status := common.GetString(data, "status")
		if status == "" {
			fmt.Fprintf(w, "%s sync task %s\n", verb, dashIfEmpty(taskID))
			return
		}
		fmt.Fprintf(w, "%s sync task %s (%s)\n", verb, dashIfEmpty(taskID), status)
	})
	return nil
}
