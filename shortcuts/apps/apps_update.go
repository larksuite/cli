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

// AppsUpdate partially updates a Miaoda app's name / description.
var AppsUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+update",
	Description: "Partially update a Miaoda app (only provided fields are sent)",
	Risk:        "write",
	Tips: []string{
		`Example: lark-cli apps +update --app-id <app_id> --name "新名称"`,
		`Example: lark-cli apps +update --app-id <app_id> --description "..."`,
	},
	Scopes:    []string{"spark:app:write"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "name", Desc: "new app display name"},
		{Name: "description", Desc: "new app description"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		body := buildAppsUpdateBody(rctx)
		if len(body) == 0 {
			return appsValidationError("provide at least one of --name or --description").
				WithParams(
					appsInvalidParam("--name", "provide at least one of --name or --description"),
					appsInvalidParam("--description", "provide at least one of --name or --description"),
				)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		return common.NewDryRunAPI().
			PATCH(fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))).
			Desc("Update a Miaoda app").
			Body(buildAppsUpdateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("PATCH", path, nil, buildAppsUpdateBody(rctx))
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "updated: %s\n", common.GetString(data, "app", "app_id"))
		})
		return nil
	},
}

func buildAppsUpdateBody(rctx *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{}
	if v := strings.TrimSpace(rctx.Str("name")); v != "" {
		body["name"] = v
	}
	if v := strings.TrimSpace(rctx.Str("description")); v != "" {
		body["description"] = v
	}
	return body
}
