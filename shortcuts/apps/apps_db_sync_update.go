// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbSyncUpdateFallbackHint = "fix the config field_maps or target table, then resubmit with +db-sync-update --yes"

// AppsDBSyncUpdate updates an existing Base data sync task configuration.
var AppsDBSyncUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-update",
	Description: "Update a Base data sync task configuration",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-sync-update --app-id <app_id> --task-id streaming_<id> --config @sync.json --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "task-id", Desc: "sync task id returned by +db-sync-create or +db-sync-list", Required: true},
		{Name: "config", Desc: "sync config JSON object, inline or via @file/-", Required: true, Input: []string{common.File, common.Stdin}},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		if strings.TrimSpace(rctx.Str("task-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
		}
		_, err := parseDBSyncConfigFlag(rctx.Str("config"), true)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		config, _ := parseDBSyncConfigFlag(rctx.Str("config"), true)
		return common.NewDryRunAPI().
			PUT(appDbSyncUpdatePath(appID)).
			Desc("Update Base data sync task").
			Params(dbEnvParams(rctx, map[string]interface{}{"task_id": strings.TrimSpace(rctx.Str("task-id"))})).
			Body(dbSyncUpdateBody(config))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		taskID := strings.TrimSpace(rctx.Str("task-id"))
		if taskID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--task-id is required").WithParam("--task-id")
		}
		config, err := parseDBSyncConfigFlag(rctx.Str("config"), true)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("PUT", appDbSyncUpdatePath(appID), dbEnvParams(rctx, map[string]interface{}{"task_id": taskID}), dbSyncUpdateBody(config))
		if err != nil {
			return withDBSyncHint(err, dbSyncUpdateFallbackHint)
		}
		outputDBSyncTaskSummary(rctx, data, "Updated")
		return nil
	},
}

func dbSyncUpdateBody(config map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"config": config}
}
