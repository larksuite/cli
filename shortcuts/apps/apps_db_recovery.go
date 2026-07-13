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

const dbRecoveryHint = "PITR window is up to 7 days back, limited by your last `+db-env-migrate`; pass --target as a time (e.g. 2h / 2026-04-15 / 2026-04-15T10:00:00Z)"

// AppsDBRecoveryDiff 预览把数据库恢复到某个时间点会带来的变更（PITR diff，不落地）。
//
// POST /apps/{app_id}/db/env_recovery，body {target, dry_run:true} → preview_request_id，
// 轮询 env_recovery_diff_status 至终态，返回受影响表与行数变化。预览也需 spark:app:write scope。
var AppsDBRecoveryDiff = common.Shortcut{
	Service:     appsService,
	Command:     "+db-recovery-diff",
	Description: "Preview restoring the database to a point in time (PITR diff)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-recovery-diff --app-id <app_id> --target 2h",
		"Apply with +db-recovery-apply --target <same> --yes.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "target", Desc: "point in time to restore to; relative (2h/3d) | date | datetime | ISO 8601 w/ TZ", Required: true},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		return normalizeTimeFlags(rctx, "target")
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().POST(appRecoveryPath(appID)).Desc("Preview PITR recovery").
			Params(dbEnvParams(rctx, map[string]interface{}{})).
			Body(map[string]interface{}{"target": rctx.Str("target"), "dry_run": true})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		target := rctx.Str("target")
		preview, err := runRecoveryPreview(rctx, appID, target)
		if err != nil {
			return err
		}
		out := recoveryDiffOutput(target, preview)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderRecoveryDiff(w, target, out)
		})
		return nil
	},
}

// AppsDBRecoveryApply 把数据库恢复到某个时间点（覆盖当前数据，异步，CLI 轮询至完成）。
//
// POST /apps/{app_id}/db/env_recovery，body {target, dry_run:false}；目标=当前态时短路 no_changes，
// 否则轮询 env_recovery_apply_status 至 success。high-risk-write。
var AppsDBRecoveryApply = common.Shortcut{
	Service:     appsService,
	Command:     "+db-recovery-apply",
	Description: "Restore the database to a point in time (overwrites current data, irreversible)",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-recovery-apply --app-id <app_id> --target 2026-04-15T10:00:00Z --yes",
		"Preview first with +db-recovery-diff.",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "target", Desc: "point in time to restore to; relative (2h/3d) | date | datetime | ISO 8601 w/ TZ", Required: true},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		return normalizeTimeFlags(rctx, "target")
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().POST(appRecoveryPath(appID)).Desc("Apply PITR recovery").
			Params(dbEnvParams(rctx, map[string]interface{}{})).
			Body(map[string]interface{}{"target": rctx.Str("target"), "dry_run": false})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		target := rctx.Str("target")
		stop := rctx.StartSpinner("Restoring database (target: " + target + ")")
		defer stop()
		submit, err := rctx.CallAPITyped("POST", appRecoveryPath(appID), dbEnvParams(rctx, map[string]interface{}{}), map[string]interface{}{"target": target, "dry_run": false})
		if err != nil {
			return withAppsHint(err, dbRecoveryHint)
		}
		// 目标=当前态 → 后端短路 no_changes，不轮询。
		if strings.ToLower(common.GetString(submit, "status")) == "no_changes" {
			stop()
			out := map[string]interface{}{"status": "no_changes", "target": target}
			rctx.OutFormat(out, nil, func(w io.Writer) {
				io.WriteString(w, "No changes — database is already at this state.\n")
			})
			return nil
		}
		final, perr := pollUntil(rctx.Ctx(), 2*time.Second, 2*time.Minute,
			func() (map[string]interface{}, error) {
				return rctx.CallAPITyped("GET", appRecoveryApplyStatusPath(appID), dbEnvParams(rctx, map[string]interface{}{}), nil)
			},
			func(d map[string]interface{}) (bool, error) {
				switch strings.ToLower(common.GetString(d, "status")) {
				case "success", "restored", "ready":
					return true, nil
				case "failed":
					msg := common.GetString(d, "error_message")
					if msg == "" {
						msg = fmt.Sprintf("recovery to %s failed", target)
					}
					return false, withAppsHint(errs.NewAPIError(errs.SubtypeServerError, "%s", msg), dbRecoveryHint)
				}
				return false, nil
			})
		if perr != nil {
			return perr
		}
		stop()
		out := map[string]interface{}{"status": "restored", "target": target}
		if n := intFromAny(final["restore_time_sec"]); n > 0 {
			out["restore_time_sec"] = n
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			if n, ok := out["restore_time_sec"].(int); ok {
				fmt.Fprintf(w, "✓ Database restored to %s (%ds elapsed)\n", target, n)
			} else {
				fmt.Fprintf(w, "✓ Database restored to %s\n", target)
			}
		})
		return nil
	},
}

