// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"

	"github.com/larksuite/cli/shortcuts/common"
)

var DocsCreate = common.Shortcut{
	Service:     "docs",
	Command:     "+create",
	Description: "Create a Lark document",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"docx:document:create"},
	Flags: []common.Flag{
		{Name: "title", Desc: "document title"},
		{Name: "markdown", Desc: "Markdown content (Lark-flavored)", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "folder-token", Desc: "parent folder token"},
		{Name: "wiki-node", Desc: "wiki node token"},
		{Name: "wiki-space", Desc: "wiki space ID (use my_library for personal library)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		count := 0
		if runtime.Str("folder-token") != "" {
			count++
		}
		if runtime.Str("wiki-node") != "" {
			count++
		}
		if runtime.Str("wiki-space") != "" {
			count++
		}
		if count > 1 {
			return common.FlagErrorf("--folder-token, --wiki-node, and --wiki-space are mutually exclusive")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		args := map[string]interface{}{
			"markdown": runtime.Str("markdown"),
		}
		if v := runtime.Str("title"); v != "" {
			args["title"] = v
		}
		if v := runtime.Str("folder-token"); v != "" {
			args["folder_token"] = v
		}
		if v := runtime.Str("wiki-node"); v != "" {
			args["wiki_node"] = v
		}
		if v := runtime.Str("wiki-space"); v != "" {
			args["wiki_space"] = v
		}
		return common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: create-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "create-doc", "arguments": args}}).
			Set("mcp_tool", "create-doc").Set("args", args)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args := map[string]interface{}{
			"markdown": runtime.Str("markdown"),
		}
		if v := runtime.Str("title"); v != "" {
			args["title"] = v
		}
		if v := runtime.Str("folder-token"); v != "" {
			args["folder_token"] = v
		}
		if v := runtime.Str("wiki-node"); v != "" {
			args["wiki_node"] = v
		}
		if v := runtime.Str("wiki-space"); v != "" {
			args["wiki_space"] = v
		}

		result, err := common.CallMCPTool(runtime, "create-doc", args)
		if err != nil {
			return err
		}

		runtime.Out(result, nil)
		return nil
	},
}
