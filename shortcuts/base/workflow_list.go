// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowList = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-list",
	Description: "List all workflows in a base (auto-paginated)",
	Risk:        "read",
	Scopes:      []string{"base:workflow:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "status", Desc: "filter by status", Enum: []string{"enabled", "disabled"}},
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size per request, range 1-100"},
	},
	Tips: []string{
		"Returns workflow_id values with wkf prefix; pass those IDs to +workflow-get/enable/disable/update.",
		"This shortcut auto-paginates and returns all matched workflows.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{
			"page_size": runtime.Int("page-size"),
		}
		if s := runtime.Str("status"); s != "" {
			body["status"] = s
		}
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/workflows/list").
			Body(body).
			Set("base_token", runtime.Str("base-token"))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		var allItems []interface{}
		pageToken := ""
		for {
			body := map[string]interface{}{
				"page_size": runtime.Int("page-size"),
			}
			if pageToken != "" {
				body["page_token"] = pageToken
			}
			if s := runtime.Str("status"); s != "" {
				body["status"] = s
			}
			data, err := baseV3Call(runtime, "POST",
				baseV3Path("bases", runtime.Str("base-token"), "workflows", "list"),
				nil,
				body,
			)
			if err != nil {
				return err
			}
			items, _ := data["items"].([]interface{})
			allItems = append(allItems, items...)
			hasMore, _ := data["has_more"].(bool)
			if !hasMore {
				break
			}
			nextToken, _ := data["page_token"].(string)
			if nextToken == "" {
				break
			}
			pageToken = nextToken
		}
		runtime.Out(map[string]interface{}{
			"items": allItems,
			"total": len(allItems),
		}, nil)
		return nil
	},
}