// runRecoveryPreview 触发 PITR 预览（dry_run=true）拿 preview_request_id，轮询 diff_status 至终态。
func runRecoveryPreview(rctx *common.RuntimeContext, appID, target string) (map[string]interface{}, error) {
	stop := rctx.StartSpinner("Previewing recovery impact (target: " + target + ")")
	defer stop()
	submit, err := rctx.CallAPITyped("POST", appRecoveryPath(appID), dbEnvParams(rctx, map[string]interface{}{}), map[string]interface{}{"target": target, "dry_run": true})
	if err != nil {
		return nil, withAppsHint(err, dbRecoveryHint)
	}
	prid := common.GetString(submit, "preview_request_id")
	if prid == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "recovery diff did not return preview_request_id")
	}
	return pollUntil(rctx.Ctx(), 1*time.Second, 2*time.Minute,
		func() (map[string]interface{}, error) {
			return rctx.CallAPITyped("GET", appRecoveryDiffStatusPath(appID), dbEnvParams(rctx, map[string]interface{}{"preview_request_id": prid}), nil)
		},
		func(d map[string]interface{}) (bool, error) {
			switch strings.ToLower(common.GetString(d, "preview_status")) {
			case "success":
				return true, nil
			case "failed":
				msg := common.GetString(d, "error_message")
				if msg == "" {
					msg = "recovery preview failed"
				}
				return false, withAppsHint(errs.NewAPIError(errs.SubtypeServerError, "%s", msg), dbRecoveryHint)
			}
			return false, nil
		})
}

type recoveryChange struct {
	Table     string      `json:"table"`
	Inserted  interface{} `json:"inserted,omitempty"`
	Deleted   interface{} `json:"deleted,omitempty"`
	Action    string      `json:"action,omitempty"`
	DroppedAt string      `json:"dropped_at,omitempty"`
}

// recoveryDiffOutput 组装 diff 输出：target / tables_affected / changes[] / estimated_seconds。
func recoveryDiffOutput(target string, preview map[string]interface{}) map[string]interface{} {
	arr, _ := preview["changes"].([]interface{})
	raw := make([]recoveryChange, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		raw = append(raw, recoveryChange{
			Table:     common.GetString(m, "table"),
			Inserted:  m["inserted"],
			Deleted:   m["deleted"],
			Action:    common.GetString(m, "action"),
			DroppedAt: common.GetString(m, "dropped_at"),
		})
	}
	// 服务端可能对同一张表既下发 schema 动作(drop/restore/alter)、又下发纯数据行变更。
	// schema 动作已涵盖数据结果(如 drop 隐含删光行)，丢弃该表的冗余数据行那条，避免同表
	// 两行 + tables_affected 翻倍。
	hasSchema := map[string]bool{}
	for _, c := range raw {
		if c.Action != "" {
			hasSchema[c.Table] = true
		}
	}
	changes := make([]recoveryChange, 0, len(raw))
	for _, c := range raw {
		if c.Action == "" && hasSchema[c.Table] {
			continue
		}
		changes = append(changes, c)
	}
	// tables_affected 按去重后的不同表数计(而非变更条数)。
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Table] = true
	}
	est := intFromAny(preview["estimated_seconds"])
	if est == 0 {
		est = 30 // PRD 兜底
	}
	return map[string]interface{}{
		"target": target, "tables_affected": len(seen),
		"changes": changes, "estimated_seconds": est,
	}
}

// renderRecoveryDiff 渲染 PITR 恢复预览：受影响表数、逐表变化描述及预估耗时；无变更打提示。
func renderRecoveryDiff(w io.Writer, target string, out map[string]interface{}) {
	changes, _ := out["changes"].([]recoveryChange)
	if len(changes) == 0 {
		io.WriteString(w, "No changes — database is already at this state.\n")
		return
	}
	fmt.Fprintf(w, "Recovery preview (→ %s):\n\n", target)
	fmt.Fprintf(w, "  tables affected: %d\n", intFromAny(out["tables_affected"]))
	for _, c := range changes {
		fmt.Fprintf(w, "  %s: %s\n", c.Table, describeRecoveryChange(c))
	}
	fmt.Fprintf(w, "\n  estimated time: ~%ds\n", intFromAny(out["estimated_seconds"]))
}

// describeRecoveryChange：schema 动作 或 数据行变化二选一（无 modified，对齐设计）。
func describeRecoveryChange(c recoveryChange) string {
	switch c.Action {
	case "restore_table":
		return "table will be restored"
	case "drop_table":
		return "table will be dropped"
	case "alter_table":
		return "table will be altered"
	case "unavailable":
		if c.DroppedAt != "" {
			return "diff unavailable: " + c.DroppedAt
		}
		return "diff unavailable"
	}
	parts := make([]string, 0, 2)
	if n := intFromAny(c.Inserted); n != 0 {
		parts = append(parts, fmt.Sprintf("+%d rows", n))
	}
	if n := intFromAny(c.Deleted); n != 0 {
		parts = append(parts, fmt.Sprintf("-%d rows", n))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}
