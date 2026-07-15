// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsCacheClear clears all cache entries for the app in the given environment.
//
// POST /apps/{app_id}/cache/clear，body {env}。清空当前应用指定环境下全部缓存，用于无法定位
// 具体 key 的快速恢复；影响面大，定 high-risk-write（框架自动注入 --yes 确认）。
var AppsCacheClear = common.Shortcut{
	Service:     appsService,
	Command:     "+cache-clear",
	Description: "Clear all cache entries for the app in the given environment",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +cache-clear --app-id <app_id> --environment dev --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		cacheEnvFlag(),
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		_, err := requireAppID(rctx.Str("app-id"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appCacheClearPath(appID)).
			Desc("Clear all cache entries for the app in the given environment").
			Body(dbEnvParams(rctx, map[string]interface{}{}))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", appCacheClearPath(appID), nil, dbEnvParams(rctx, map[string]interface{}{}))
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := map[string]interface{}{
			"environment":       resolvedEnv(data, rctx),
			"deleted_key_count": cacheInt(data["deleted_key_count"]),
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderCacheClearPretty(w, out)
		})
		return nil
	},
}

// renderCacheClearPretty 打 "✓ cache cleared: N entries (env)"。
func renderCacheClearPretty(w io.Writer, out map[string]interface{}) {
	n := int64(0)
	if f, ok := numericAsFloat(out["deleted_key_count"]); ok {
		n = int64(f)
	}
	fmt.Fprintf(w, "✓ cache cleared: %d entries (%s)\n", n, common.GetString(out, "environment"))
}
