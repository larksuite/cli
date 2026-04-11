// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// WikiNodeList lists child nodes in a wiki space or under a parent node.
var WikiNodeList = common.Shortcut{
	Service:     "wiki",
	Command:     "+node-list",
	Description: "List wiki nodes in a space or under a parent node",
	Risk:        "read",
	Scopes:      []string{"wiki:node:retrieve"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "space-id", Desc: "wiki space ID; use my_library for personal document library", Required: true},
		{Name: "parent-node-token", Desc: "parent node token; if omitted, lists the root-level nodes of the space"},
	},
	Tips: []string{
		"Use --parent-node-token to drill into a sub-directory.",
		"--space-id my_library lists the root of your personal document library.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		if err := validateOptionalResourceName(spaceID, "--space-id"); err != nil {
			return err
		}
		return validateOptionalResourceName(strings.TrimSpace(runtime.Str("parent-node-token")), "--parent-node-token")
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		params := map[string]interface{}{"page_size": 50}
		if pt := strings.TrimSpace(runtime.Str("parent-node-token")); pt != "" {
			params["parent_node_token"] = pt
		}
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", validate.EncodePathSegment(spaceID))).
			Params(params).
			Set("space_id", spaceID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spaceID := strings.TrimSpace(runtime.Str("space-id"))
		parentNodeToken := strings.TrimSpace(runtime.Str("parent-node-token"))

		var nodes []map[string]interface{}
		pageToken := ""
		for {
			params := map[string]interface{}{"page_size": 50}
			if parentNodeToken != "" {
				params["parent_node_token"] = parentNodeToken
			}
			if pageToken != "" {
				params["page_token"] = pageToken
			}
			data, err := runtime.CallAPI("GET",
				fmt.Sprintf("/open-apis/wiki/v2/spaces/%s/nodes", validate.EncodePathSegment(spaceID)),
				params, nil)
			if err != nil {
				return err
			}
			items, _ := data["items"].([]interface{})
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					nodes = append(nodes, wikiNodeListItem(m))
				}
			}
			next, _ := data["page_token"].(string)
			hasMore, _ := data["has_more"].(bool)
			if !hasMore || next == "" {
				break
			}
			pageToken = next
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d node(s)\n", len(nodes))
		runtime.Out(map[string]interface{}{
			"nodes": nodes,
			"total": len(nodes),
		}, nil)
		return nil
	},
}

func wikiNodeListItem(m map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"space_id":          common.GetString(m, "space_id"),
		"node_token":        common.GetString(m, "node_token"),
		"obj_token":         common.GetString(m, "obj_token"),
		"obj_type":          common.GetString(m, "obj_type"),
		"parent_node_token": common.GetString(m, "parent_node_token"),
		"node_type":         common.GetString(m, "node_type"),
		"title":             common.GetString(m, "title"),
		"has_child":         common.GetBool(m, "has_child"),
	}
}
