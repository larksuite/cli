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

// dbSyncTargetTableMissingCode is the db-sync "target table does not exist" code.
const dbSyncTargetTableMissingCode = 400002483

// dbSyncUpdateTargetTableMissingHint replaces the shared 400002483 guidance for
// update. That guidance is written for create — switch target.table.action to
// create, rerun preview — but update never creates the table and has no preview.
// An update runs in the environment the task was created in, and the server
// message names that environment, so the table has to exist there. The online
// environment of a multi-environment app forbids direct DDL (k_dl_4000001), so
// there the table is created in dev and published instead.
const dbSyncUpdateTargetTableMissingHint = "The target table does not exist in the environment this sync task belongs to (the one named in the error message; an update always runs in the environment the task was created in). " +
	"Create or restore the table in that environment, for example with +db-execute --environment <that environment>, or point the config's target table at a table that already exists there, then resubmit with +db-sync-update. " +
	"If that is the online environment of a multi-environment app, tables cannot be created there directly: create the table in dev, then publish it to online with +db-env-diff and +db-env-migrate after the user confirms. " +
	"Do not change target.table.action and do not rerun preview."

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
	}, dbEnvFlags("", []string{"dev", "online"}, "compatibility flag; db-sync update ignores this value and updates the task in its existing environment")...),
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
		_, err := parseDBSyncConfigFlag(rctx.Str("config"), true, false)
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		config, _ := parseDBSyncConfigFlag(rctx.Str("config"), true, false)
		return common.NewDryRunAPI().
			PUT(appDbSyncUpdatePath(appID)).
			Desc("Update Base data sync task").
			Body(dbSyncUpdateBody(rctx, strings.TrimSpace(rctx.Str("task-id")), config))
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
		config, err := parseDBSyncConfigFlag(rctx.Str("config"), true, false)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("PUT", appDbSyncUpdatePath(appID), nil, dbSyncUpdateBody(rctx, taskID, config))
		if err != nil {
			return withDBSyncUpdateHint(err)
		}
		outputDBSyncTaskSummary(rctx, data, "Updated")
		return nil
	},
}

func dbSyncUpdateBody(_ *common.RuntimeContext, taskID string, config map[string]interface{}) map[string]interface{} {
	// The update request contract has no env field. Keep --environment as a
	// compatibility flag, but never send it or imply that it selects a branch.
	return map[string]interface{}{"task_id": taskID, "config": config}
}

// withDBSyncUpdateHint is withDBSyncHint with the update-specific answer to a
// missing target table; every other failure keeps the shared db-sync guidance.
func withDBSyncUpdateHint(err error) error {
	if p, ok := errs.ProblemOf(err); ok && p.Code == dbSyncTargetTableMissingCode &&
		p.Subtype != errs.SubtypeRateLimit && strings.TrimSpace(p.Hint) == "" {
		p.Hint = dbSyncUpdateTargetTableMissingHint
		return err
	}
	return withDBSyncHint(err, dbSyncUpdateFallbackHint)
}
