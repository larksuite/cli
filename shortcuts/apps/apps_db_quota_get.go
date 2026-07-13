// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsDBQuotaGet reports an app's database storage usage and object counts.
//
// GET /apps/{app_id}/db/quota。storage_quota_bytes / usage_percent 在配额未对接（=0）时
// 不输出（与 +file-quota-get 一致）；tables / views 始终输出。
var AppsDBQuotaGet = common.Shortcut{
	Service:     appsService,
	Command:     "+db-quota-get",
	Description: "Get an app's database storage usage",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +db-quota-get --app-id <app_id>",
		"Example: lark-cli apps +db-quota-get --app-id <app_id> --environment dev",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
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
			GET(appDbQuotaPath(appID)).
			Desc("Get Miaoda app database storage usage").
			Params(dbEnvParams(rctx, map[string]interface{}{}))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appDbQuotaPath(appID), dbEnvParams(rctx, map[string]interface{}{}), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := projectDbQuota(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderDbQuotaPretty(w, out)
		})
		return nil
	},
}

// projectDbQuota 白名单投影 db quota 字段：只保留 storage_used_bytes / tables / views，
// 配额已对接时再加 storage_quota_bytes / usage_percent。不透传后端其它字段，避免无用字段消耗上下文。
func projectDbQuota(data map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{"storage_used_bytes": data["storage_used_bytes"]}
	for _, k := range []string{"tables", "views"} {
		if v, ok := data[k]; ok {
			out[k] = v
		}
	}
	// 配额未对接（storage_quota_bytes=0/缺失）时不输出 quota / usage_percent。
	if q, ok := numericAsFloat(data["storage_quota_bytes"]); ok && q > 0 {
		out["storage_quota_bytes"] = data["storage_quota_bytes"]
		if v, ok := data["usage_percent"]; ok {
			out["usage_percent"] = v
		}
	}
	return out
}

// renderDbQuotaPretty 打 Storage（已用 / 配额 (百分比)）与 Tables / Views 行（标签对齐 miaoda-cli）。
func renderDbQuotaPretty(w io.Writer, data map[string]interface{}) {
	used := humanBytes(data["storage_used_bytes"])
	usage := used
	if q, ok := numericAsFloat(data["storage_quota_bytes"]); ok && q > 0 {
		pct := ""
		if p, ok := numericAsFloat(data["usage_percent"]); ok {
			pct = fmt.Sprintf(" (%.1f%%)", p)
		}
		usage = fmt.Sprintf("%s / %s%s", used, humanBytes(data["storage_quota_bytes"]), pct)
	}
	pairs := [][2]string{{"Storage", usage}}
	if f, ok := numericAsFloat(data["tables"]); ok {
		pairs = append(pairs, [2]string{"Tables", fmt.Sprintf("%d", int64(f))})
	}
	if f, ok := numericAsFloat(data["views"]); ok {
		pairs = append(pairs, [2]string{"Views", fmt.Sprintf("%d", int64(f))})
	}
	renderKeyValuePairs(w, pairs)
}
