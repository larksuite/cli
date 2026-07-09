// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsDBAuditStatus 查看数据表的审计开关状态（哪些表开了行级审计、保留期）。
//
// GET /apps/{app_id}/db/audit_status。--table 指定单表（无记录时占位 enabled=false）；
// 不指定返回所有已配置表。json 单表返对象、多表返数组；pretty 单表 key/value、多表表格。
var AppsDBAuditStatus = common.Shortcut{
	Service:     appsService,
	Command:     "+db-audit-status",
	Description: "Show table audit (row-change tracking) status",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-audit-status --app-id <app_id>",
		"Check one table: --table orders",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "show status for a single table (default: all configured tables)"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		return rejectLegacyEnvFlag(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appAuditStatusPath(appID)).
			Desc("Get table audit status").
			Params(buildAuditStatusParams(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appAuditStatusPath(appID), buildAuditStatusParams(rctx), nil)
		if err != nil {
			return withAppsHint(err, dbChangelogHint)
		}
		table := strings.TrimSpace(rctx.Str("table"))
		items := projectAuditStatusItems(data["items"])
		// 单表查询但后端无记录 → 占位 enabled=false（与 miaoda 一致）。
		if table != "" && len(items) == 0 {
			items = []map[string]interface{}{{"table": table, "enabled": false}}
		}
		// json：单表返对象、多表返数组。
		var out interface{}
		if table != "" && len(items) == 1 {
			out = items[0]
		} else {
			out = map[string]interface{}{"items": items}
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderAuditStatusPretty(w, items, table)
		})
		return nil
	},
}

// buildAuditStatusParams 组装 audit_status 查询参数：env 及可选 table（单表查询）。
func buildAuditStatusParams(rctx *common.RuntimeContext) map[string]interface{} {
	params := dbEnvParams(rctx, map[string]interface{}{})
	if t := strings.TrimSpace(rctx.Str("table")); t != "" {
		params["table"] = t
	}
	return params
}

// projectAuditStatusItems 透出 {table, enabled, enabled_at?, retention?}。
func projectAuditStatusItems(raw interface{}) []map[string]interface{} {
	arr, _ := raw.([]interface{})
	out := make([]map[string]interface{}, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		row := map[string]interface{}{
			"table":   common.GetString(m, "table"),
			"enabled": m["enabled"] == true,
		}
		if v := common.GetString(m, "enabled_at"); v != "" {
			row["enabled_at"] = v
		}
		if v := common.GetString(m, "retention"); v != "" {
			row["retention"] = v
		}
		out = append(out, row)
	}
	return out
}

// renderAuditStatusPretty 单表渲染 key/value、多表渲染对齐表格（table/enabled/enabled_at/retention）。
func renderAuditStatusPretty(w io.Writer, items []map[string]interface{}, table string) {
	if len(items) == 0 {
		io.WriteString(w, "No audit configuration found.\n")
		return
	}
	yesNo := func(m map[string]interface{}) string {
		if m["enabled"] == true {
			return "yes"
		}
		return "no"
	}
	get := func(m map[string]interface{}, k string) string { return dashIfEmpty(common.GetString(m, k)) }
	// 单表 → key/value
	if table != "" && len(items) == 1 {
		it := items[0]
		renderKeyValuePairs(w, [][2]string{
			{"table", common.GetString(it, "table")},
			{"enabled", yesNo(it)},
			{"enabled_at", get(it, "enabled_at")},
			{"retention", get(it, "retention")},
		})
		return
	}
	// 多表 → 表格
	headers := []string{"table", "enabled", "enabled_at", "retention"}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, []string{common.GetString(it, "table"), yesNo(it), get(it, "enabled_at"), get(it, "retention")})
	}
	renderAlignedTable(w, headers, rows)
}
