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
	Scopes:      []string{"spark:app:write"}, // 对齐 BOE 后端 scope 命名 (spark 命名空间)
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "name", Desc: "app display name", Required: true},
		{Name: "description", Desc: "app description"},
		{Name: "icon-url", Desc: "app icon URL (server uses default if omitted)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return output.ErrValidation("--name is required")
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
			fmt.Fprintf(w, "created: %s\n", common.GetString(data, "app_id"))
		})
		return nil
	},
}

func buildAppsCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"name": strings.TrimSpace(rctx.Str("name")),
	}
	if desc := strings.TrimSpace(rctx.Str("description")); desc != "" {
		body["description"] = desc
	}
	if icon := strings.TrimSpace(rctx.Str("icon-url")); icon != "" {
		body["icon_url"] = icon
	}
	return body
}
