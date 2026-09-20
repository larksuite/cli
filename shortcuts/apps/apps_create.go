// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/shortcuts/common"
)

const createHint = "verify --app-type is html, frontend or full_stack and --name is non-empty; if the server rejected access_scope, retry without it and the app is created with the server default scope; if this is a permission error, confirm your account can create apps"

// AppsCreate creates a new app.
var AppsCreate = common.Shortcut{
	Service:     appsService,
	Command:     "+create",
	Description: "Create a new app",
	Risk:        "write",
	Tips: []string{
		`Example: lark-cli apps +create --name "审批系统" --app-type full_stack`,
		`Example: lark-cli apps +create --name "工具页" --app-type frontend --description "纯前端工具"`,
		`Example: lark-cli apps +create --name "活动页" --app-type html --description "活动报名"`,
		`Only when a visibility was asked for: lark-cli apps +create --name "内部工具" --app-type frontend --access-scope tenant   (omit --access-scope otherwise and the server picks the default)`,
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "name", Desc: "app display name", Required: true},
		{Name: "app-type", Desc: "app type", Required: true, Enum: []string{"html", "frontend", "full_stack"}},
		{Name: "description", Desc: "app description"},
		{Name: "icon-url", Desc: "app icon URL (server uses default if omitted)"},
		{Name: "access-scope", Desc: "default runtime access scope, for when one was actually asked for: self (only the creator) | tenant (everyone in the organization) | public (anyone on the internet); leave it out otherwise and the server picks the default", Enum: []string{"self", "tenant", "public"}},
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
			Desc("Create an app").
			Body(buildAppsCreateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		data, err := rctx.CallAPITyped("POST", apiBasePath+"/apps", nil, buildAppsCreateBody(rctx))
		if err != nil {
			return withAppsHint(err, createHint)
		}
		// The app exists from here on; a refused scope is reported, not raised.
		warnAccessScopeResult(rctx.IO().ErrOut, rctx.Str("access-scope"), data)
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "created: %s\n", common.GetString(data, "app", "app_id"))
		})
		return nil
	},
}

func buildAppsCreateBody(rctx *common.RuntimeContext) map[string]interface{} {
	// --app-type is constrained to the lowercase enum (html / frontend / full_stack) by the
	// flag's Enum, so send it through verbatim. Legacy uppercase compatibility is
	// a server concern and is intentionally not surfaced by the CLI.
	agent := envvars.AgentName()
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
	// --access-scope is a strict lowercase enum guarded by the flag's Enum, and the
	// server defines the same values, so forward it verbatim; omitting the flag
	// leaves the field out and lets the server pick its default.
	if scope := rctx.Str("access-scope"); scope != "" {
		body["access_scope"] = scope
	}
	if agent != "" {
		body["source_agent"] = agent
	}
	return body
}
