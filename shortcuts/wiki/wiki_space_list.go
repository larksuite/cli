// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"

	"github.com/larksuite/cli/shortcuts/common"
)

// WikiSpaceList lists all wiki spaces the caller has access to.
var WikiSpaceList = common.Shortcut{
	Service:     "wiki",
	Command:     "+space-list",
	Description: "List wiki spaces accessible to the caller",
	Risk:        "read",
	Scopes:      []string{"wiki:space:retrieve"},
	AuthTypes:   []string{"user", "bot"},
	Flags:       []common.Flag{},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			GET("/open-apis/wiki/v2/spaces").
			Params(map[string]interface{}{"page_size": 50})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		var spaces []map[string]interface{}
		pageToken := ""
		for {
			params := map[string]interface{}{"page_size": 50}
			if pageToken != "" {
				params["page_token"] = pageToken
			}
			data, err := runtime.CallAPI("GET", "/open-apis/wiki/v2/spaces", params, nil)
			if err != nil {
				return err
			}
			items, _ := data["items"].([]interface{})
			for _, item := range items {
				if m, ok := item.(map[string]interface{}); ok {
					spaces = append(spaces, parseWikiSpaceItem(m))
				}
			}
			next, _ := data["page_token"].(string)
			hasMore, _ := data["has_more"].(bool)
			if !hasMore || next == "" {
				break
			}
			pageToken = next
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Found %d wiki space(s)\n", len(spaces))
		runtime.Out(map[string]interface{}{
			"spaces": spaces,
			"total":  len(spaces),
		}, nil)
		return nil
	},
}

func parseWikiSpaceItem(m map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"space_id":     common.GetString(m, "space_id"),
		"name":         common.GetString(m, "name"),
		"description":  common.GetString(m, "description"),
		"space_type":   common.GetString(m, "space_type"),
		"visibility":   common.GetString(m, "visibility"),
		"open_sharing": common.GetString(m, "open_sharing"),
	}
}
