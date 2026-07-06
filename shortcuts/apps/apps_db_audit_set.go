// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// 审计保留期合法取值。
var auditRetentions = []string{"7d", "30d", "180d", "360d", "forever"}

const dbAuditSetHint = "verify --app-id and --table; check current config with `lark-cli apps +db-audit-status --app-id <app_id>`"

// AppsDBAuditEnable 为某张表开启行级审计（变更追溯）。
//
// POST /apps/{app_id}/db/audit_set，body {table, enabled:true, retention}。--retention 默认 7d。
var AppsDBAuditEnable = common.Shortcut{
	Service:     appsService,
	Command:     "+db-audit-enable",
	Description: "Enable row-change audit logging for a table",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +db-audit-enable --app-id <app_id> --table orders --retention 30d",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "table to enable audit for", Required: true},
		{Name: "retention", Default: "7d", Enum: auditRetentions, Desc: "how long to keep audit logs"},
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
			POST(appAuditSetPath(appID)).
			Desc("Enable table audit").
			Params(dbEnvParams(rctx, map[string]interface{}{})).
			Body(map[string]interface{}{"table": strings.TrimSpace(rctx.Str("table")), "enabled": true, "retention": rctx.Str("retention")})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		table := strings.TrimSpace(rctx.Str("table"))
		retention := rctx.Str("retention")
		stop := rctx.StartSpinner("Enabling audit logging for " + table)
		defer stop()
		data, err := rctx.CallAPITyped("POST", appAuditSetPath(appID),
			dbEnvParams(rctx, map[string]interface{}{}),
			map[string]interface{}{"table": table, "enabled": true, "retention": retention})
		stop()
		if err != nil {
			return withAppsHint(err, dbAuditSetHint)
		}
		st := auditSetStatus(data, table)
		ret := common.GetString(st, "retention")
		if ret == "" {
			ret = retention
		}
		out := map[string]interface{}{"table": common.GetString(st, "table"), "enabled": true, "retention": ret}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Audit enabled for table '%s' (retention: %s)\n", common.GetString(out, "table"), ret)
		})
		return nil
	},
}

// AppsDBAuditDisable 关闭某张表的行级审计。
//
// POST /apps/{app_id}/db/audit_set，body {table, enabled:false}。
var AppsDBAuditDisable = common.Shortcut{
	Service:     appsService,
	Command:     "+db-audit-disable",
	Description: "Disable row-change audit logging for a table",
	Risk:        "write",
	Tips: []string{
		"Example: lark-cli apps +db-audit-disable --app-id <app_id> --table orders",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "table", Desc: "table to disable audit for", Required: true},
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
			POST(appAuditSetPath(appID)).
			Desc("Disable table audit").
			Params(dbEnvParams(rctx, map[string]interface{}{})).
			Body(map[string]interface{}{"table": strings.TrimSpace(rctx.Str("table")), "enabled": false})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		table := strings.TrimSpace(rctx.Str("table"))
		data, err := rctx.CallAPITyped("POST", appAuditSetPath(appID),
			dbEnvParams(rctx, map[string]interface{}{}),
			map[string]interface{}{"table": table, "enabled": false})
		if err != nil {
			return withAppsHint(err, dbAuditSetHint)
		}
		st := auditSetStatus(data, table)
		out := map[string]interface{}{"table": common.GetString(st, "table"), "enabled": false}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✓ Audit disabled for table '%s'\n", common.GetString(out, "table"))
		})
		return nil
	},
}

// auditSetStatus 取响应里的 status 对象（缺失时用入参 table 兜底）。
func auditSetStatus(data map[string]interface{}, table string) map[string]interface{} {
	if st, ok := data["status"].(map[string]interface{}); ok {
		if common.GetString(st, "table") == "" {
			st["table"] = table
		}
		return st
	}
	return map[string]interface{}{"table": table}
}
