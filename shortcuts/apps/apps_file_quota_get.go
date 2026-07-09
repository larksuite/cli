// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsFileQuotaGet reports an app's file-storage usage（动词对齐 +db-quota-get）。
//
// GET /apps/{app_id}/storage/file_quota。storage_quota_bytes / usage_percent 在配额未对接（=0）时
// 不输出（json 删字段、pretty 只打已用量）。
var AppsFileQuotaGet = common.Shortcut{
	Service:     appsService,
	Command:     "+file-quota-get",
	Description: "Get an app's file-storage usage",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +file-quota-get --app-id <app_id>",
		"Tip: get just the usage percent with -q '.usage_percent'",
	},
	Scopes:    []string{"spark:app:read"},
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
		return common.NewDryRunAPI().
			GET(appFileQuotaPath(appID)).
			Desc("Get Miaoda app file-storage usage")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("GET", appFileQuotaPath(appID), nil, nil)
		if err != nil {
			return err
		}
		out := projectFileQuota(data)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderFileQuotaPretty(w, out)
		})
		return nil
	},
}

// projectFileQuota 白名单投影 file quota 字段：只保留 agent 需要的 storage_used_bytes / files，
// 配额已对接时再加 storage_quota_bytes / usage_percent。不透传后端其它字段，避免无用字段消耗上下文。
func projectFileQuota(data map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{"storage_used_bytes": data["storage_used_bytes"]}
	if v, ok := data["files"]; ok {
		out["files"] = v
	}
	// 配额未对接（storage_quota_bytes=0/缺失）时不输出 quota / usage_percent，避免误导。
	if q, ok := numericAsFloat(data["storage_quota_bytes"]); ok && q > 0 {
		out["storage_quota_bytes"] = data["storage_quota_bytes"]
		if v, ok := data["usage_percent"]; ok {
			out["usage_percent"] = v
		}
	}
	return out
}

// renderFileQuotaPretty 打 Storage（已用 / 配额 (百分比)）与 Files 行（标签对齐 miaoda-cli）。
func renderFileQuotaPretty(w io.Writer, data map[string]interface{}) {
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
	if f, ok := numericAsFloat(data["files"]); ok {
		pairs = append(pairs, [2]string{"Files", fmt.Sprintf("%d", int64(f))})
	}
	renderKeyValuePairs(w, pairs)
}
