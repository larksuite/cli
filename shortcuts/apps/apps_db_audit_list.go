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

// AppsDBAuditList 列出数据表的行级审计事件（INSERT/UPDATE/DELETE 的变更追溯）。
//
// GET /apps/{app_id}/db/audit_list（cursor 分页）。--table 可重复传多张表；--since/--until 多格式时间。
// operator 透传 {id,name}（json 还原对象、pretty 取 name）；before/after 是条件出现的 JSON
// （INSERT 无 before、DELETE 无 after），json 还原成对象。
//
// 多表查询时，CLI 先用 schema（表是否存在）+ status（审计是否开启）在本地过滤，把不存在 /
// 未开启审计的表剔除后再查 audit_list，被剔除的表及原因放进 skipped（服务端不再返该字段）。
var AppsDBAuditList = common.Shortcut{
	Service:     appsService,
	Command:     "+db-audit-list",
	Description: "List row-change audit events for one or more tables (cursor pagination)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-audit-list --app-id <app_id> --table orders",
		"Multiple tables: repeat --table; filter time with --since 7d / --until 2026-04-15.",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Type: "string_slice", Desc: "table(s) to list audit events for (repeatable)", Required: true},
		{Name: "since", Desc: "filter: event at or after; relative (7d/2h) | date | datetime | ISO 8601 w/ TZ (bare date/datetime read in local timezone)"},
		{Name: "until", Desc: "filter: event at or before; same formats as --since"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size"},
		{Name: "page-token", Desc: "pagination cursor from previous response"},
	}, dbEnvFlags("", []string{"dev", "online"}, "target db environment; leave unset to auto-select (multi-env app uses dev, single-env uses online), or pass dev/online")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		if err := rejectLegacyEnvFlag(rctx); err != nil {
			return err
		}
		if len(auditListTables(rctx)) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--table is required (at least one table)").WithParam("--table")
		}
		return normalizeTimeFlags(rctx, "since", "until")
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appAuditListPath(appID)).
			Desc("List Miaoda app table audit events").
			Params(buildAuditListParams(rctx, auditListTables(rctx)))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		requested := auditListTables(rctx)
		env := dbEnv(rctx)

		// 多表查询：CLI 侧先用 schema（表是否存在）+ status（审计是否开启）过滤，
		// 不存在 / 未开启审计的表不进 audit_list 查询，单独在 skipped 里给出原因。
		// 单表查询直接打 audit_list，由后端就 table-not-found / audit-not-enabled 报错。
		queryTables := requested
		var skipped []auditSkippedEntry
		if len(requested) > 1 {
			queryTables, skipped, err = filterAuditTables(rctx, appID, env, requested)
			if err != nil {
				return withAppsHint(err, dbChangelogHint)
			}
			// 所有请求表都被过滤掉 → 无可查询表，直接返回空 + skipped 提示，不调 audit_list。
			if len(queryTables) == 0 {
				out := map[string]interface{}{"items": []auditLogItem{}, "has_more": false, "skipped": skipped}
				rctx.OutFormat(out, nil, func(w io.Writer) {
					io.WriteString(w, "No audit events found.\n")
					writeAuditSkipped(w, skipped, len(requested))
				})
				return nil
			}
		}

		data, err := rctx.CallAPITyped("GET", appAuditListPath(appID), buildAuditListParams(rctx, queryTables), nil)
		if err != nil {
			return withAppsHint(err, dbChangelogHint)
		}
		items := projectAuditLogItems(data["items"])
		data["items"] = items
		// 服务端不再返 skipped；改由 CLI 算出的 skipped 写回输出。
		if len(skipped) > 0 {
			data["skipped"] = skipped
		} else {
			delete(data, "skipped")
		}
		multi := len(requested) > 1
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderAuditListPretty(w, items, skipped, len(requested), multi)
		})
		return nil
	},
}

// auditSkippedEntry 是被 CLI 预过滤掉的表及原因（替代已删除的服务端 skipped 字段）。
type auditSkippedEntry struct {
	Table  string `json:"table"`
	Reason string `json:"reason"`
}

// filterAuditTables 用 schema（存在性）+ status（审计开关）把请求表分成「可查询」与「跳过」两组。
func filterAuditTables(rctx *common.RuntimeContext, appID, env string, requested []string) ([]string, []auditSkippedEntry, error) {
	existing, err := fetchExistingTables(rctx, appID, env)
	if err != nil {
		return nil, nil, err
	}
	enabled, err := fetchAuditEnabledTables(rctx, appID, env)
	if err != nil {
		return nil, nil, err
	}
	valid := make([]string, 0, len(requested))
	var skipped []auditSkippedEntry
	for _, t := range requested {
		switch {
		case !existing[t]:
			skipped = append(skipped, auditSkippedEntry{Table: t, Reason: "table not found"})
		case !enabled[t]:
			skipped = append(skipped, auditSkippedEntry{Table: t, Reason: "audit not enabled"})
		default:
			valid = append(valid, t)
		}
	}
	return valid, skipped, nil
}

