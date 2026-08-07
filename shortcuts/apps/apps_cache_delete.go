// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// AppsCacheDelete deletes a single business cache key (idempotent).
//
// DELETE /apps/{app_id}/cache?env=&key=。缓存是派生数据、删单 key 影响面小且可重建，
// 故定 write（非 high-risk-write、不需 --yes）。目标不存在按幂等成功处理（deleted_key_count=0）。
var AppsCacheDelete = common.Shortcut{
	Service:     appsService,
	Command:     "+cache-delete",
	Description: "Delete a single business cache key (idempotent)",
	Risk:        common.RiskWrite,
	Tips: []string{
		"Example: lark-cli apps +cache-delete --app-id <app_id> --environment dev --key <key>",
	},
	Scopes:    []string{"spark:app:write"},
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
			DELETE(appCachePath(appID)).
			Desc("Delete a Miaoda app runtime cache key").
			Params(dbEnvParams(rctx, map[string]interface{}{"key": rctx.Str("key")}))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		key := rctx.Str("key")
		data, err := rctx.CallAPITyped("DELETE", appCachePath(appID), dbEnvParams(rctx, map[string]interface{}{"key": key}), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		out := map[string]interface{}{
			"key":               key,
			"environment":       resolvedEnv(data, rctx),
			"deleted_key_count": cacheInt(data["deleted_key_count"]),
		}
		rctx.OutFormat(out, nil, func(w io.Writer) {
			renderCacheDeletePretty(w, out)
		})
		return nil
	},
}

// renderCacheDeletePretty 命中打 "✓ cache deleted"，幂等未命中打 "✓ cache already absent"（措辞区分，都成功）。
func renderCacheDeletePretty(w io.Writer, out map[string]interface{}) {
	key := common.GetString(out, "key")
	if n, ok := numericAsFloat(out["deleted_key_count"]); ok && n > 0 {
		fmt.Fprintf(w, "✓ cache deleted: %s\n", key)
		return
	}
	fmt.Fprintf(w, "✓ cache already absent: %s\n", key)
}
