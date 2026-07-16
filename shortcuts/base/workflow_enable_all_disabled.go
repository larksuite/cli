// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var BaseWorkflowEnableAllDisabled = common.Shortcut{
	Service:     "base",
	Command:     "+workflow-enable-all-disabled",
	Description: "Enable all currently disabled workflows in a base and summarize per-workflow failures",
	Risk:        "write",
	Scopes:      []string{"base:workflow:read", "base:workflow:update"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "base-token", Desc: "base token", Required: true},
		{Name: "page-size", Type: "int", Default: "100", Desc: "page size per list request, range 1-100"},
	},
	Tips: []string{
		"Use this when the user asks to enable every disabled workflow in a Base.",
		"The command lists disabled workflows, enables each one, continues after per-workflow API failures, and returns succeeded/failed/remaining_disabled summaries.",
		"Per-workflow failures usually mean the platform rejected that workflow configuration; do not switch to steps repair unless the user explicitly asks to fix the workflow definition.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("base-token")) == "" {
			return baseFlagErrorf("--base-token must not be blank")
		}
		_, err := common.ValidatePageSizeTyped(runtime, "page-size", 100, 1, 100)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/base/v3/bases/:base_token/workflows/list").
			Body(map[string]interface{}{"page_size": runtime.Int("page-size"), "status": "disabled"}).
			Set("base_token", runtime.Str("base-token")).
			PATCH("/open-apis/base/v3/bases/:base_token/workflows/:workflow_id/enable").
			Desc("For each disabled workflow_id returned by the list call. Per-item failures are collected and do not stop the batch.").
			Set("workflow_id", "<workflow_id>")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		disabled, err := listWorkflowsByStatus(runtime, runtime.Str("base-token"), "disabled", runtime.Int("page-size"))
		if err != nil {
			return err
		}

		succeeded := []interface{}{}
		failed := []interface{}{}
		for _, workflow := range disabled {
			workflowID := strings.TrimSpace(common.GetString(workflow, "workflow_id"))
			if workflowID == "" {
				workflowID = strings.TrimSpace(common.GetString(workflow, "id"))
			}
			if workflowID == "" {
				failed = append(failed, map[string]interface{}{
					"workflow": workflow,
					"error":    "missing workflow_id in list response",
				})
				continue
			}
			data, err := baseV3Call(runtime, "PATCH",
				baseV3Path("bases", runtime.Str("base-token"), "workflows", workflowID, "enable"),
				nil,
				map[string]interface{}{},
			)
			if err != nil {
				failed = append(failed, map[string]interface{}{
					"workflow_id": workflowID,
					"title":       common.GetString(workflow, "title"),
					"error":       err.Error(),
				})
				continue
			}
			succeeded = append(succeeded, data)
		}

		remaining, err := listWorkflowsByStatus(runtime, runtime.Str("base-token"), "disabled", runtime.Int("page-size"))
		if err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{
			"summary": map[string]interface{}{
				"initial_disabled":   len(disabled),
				"enabled":            len(succeeded),
				"failed":             len(failed),
				"remaining_disabled": len(remaining),
			},
			"succeeded":          succeeded,
			"failed":             failed,
			"remaining_disabled": remaining,
		}, nil)
		return nil
	},
}

func listWorkflowsByStatus(runtime *common.RuntimeContext, baseToken string, status string, pageSize int) ([]map[string]interface{}, error) {
	allItems := []map[string]interface{}{}
	pageToken := ""
	for {
		body := map[string]interface{}{
			"page_size": pageSize,
			"status":    status,
		}
		if pageToken != "" {
			body["page_token"] = pageToken
		}
		data, err := baseV3Call(runtime, "POST", baseV3Path("bases", baseToken, "workflows", "list"), nil, body)
		if err != nil {
			return nil, err
		}
		if items, _ := data["items"].([]interface{}); len(items) > 0 {
			for _, item := range items {
				if obj, ok := item.(map[string]interface{}); ok {
					allItems = append(allItems, obj)
				}
			}
		}
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
	return allItems, nil
}