// fetchExistingTables 翻页拉全量表清单，返回存在表名集合（schema 命令同源接口）。
func fetchExistingTables(rctx *common.RuntimeContext, appID, env string) (map[string]bool, error) {
	existing := map[string]bool{}
	token := ""
	for {
		params := map[string]interface{}{"page_size": 100}
		if env != "" {
			params["env"] = env
		}
		if token != "" {
			params["page_token"] = token
		}
		data, err := rctx.CallAPITyped("GET", appTablesPath(appID), params, nil)
		if err != nil {
			return nil, err
		}
		for _, it := range asMapSlice(data["items"]) {
			if name := common.GetString(it, "name"); name != "" {
				existing[name] = true
			}
		}
		token = common.GetString(data, "page_token")
		if data["has_more"] != true || token == "" {
			break
		}
	}
	return existing, nil
}

// fetchAuditEnabledTables 拉审计状态，返回当前已开启审计的表名集合（status 命令同源接口）。
func fetchAuditEnabledTables(rctx *common.RuntimeContext, appID, env string) (map[string]bool, error) {
	statusParams := map[string]interface{}{}
	if env != "" {
		statusParams["env"] = env
	}
	data, err := rctx.CallAPITyped("GET", appAuditStatusPath(appID), statusParams, nil)
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, it := range asMapSlice(data["items"]) {
		if it["enabled"] == true {
			if name := common.GetString(it, "table"); name != "" {
				enabled[name] = true
			}
		}
	}
	return enabled, nil
}

// asMapSlice 把 interface{}（[]interface{}）里的每个 map 元素取出，非 map 丢弃。
func asMapSlice(raw interface{}) []map[string]interface{} {
	arr, _ := raw.([]interface{})
	out := make([]map[string]interface{}, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// auditListTables 取 --table 切片，trim 去空。
func auditListTables(rctx *common.RuntimeContext) []string {
	out := make([]string, 0)
	for _, t := range rctx.StrSlice("table") {
		if v := strings.TrimSpace(t); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// buildAuditListParams 组装 audit_list 查询参数：env / tables(逗号拼接) / page_size 及可选 since/until/page_token。
func buildAuditListParams(rctx *common.RuntimeContext, tables []string) map[string]interface{} {
	params := dbEnvParams(rctx, map[string]interface{}{
		"tables":    strings.Join(tables, ","),
		"page_size": rctx.Int("page-size"),
	})
	addStr := func(flag, key string) {
		if v := strings.TrimSpace(rctx.Str(flag)); v != "" {
			params[key] = v
		}
	}
	addStr("since", "since")
	addStr("until", "until")
	addStr("page-token", "page_token")
	return params
}

type auditLogItem struct {
	EventID     string       `json:"event_id"`
	EventTime   string       `json:"event_time"`
	TargetTable string       `json:"target_table"`
	Type        string       `json:"type"`
	Operator    *operatorRef `json:"operator,omitempty"`
	Summary     string       `json:"summary"`
	Before      interface{}  `json:"before,omitempty"`
	After       interface{}  `json:"after,omitempty"`
}

// projectAuditLogItems 把服务端原始审计事件投影为白名单 auditLogItem（operator 解析、before/after 还原成对象）。
func projectAuditLogItems(raw interface{}) []auditLogItem {
	arr, _ := raw.([]interface{})
	out := make([]auditLogItem, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		row := auditLogItem{
			EventID:     common.GetString(m, "event_id"),
			EventTime:   common.GetString(m, "event_time"),
			TargetTable: common.GetString(m, "target_table"),
			Type:        common.GetString(m, "type"),
			Operator:    parseOperator(common.GetString(m, "operator")),
			Summary:     common.GetString(m, "summary"),
		}
		// before/after 条件出现：INSERT 无 before、DELETE 无 after。JSON 字符串 → 还原对象。
		if b := common.GetString(m, "before"); b != "" {
			row.Before = safeParseJSON(b)
		}
		if a := common.GetString(m, "after"); a != "" {
			row.After = safeParseJSON(a)
		}
		out = append(out, row)
	}
	return out
}

// renderAuditListPretty 单表 5 列 / 多表 6 列（首列 target_table）；末尾列出 skipped 表。
func renderAuditListPretty(w io.Writer, items []auditLogItem, skipped []auditSkippedEntry, totalRequested int, multi bool) {
	if len(items) == 0 {
		io.WriteString(w, "No audit events found.\n")
		writeAuditSkipped(w, skipped, totalRequested)
		return
	}
	var headers []string
	if multi {
		headers = []string{"target_table", "event_time", "type", "event_id", "operator", "summary"}
	} else {
		headers = []string{"event_time", "type", "event_id", "operator", "summary"}
	}
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		cells := []string{dashIfEmpty(it.EventTime), it.Type, it.EventID, operatorName(it.Operator), dashIfEmpty(it.Summary)}
		if multi {
			cells = append([]string{dashIfEmpty(it.TargetTable)}, cells...)
		}
		rows = append(rows, cells)
	}
	renderAlignedTable(w, headers, rows)
	writeAuditSkipped(w, skipped, totalRequested)
}

// writeAuditSkipped 打 "— Skipped N of M tables: orders (audit not enabled), foo (table not found)"。
func writeAuditSkipped(w io.Writer, skipped []auditSkippedEntry, totalRequested int) {
	if len(skipped) == 0 {
		return
	}
	parts := make([]string, 0, len(skipped))
	for _, s := range skipped {
		parts = append(parts, fmt.Sprintf("%s (%s)", s.Table, s.Reason))
	}
	fmt.Fprintf(w, "— Skipped %d of %d tables: %s\n", len(skipped), totalRequested, strings.Join(parts, ", "))
}
