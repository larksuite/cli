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

const createHint = "verify --app-type is html or full_stack and --name is non-empty; if this is a permission error, confirm your account can create Miaoda apps"

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
		{Name: "app-type", Desc: "app type", Required: true, Enum: []string{"html", "full_stack"}},
		{Name: "description", Desc: "app description"},
		{Name: "icon-url", Desc: "app icon URL (server uses default if omitted)"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("name")) == "" {
			return appsValidationParamError("--name", "--name is required")
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
		data, err := rctx.CallAPITyped("POST", apiBasePath+"/apps", nil, buildAppsCreateBody(rctx))
		if err != nil {
			return withAppsHint(err, createHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "created: %s\n", common.GetString(data, "app", "app_id"))
		})
		return nil
	},
}

func buildAppsCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	// --app-type is constrained to the lowercase enum (html / full_stack) by the
	// flag's Enum, so send it through verbatim. Legacy uppercase compatibility is
	// a server concern and is intentionally not surfaced by the CLI.
	body := map[string]interface{}{
		"name":     strings.TrimSpace(rctx.Str("name")),
		"app_type": rctx.Str("app-type"),
	}
	if desc := strings.TrimSpace(rctx.Str("description")); desc != "" {
		body["description"] = desc
	}
	if icon := strings.TrimSpace(rctx.Str("icon-url")); icon != "" {
		body["icon_url"] = icon
	}
	return body
}
