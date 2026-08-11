// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbSyncFallbackHint = "rerun +db-sync-create --preview after fixing the config, then submit the resolved config with --yes"

// AppsDBSyncCreate previews or creates a Base-to-database sync task.
var AppsDBSyncCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-create",
	Description: "Preview or create a Base data sync task",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-sync-create --app-id <app_id> --config @sync.json --preview --output resolved.json",
		"Example: lark-cli apps +db-sync-create --app-id <app_id> --config @resolved.json --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "config", Desc: "sync config JSON object, inline or via @file/-", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "preview", Type: "bool", Desc: "preview and resolve sync config without creating a task"},
		{Name: "output", Desc: "relative path for writing resolved preview config"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to use online, or pass dev/online explicitly")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		cfg, err := parseDBSyncConfigFlag(rctx.Str("config"), !rctx.Bool("preview"), true)
		if err != nil {
			return err
		}
		if err := requireDBSyncSourceTableIdentifiable(cfg); err != nil {
			return err
		}
		return rejectOutputTraversal(rctx.Str("output"))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		config, _ := parseDBSyncConfigFlag(rctx.Str("config"), !rctx.Bool("preview"), true)
		return common.NewDryRunAPI().
			POST(appDbSyncCreatePath(appID)).
			Desc("Preview or create Base data sync task").
			Body(dbSyncCreateBody(rctx, config, rctx.Bool("preview")))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		config, err := parseDBSyncConfigFlag(rctx.Str("config"), !rctx.Bool("preview"), true)
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", appDbSyncCreatePath(appID), nil, dbSyncCreateBody(rctx, config, rctx.Bool("preview")))
		if err != nil {
			return withDBSyncHint(err, dbSyncFallbackHint)
		}
		if rctx.Bool("preview") {
			return outputDBSyncPreview(rctx, data)
		}
		outputDBSyncTaskSummary(rctx, data, "Created")
		return nil
	},
	PostMount: allowDBSyncPreviewWithoutConfirmation,
}

func dbSyncCreateBody(rctx *common.RuntimeContext, config map[string]interface{}, preview bool) map[string]interface{} {
	body := map[string]interface{}{
		"config":  config,
		"preview": preview,
	}
	// The sync_create endpoint reads env from the request body (peer of config/preview),
	// not the query string. Only attach env when the caller specified one; omitted env
	// intentionally lets the db-sync backend use its online default.
	return dbEnvBody(rctx, body)
}

func allowDBSyncPreviewWithoutConfirmation(cmd *cobra.Command) {
	preRun := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if preRun != nil {
			if err := preRun(c, args); err != nil {
				return err
			}
		}
		preview, _ := c.Flags().GetBool("preview")
		if preview {
			_ = c.Flags().Set("yes", "true")
		}
		return nil
	}
}

func outputDBSyncPreview(rctx *common.RuntimeContext, data map[string]interface{}) error {
	output := strings.TrimSpace(rctx.Str("output"))
	if output != "" {
		// The saved file is documented as a ready-to-use create input, so refuse to
		// persist a missing or non-object config rather than writing "null" and
		// exiting success.
		config, ok := data["config"].(map[string]interface{})
		if !ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "preview response has no config object to write")
		}
		configJSON, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "preview config is not JSON-serializable").WithCause(err)
		}
		configJSON = append(configJSON, '\n')
		saved, err := rctx.FileIO().Save(output, fileio.SaveOptions{
			ContentType:   "application/json",
			ContentLength: int64(len(configJSON)),
		}, bytes.NewReader(configJSON))
		if err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output: %v", err).WithParam("--output")
		}
		_ = saved
	}
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Preview sync config: mode=%s target=%s mapped_fields=%s syncable_source_fields=%s estimated_records=%s",
			dbSyncMode(data), dbSyncTargetTable(data), dbSyncSummaryValue(data, "mapped_field_count"),
			dbSyncSummaryValue(data, "syncable_source_field_count"), dbSyncSummaryValue(data, "estimated_record_count"))
		if output != "" {
			fmt.Fprintf(w, " output=%s", output)
		}
		fmt.Fprintln(w)
	})
	return nil
}

func outputDBSyncTaskSummary(rctx *common.RuntimeContext, data map[string]interface{}, verb string) {
	rctx.OutFormat(data, nil, func(w io.Writer) {
		taskID := common.GetString(data, "task_id")
		mode := common.GetString(data, "mode")
		status := common.GetString(data, "status")
		if mode != "" && status != "" {
			fmt.Fprintf(w, "%s sync task %s (%s, %s)\n", verb, taskID, mode, status)
			return
		}
		if status != "" {
			fmt.Fprintf(w, "%s sync task %s (%s)\n", verb, taskID, status)
			return
		}
		fmt.Fprintf(w, "%s sync task %s\n", verb, taskID)
	})
}

func dbSyncMode(data map[string]interface{}) string {
	if mode := common.GetString(data, "mode"); mode != "" {
		return mode
	}
	if cfg := common.GetMap(data, "config"); cfg != nil {
		return common.GetString(cfg, "mode")
	}
	return "unknown"
}

func dbSyncTargetTable(data map[string]interface{}) string {
	cfg := common.GetMap(data, "config")
	target := common.GetMap(cfg, "target")
	table := common.GetMap(target, "table")
	if name := common.GetString(table, "name"); name != "" {
		return name
	}
	return "unknown"
}

func dbSyncSummaryValue(data map[string]interface{}, key string) string {
	summary := common.GetMap(data, "summary")
	if v, ok := numericAsFloat(summary[key]); ok {
		return fmt.Sprintf("%.0f", v)
	}
	if s := common.GetString(summary, key); s != "" {
		return s
	}
	return "unknown"
}
