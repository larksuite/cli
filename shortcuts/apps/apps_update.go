// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// AppsUpdate partially updates a Miaoda app's name / description.
var AppsUpdate = common.Shortcut{
	Service:     appsService,
	Command:     "+update",
	Description: "Partially update a Miaoda app (only provided fields are sent)",
	Risk:        "write",
	Scopes:      []string{"spark:app.table:write"}, // BOE 端 OAPI 当前以 spark 系列 scope 注册；待后端 miaoda:app:* 注册稳定后再切回
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "app ID", Required: true},
		{Name: "name", Desc: "new app display name"},
		{Name: "description", Desc: "new app description"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		body := buildAppsUpdateBody(rctx)
		if len(body) == 0 {
			return output.ErrValidation("provide at least one of --name or --description")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			PATCH(fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(rctx.Str("app-id")))).
			Desc("Update a Miaoda app").
			Body(buildAppsUpdateBody(rctx))
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		path := fmt.Sprintf("%s/apps/%s", apiBasePath, validate.EncodePathSegment(rctx.Str("app-id")))
		data, err := rctx.CallAPI("PATCH", path, nil, buildAppsUpdateBody(rctx))
		if err != nil {
			return err
		}
		rctx.OutFormat(data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "updated: %s\n", common.GetString(data, "app_id"))
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
