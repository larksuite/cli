// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsCreate creates a new Miaoda app.
var AppsCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+create",
	Description: "Create a new Miaoda app",
	Risk:        "write",
	Tips: []string{
		`Example: lark-cli apps +create --name "审批系统" --app-type full_stack`,
		`Example: lark-cli apps +create --name "活动页" --app-type html --description "活动报名"`,
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "name", Desc: "app display name", Required: true},
		{Name: "app-type", Desc: "app type (html or full_stack)", Required: true},
		{Name: "description", Desc: "app description"},
		{Name: "icon-url", Desc: "app icon URL (server uses default if omitted)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return output.ErrValidation("--name is required")
		}
		appType := strings.TrimSpace(rctx.Str("app-type"))
		if appType == "" {
			return output.ErrValidation("--app-type is required")
		}
		if !validAppTypes[appType] {
			return output.ErrValidation(fmt.Sprintf("--app-type %q is not supported (allowed: html, full_stack)", appType))
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST(apiBasePath + "/apps").
			Desc("Create a Miaoda app").
			Body(buildAppsCreateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPI("POST", apiBasePath+"/apps", nil, buildAppsCreateBody(rctx))
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "created: %s\n", common.GetString(data, "app", "app_id"))
		})
		return nil
	},
}

// 应用类型枚举。大小写敏感精确匹配（对外统一小写）。
var validAppTypes = map[string]bool{
	"html":       true,
	"full_stack": true,
}

func buildAppsCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	appType := strings.TrimSpace(rctx.Str("app-type"))
	body := map[string]interface{}{
		"name":     strings.TrimSpace(rctx.Str("name")),
		"app_type": appType,
	}
	if desc := strings.TrimSpace(rctx.Str("description")); desc != "" {
		body["description"] = desc
	}
	if icon := strings.TrimSpace(rctx.Str("icon-url")); icon != "" {
		body["icon_url"] = icon
	}
	return body
}
