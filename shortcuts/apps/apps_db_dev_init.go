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

// AppsDBDevInit initializes dev/online multi-env DB ().
//
// 调 POST /apps/{app_id}/db_dev_init。
// 不可逆：单库一旦拆成 dev/online 双库无法回退。Risk: high-risk-write 触发框架自动注入 --yes 确认关卡。
var AppsDBDevInit = common.Shortcut{
	Service:     appsService,
	Command:     "+db-dev-init",
	Description: "Initialize dev environment (split single-env DB into dev/online, irreversible)",
	Risk:        "high-risk-write",
	Scopes:      []string{"spark:app:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app id", Required: true},
		{Name: "sync-data", Type: "bool", Desc: "copy existing online data into the new dev branch"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		_, err := requireAppID(rctx.Str("app-id"))
		return err
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID, _ := requireAppID(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			POST(appDbDevInitPath(appID)).
			Desc("Initialize Miaoda app multi-env database").
			Body(map[string]interface{}{"sync_data": rctx.Bool("sync-data")})
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID, err := requireAppID(rctx.Str("app-id"))
		if err != nil {
			return err
		}
		data, err := rctx.CallAPI("POST", appDbDevInitPath(appID), nil,
			map[string]interface{}{"sync_data": rctx.Bool("sync-data")})
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			renderDevInitPretty(w, data)
		})
		return nil
	},
}

// renderDevInitPretty 输出 4 行（pretty 模式）：
//
//	✓ Multi-env initialized
//	Environments: dev, online
//	Data synced: yes
//	Note: structure changes in dev now need to be released to online.
func renderDevInitPretty(w io.Writer, data map[string]interface{}) {
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
