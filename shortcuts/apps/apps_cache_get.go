// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsCacheGet reads a single business cache key's value + metadata.
//
// GET /apps/{app_id}/cache?env=&key=。value 在 wire 上是 JSON 字符串透传：--format json
// 原样输出该字符串（不反序列化），--format pretty 反序列化后缩进展开。value_size_bytes 由 CLI
// 按 value 字节长度算出（端点不返回）；未命中（exists=false）时不带 value，ttl_ms/value_size_bytes 为 null。
var AppsCacheGet = common.Shortcut{
	Service:     appsService,
	Command:     "+cache-get",
	Description: "Get a business cache key's value and metadata",
	Risk:        common.RiskRead,
	Tips: []string{
		"Example: lark-cli apps +cache-get --app-id <app_id> --key spotbonus:2026:winners:list:v1",
		"Example: lark-cli apps +cache-get --app-id <app_id> --environment online --key <key>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "key", Desc: "business cache key", Required: true},
		cacheEnvFlag(),
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		_, err := requireAppID(rctx.Str("app-id"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(appCachePath(appID)).
			Desc("Get a Miaoda app runtime cache key").
			Params(dbEnvParams(rctx, map[string]interface{}{"key": rctx.Str("key")}))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		key := rctx.Str("key")
		data, err := rctx.CallAPITyped("GET", appCachePath(appID), dbEnvParams(rctx, map[string]interface{}{"key": key}), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := projectCacheGet(data, key, rctx)
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderCacheGetPretty(w, out)
		})
		return nil
	},
}

// projectCacheGet 组装 cache-get 输出：key 回显、environment 取 resolved env、exists 直读；
// 命中时带 ttl_ms + value（原始串）+ value_size_bytes（CLI 算），未命中时 ttl_ms/value_size_bytes 为 null、无 value。
func projectCacheGet(data map[string]interface{}, key string, rctx *common.RuntimeContext) map[string]interface{} {
	exists := cacheBool(data["exists"])
	out := map[string]interface{}{
		"key":         key,
		"environment": resolvedEnv(data, rctx),
		"exists":      exists,
	}
	if exists {
		val := common.GetString(data, "value")
		out["ttl_ms"] = cacheInt(data["ttl_ms"])
		out["value_size_bytes"] = len([]byte(val))
		out["value"] = val
	} else {
		out["ttl_ms"] = nil
		out["value_size_bytes"] = nil
	}
	return out
}

// renderCacheGetPretty 打元信息块（key/environment/exists，命中再加 ttl/value_size），命中时末尾展开 value。
func renderCacheGetPretty(w io.Writer, out map[string]interface{}) {
	exists, _ := out["exists"].(bool)
	pairs := [][2]string{
		{"key", common.GetString(out, "key")},
		{"environment", common.GetString(out, "environment")},
		{"exists", fmt.Sprintf("%v", exists)},
	}
	if exists {
		pairs = append(pairs,
			[2]string{"ttl", formatCacheTTL(out["ttl_ms"])},
			[2]string{"value_size", humanBytes(out["value_size_bytes"])},
		)
	}
	renderKeyValuePairs(w, pairs)
	if exists {
		fmt.Fprintln(w, "value:")
		printCacheValuePretty(w, common.GetString(out, "value"))
	}
}
