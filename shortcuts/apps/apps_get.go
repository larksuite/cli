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

// AppsGet fetches a single app's detail by app ID.
var AppsGet = common.Shortcut{
	Service:     appsService,
	Command:     "+get",
	Description: "Get a single app's detail by app ID (returns app_type, name, description, publish status, etc.)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +get --app-id <app_id>",
		"Example: lark-cli apps +get --app-id <app_id> --dry-run",
		"Tip: extract app type with --jq '.data.app.app_type'",
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
			GET(fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))).
			Desc("Get app detail (returns app_id, app_type, name, description, icon_url, created_at, updated_at, is_published)")
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		data, err := rctx.CallAPITyped("GET", fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID)), nil, nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			app, _ := data["app"].(map[string]interface{})
			if app == nil {
				return
			}
			fmt.Fprintf(w, "app_id: %v\n", app["app_id"])
			fmt.Fprintf(w, "app_type: %v\n", app["app_type"])
			fmt.Fprintf(w, "name: %v\n", app["name"])
			if desc, ok := app["description"].(string); ok && desc != "" {
				fmt.Fprintf(w, "description: %s\n", desc)
			}
			fmt.Fprintf(w, "is_published: %v\n", app["is_published"])
			fmt.Fprintf(w, "updated_at: %v\n", app["updated_at"])
		})
		return nil
	},
}
