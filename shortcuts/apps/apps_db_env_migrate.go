// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const dbEnvMigrateHint = "ensure the app is multi-env (`+db-env-create`) and has pending dev changes; preview with `+db-env-diff`"

// AppsDBEnvDiff 预览 dev→online 待发布的结构变更（不落地）。
//
// POST /apps/{app_id}/db/env_migrate，body {dry_run:true}，同步返 {from,to,changes[]}。
// 与 +db-env-migrate 同端点、dry_run 区分；预览也需 spark:app:write scope。
var AppsDBEnvDiff = common.Shortcut{
	Service:     appsService,
	Command:     "+db-env-diff",
	Description: "Preview pending dev→online schema changes (no apply)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-env-diff --app-id <app_id>",
		"Apply the previewed changes with +db-env-migrate --yes.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		_, err := requireAppID(rctx.Str("app-id"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().POST(appEnvMigratePath(appID)).Desc("Preview dev→online migration").Body(map[string]interface{}{"dry_run": true})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		stop := rctx.StartSpinner("Previewing migration diff (dev → online)")
		defer stop()
		data, err := rctx.CallAPITyped("POST", appEnvMigratePath(appID), nil, map[string]interface{}{"dry_run": true})
		stop()
		if err != nil {
			return withAppsHint(err, dbEnvMigrateHint)
		}
		from, to := common.GetString(data, "from"), common.GetString(data, "to")
		changes := projectMigrationChanges(data["changes"])
		out := map[string]interface{}{"from": from, "to": to, "changes": changes}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderMigrationDiff(w, from, to, changes)
		})
		return nil
	},
}

// AppsDBEnvMigrate 把 dev 的待发布结构变更发布到 online（异步，CLI 轮询至完成）。
//
// POST /apps/{app_id}/db/env_migrate，body {dry_run:false} → task_id，轮询 env_migrate_status
// 至 success；后端 status:applied，CLI 对外统一呈现 migrated。high-risk-write。
var AppsDBEnvMigrate = common.Shortcut{
	Service:     appsService,
	Command:     "+db-env-migrate",
	Description: "Publish pending dev→online schema changes (irreversible)",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-env-migrate --app-id <app_id> --yes",
		"Preview first with +db-env-diff.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		_, err := requireAppID(rctx.Str("app-id"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().POST(appEnvMigratePath(appID)).Desc("Apply dev→online migration").Body(map[string]interface{}{"dry_run": false})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		// 先 dry_run 预览拿待发布变更数（对齐 miaoda-cli 的 diff-then-apply）：服务端在未经
		// dry_run 预热时直接 apply，虽发布成功却把 changes_applied 回填成 0（展示「Migrated (0 changes)」）。
		// 这一步既预热服务端计数、又作为 apply 仍回 0 时的兜底数。dry_run 报错（如无待发布变更）不阻断，
		// 交由下面真实 apply 统一报同样的业务错。
		pending := 0
		var previewFrom, previewTo string
		if preview, perr := rctx.CallAPITyped("POST", appEnvMigratePath(appID), nil, map[string]interface{}{"dry_run": true}); perr == nil {
			pending = len(projectMigrationChanges(preview["changes"]))
			previewFrom, previewTo = common.GetString(preview, "from"), common.GetString(preview, "to")
		}
		stop := rctx.StartSpinner("Applying migration (dev → online)")
		defer stop()
		submit, err := rctx.CallAPITyped("POST", appEnvMigratePath(appID), nil, map[string]interface{}{"dry_run": false})
		if err != nil {
			return withAppsHint(err, dbEnvMigrateHint)
		}
		from, to := common.GetString(submit, "from"), common.GetString(submit, "to")
		if from == "" {
			from = previewFrom
		}
		if to == "" {
			to = previewTo
		}
		taskID := common.GetString(submit, "task_id")
		applied := intFromAny(submit["changes_applied"])
		if applied == 0 {
			applied = len(projectMigrationChanges(submit["changes"]))
		}
		// 有 task_id → 异步，轮询至终态；无 task_id（同步完成）则直接用 submit 结果。
		if taskID != "" {
			final, perr := pollUntil(rctx.Ctx(), 1*time.Second, 2*time.Minute,
				func() (map[string]interface{}, error) {
					return rctx.CallAPITyped("GET", appEnvMigrateStatusPath(appID), map[string]interface{}{"task_id": taskID}, nil)
				},
				func(d map[string]interface{}) (bool, error) {
					switch strings.ToLower(common.GetString(d, "status")) {
					case "success", "applied", "migrated":
						return true, nil
					case "failed":
						return false, withAppsHint(errs.NewAPIError(errs.SubtypeServerError, "%s", migrateFailMsg(d, taskID)), dbEnvMigrateHint)
					}
					return false, nil
				})
			if perr != nil {
				return perr
			}
			if n := intFromAny(final["changes_applied"]); n > 0 {
				applied = n
			}
		}
		// 服务端把发布成功的变更数回 0 时，用发布前 dry_run 预览的 pending 数兜底，避免误显示「(0 changes)」。
		if applied == 0 && pending > 0 {
			applied = pending
		}
		stop() // clear spinner before printing the result
		out := map[string]interface{}{"status": "migrated", "from": from, "to": to, "changes_applied": applied}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Migrated %s → %s (%d changes)\n", from, to, applied)
		})
		return nil
	},
}

type migrationChange struct {
	Type      string `json:"type"`
	Table     string `json:"table"`
	Statement string `json:"statement"`
}

// projectMigrationChanges 把服务端原始变更项投影为白名单 migrationChange（type/table/statement）。
func projectMigrationChanges(raw interface{}) []migrationChange {
	arr, _ := raw.([]interface{})
	out := make([]migrationChange, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok {
			out = append(out, migrationChange{
				Type:      common.GetString(m, "type"),
				Table:     common.GetString(m, "table"),
				Statement: common.GetString(m, "statement"),
			})
		}
	}
	return out
}

// renderMigrationDiff 渲染 dev→online 待发布变更：无变更打提示，否则逐条打 statement。
func renderMigrationDiff(w io.Writer, from, to string, changes []migrationChange) {
	if len(changes) == 0 {
		fmt.Fprintf(w, "No pending changes from %s to %s.\n", from, to)
		return
	}
	fmt.Fprintf(w, "%s → %s (%d changes):\n\n", from, to, len(changes))
	for _, c := range changes {
		fmt.Fprintf(w, "  %s\n", c.Statement)
	}
}

// migrateFailMsg 取发布失败信息：优先服务端 error_message，缺失则用带 task_id 的兜底文案。
func migrateFailMsg(d map[string]interface{}, taskID string) string {
	if m := common.GetString(d, "error_message"); m != "" {
		return m
	}
	return fmt.Sprintf("migration apply failed (task_id=%s)", taskID)
}

// intFromAny 把 JSON number / json.Number 转 int（计数用）。
func intFromAny(v interface{}) int {
	if f, ok := numericAsFloat(v); ok {
		return int(f)
	}
	return 0
}
