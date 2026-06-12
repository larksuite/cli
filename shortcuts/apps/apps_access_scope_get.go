// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsAccessScopeGet reads the current access scope configuration of a Miaoda app.
// 响应原样透传服务端契约（字符串 scope 枚举 All/Tenant/Range + 拆分的 users/departments/chats 数组）。
var AppsAccessScopeGet = common.Shortcut{
	Service:     appsService,
	Command:     "+access-scope-get",
	Description: "Get Miaoda app access scope configuration",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +access-scope-get --app-id <app_id>",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(appID))).
			Desc("Get Miaoda app access scope")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf("%s/apps/%s/access-scope", apiBasePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("GET", path, nil, nil)
		if err != nil {
			return withAppsHint(err, "verify --app-id is correct and you have access to the app; list your apps with `lark-cli apps +list`")
		}
		// 原样透传 — 保留服务端字符串枚举 (All/Tenant/Range)，不合并 users/departments/chats。
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "scope: %v\n", data["scope"])
		})
		return nil
	},
}
