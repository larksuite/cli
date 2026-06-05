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

// AppsReleaseList lists a Miaoda app's release history (most recent first).
var AppsReleaseList = common.Shortcut{
	Service:     appsService,
	Command:     "+release-list",
	Description: "List a Miaoda app's release history (most recent first)",
	Risk:        "read",
	Tips: []string{
		"Example: lark-cli apps +release-list --app-id <app_id>",
		"Tip: filter fields with --jq, e.g. -q '.data.releases[].release_id'",
	},
	Scopes:    []string{"spark:app:read"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "status", Enum: []string{"publishing", "finished", "failed"}, Desc: "filter by release status: publishing | finished | failed"},
		{Name: "page-size", Type: "int", Default: "20", Desc: "page size (max 500)"},
		{Name: "page-token", Desc: "pagination cursor from a previous response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return appsValidationParamError("--app-id", "--app-id is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		status := strings.TrimSpace(rctx.Str("status"))
		pageSize := rctx.Int("page-size")
		pageToken := strings.TrimSpace(rctx.Str("page-token"))
		dry := common.NewDryRunAPI()
		dry.GET(fmt.Sprintf(releaseListPath, validate.EncodePathSegment(appID))).
			Desc("List release history").
			Params(buildReleaseListQuery(status, pageSize, pageToken))
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		status := strings.TrimSpace(rctx.Str("status"))
		pageSize := rctx.Int("page-size")
		pageToken := strings.TrimSpace(rctx.Str("page-token"))
		path := fmt.Sprintf(releaseListPath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPITyped("GET", path, buildReleaseListQuery(status, pageSize, pageToken), nil)
		if err != nil {
			return withAppsHint(err, appIDListHint)
		}
		releases, _ := data["releases"].([]interface{})
		rctx.OutFormat(data, nil, func(w io.Writer) {
			rows := make([]map[string]interface{}, 0, len(releases))
			for _, it := range releases {
				m, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, map[string]interface{}{
					"release_id": m["release_id"],
					"status":     m["status"],
					"created_at": m["created_at"],
					"updated_at": m["updated_at"],
				})
			}
			output.PrintTable(w, rows)
		})
		return nil
	},
}

// buildReleaseListQuery builds the list-releases query parameters. app_id is in
// the path. page_size is always sent; status and page_token (snake) are included
// only when non-empty.
func buildReleaseListQuery(status string, pageSize int, pageToken string) map[string]interface{} {
	q := map[string]interface{}{
		"page_size": pageSize,
	}
	if status != "" {
		q["status"] = status
	}
	if pageToken != "" {
		q["page_token"] = pageToken
	}
	return q
}
