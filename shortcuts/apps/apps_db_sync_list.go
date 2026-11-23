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

const dbSyncListHint = "verify --app-id is correct; if targeting --environment dev, create it first with `lark-cli apps +db-env-create --app-id <app_id> --environment dev`"

// AppsDBSyncList lists Base data sync tasks under an app.
var AppsDBSyncList = common.Shortcut{
	Service:     appsService,
	Command:     "+db-sync-list",
	Description: "List Base data sync tasks for an app",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-sync-list --app-id <app_id>",
		"Example: lark-cli apps +db-sync-list --app-id <app_id> --mode streaming --status enabled",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "app id", Required: true},
		{Name: "mode", Enum: []string{"batch", "streaming"}, Desc: "filter by sync mode: batch or streaming"},
		{Name: "status", Desc: "filter by sync task status"},
		{Name: "table", Desc: "filter by source or target table name"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to use online, or pass dev/online explicitly")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		if rctx.Int("page-size") <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--page-size must be a positive integer").WithParam("--page-size")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appDbSyncListPath(appID)).
			Desc("List Base data sync tasks").
			Params(buildDBSyncListParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appDbSyncListPath(appID), buildDBSyncListParams(rctx), nil)
		if err != nil {
			return withDBSyncHint(err, dbSyncListHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderDBSyncListPretty(w, data)
		})
		return nil
	},
}

func buildDBSyncListParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := dbEnvParams(rctx, map[string]interface{}{
		"page_size": rctx.Int("page-size"),
	})
	for _, name := range []string{"mode", "status", "table"} {
		if value := strings.TrimSpace(rctx.Str(name)); value != "" {
			params[name] = value
		}
	}
	if token := strings.TrimSpace(rctx.Str("page-token")); token != "" {
		params["page_token"] = token
	}
	return params
}

func renderDBSyncListPretty(w io.Writer, data map[string]interface{}) {
	items := common.GetSlice(data, "items")
	headers := []string{"task_id", "mode", "status", "table", "created_at"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, []string{
			common.GetString(m, "task_id"),
			common.GetString(m, "mode"),
			common.GetString(m, "status"),
			dbSyncTaskTable(m),
			dashIfEmpty(common.GetString(m, "created_at")),
		})
	}
	renderAlignedTable(w, headers, rows)
	fmt.Fprint(w, common.PaginationHint(data, len(items)))
}

func dbSyncTaskTable(task map[string]interface{}) string {
	for _, path := range [][2]string{{"source", "table"}, {"target", "table"}} {
		table := common.GetMap(task, path[0], path[1])
		if name := strings.TrimSpace(common.GetString(table, "name")); name != "" {
			return name
		}
	}
	if name := strings.TrimSpace(common.GetString(task, "table")); name != "" {
		return name
	}
	return "—"
}
