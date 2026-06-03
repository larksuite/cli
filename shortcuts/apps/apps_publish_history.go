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

// AppsPublishHistory lists a Miaoda app's release history (most recent first).
var AppsPublishHistory = common.Shortcut{
	Service:     appsService,
	Command:     "+publish-history",
	Description: "List a Miaoda app's release history (server returns ~50 most recent by default)",
	Risk:        "read",
	Scopes:      []string{"spark:app:read"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "app-id", Desc: "Miaoda app ID", Required: true},
		{Name: "status", Enum: []string{"publishing", "finished", "failed"}, Desc: "filter by release status: publishing | finished | failed"},
		{Name: "limit", Type: "int", Desc: "page size (1-500); omit to use server default (~50)"},
		{Name: "page-token", Desc: "pagination cursor from a previous response"},
	},
	Validate: func(ctx context.Context, rctx *common.RuntimeContext) error {
		if strings.TrimSpace(rctx.Str("app-id")) == "" {
			return output.ErrValidation("--app-id is required")
		}
		return validateHistoryLimit(rctx.Int("limit"))
	},
	DryRun: func(ctx context.Context, rctx *common.RuntimeContext) *common.DryRunAPI {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		status := strings.TrimSpace(rctx.Str("status"))
		limit := rctx.Int("limit")
		pageToken := strings.TrimSpace(rctx.Str("page-token"))
		dry := common.NewDryRunAPI()
		dry.GET(fmt.Sprintf(publishListPath, validate.EncodePathSegment(appID))).
			Desc("List release history").
			Params(buildHistoryQuery(status, limit, pageToken))
		return dry
	},
	Execute: func(ctx context.Context, rctx *common.RuntimeContext) error {
		appID := strings.TrimSpace(rctx.Str("app-id"))
		status := strings.TrimSpace(rctx.Str("status"))
		limit := rctx.Int("limit")
		pageToken := strings.TrimSpace(rctx.Str("page-token"))
		path := fmt.Sprintf(publishListPath, validate.EncodePathSegment(appID))
		data, err := rctx.CallAPI("GET", path, buildHistoryQuery(status, limit, pageToken), nil)
		if err != nil {
			return err
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

// buildHistoryQuery builds the list-releases query parameters. app_id is in the
// path. status is included when non-empty; limit when > 0; page_token (snake)
// when non-empty.
func buildHistoryQuery(status string, limit int, pageToken string) map[string]interface{} {
	q := map[string]interface{}{}
	if status != "" {
		q["status"] = status
	}
	if limit > 0 {
		q["limit"] = limit
	}
	if pageToken != "" {
		q["page_token"] = pageToken
	}
	return q
}

// validateHistoryLimit accepts 0 (unset) or 1-500.
func validateHistoryLimit(limit int) error {
	if limit == 0 {
		return nil
	}
	if limit < 1 || limit > 500 {
		return output.ErrValidation("--limit must be between 1 and 500")
	}
	return nil
}
