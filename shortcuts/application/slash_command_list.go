// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package application

import (
	"context"
	"fmt"
	"io"

	"github.com/larksuite/cli/shortcuts/common"
)

// SlashCommandList lists all slash commands of the current bound app.
var SlashCommandList = common.Shortcut{
	Service:     "application",
	Command:     "+slash-command-list",
	Description: "List all slash commands (/ commands) registered on the currently bound Open Platform app; source of command_id for update/delete",
	Risk:        "read",
	Scopes:      []string{"application:app_slash_command:read"},
	AuthTypes:   []string{"bot", "user"},
	Tips: []string{
		"lark-cli application +slash-command-list --as bot",
		"user identity needs explicit authorization first: lark-cli auth login --scope application:app_slash_command:read",
		"the upstream API returns all commands at once (max 100 per app, no pagination)",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("List all slash commands of the current bound app (read-only)").
			GET(slashCommandBasePath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped("GET", slashCommandBasePath, nil, nil)
		if err != nil {
			return err
		}
		items, _ := data["items"].([]interface{})
		if items == nil {
			items = []interface{}{}
		}
		out := map[string]interface{}{"items": items, "count": len(items)}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%d slash command(s)\n", len(items))
			for _, it := range items {
				m, ok := it.(map[string]interface{})
				if !ok {
					continue
				}
				desc := ""
				if d, ok := m["description"].(map[string]interface{}); ok {
					desc, _ = d["default_value"].(string)
				}
				fmt.Fprintf(w, "  /%v\t%v\t%s\n", m["command"], m["command_id"], desc)
			}
		})
		return nil
	},
}
