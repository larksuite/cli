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

const dbEnvCreateHint = "verify --app-id is correct; if the app is already multi-env this is a conflict — inspect current tables with `lark-cli apps +db-table-list --app-id <app_id> --environment dev`"

// AppsDBEnvCreate creates a DB environment for an app（拆分单库为 dev/online 多环境）。
//
// 调 POST /apps/{app_id}/db_dev_init。--environment 指定要创建的环境，由调用方传入，目前只支持 dev。
// 不可逆：单库一旦拆成 dev/online 双库无法回退。Risk: high-risk-write 触发框架自动注入 --yes 确认关卡。
var AppsDBEnvCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+db-env-create",
	Description: "Create a DB environment (split single-env DB into dev/online, irreversible)",
	Risk:        "high-risk-write",
	Tips: []string{
		"Example: lark-cli apps +db-env-create --environment dev --sync-data --app-id <app_id> --yes",
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: append([]common.Flag{
		{Name: "app-id", Desc: "app id", Required: true},
		{Name: "sync-data", Type: "bool", Desc: "copy existing online data into the new environment (default off)"},
	}, dbEnvFlags("dev", []string{"dev"}, "environment to create (only dev supported for now)")...),
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if _, err := requireAppID(rctx.Str("app-id")); err != nil {
			return err
		}
		return rejectLegacyEnvFlag(rctx)
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appDbEnvCreatePath(appID)).
			Desc("Create app DB environment").
			Body(buildDBEnvCreateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPITyped("POST", appDbEnvCreatePath(appID), nil, buildDBEnvCreateBody(rctx))
		if err != nil {
			return withAppsHint(err, dbEnvCreateHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderEnvCreatePretty(w, data)
		})
		return nil
	},
}

// buildDBEnvCreateBody 构造 db 环境创建 body：sync_data（bool）。
// --environment 目前只支持 dev、服务端接口本身即创建 dev 环境，故不下发 env 字段（仅做 CLI 入参校验/前向兼容）。
func buildDBEnvCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	return map[string]interface{}{
		"sync_data": rctx.Bool("sync-data"),
	}
}

// renderEnvCreatePretty 输出 4 行（pretty 模式）：
//
//	✓ Multi-env initialized
//	Environments: dev, online
//	Data synced: yes
//	Note: structure changes in dev now need to be released to online.
func renderEnvCreatePretty(w io.Writer, data map[string]interface{}) {
	fmt.Fprintln(w, "✓ Multi-env initialized")

	if envs, ok := data["environments"].([]interface{}); ok && len(envs) > 0 {
		names := make([]string, 0, len(envs))
		for _, e := range envs {
			if s, ok := e.(string); ok {
				names = append(names, s)
			}
		}
		fmt.Fprintf(w, "Environments: %s\n", strings.Join(names, ", "))
	}

	synced := "no"
	if ds, ok := data["data_synced"].(bool); ok && ds {
		synced = "yes"
	}
	fmt.Fprintf(w, "Data synced: %s\n", synced)

	fmt.Fprintln(w, "Note: structure changes in dev now need to be released to online.")
}
