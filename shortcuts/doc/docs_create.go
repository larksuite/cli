// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"os"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

var DocsCreate = common.Shortcut{
	Service:     "docs",
	Command:     "+create",
	Description: "Create a Lark document",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"docx:document:create", "docs:document.media:upload", "docx:document:write_only", "docx:document:readonly"},
	Flags: []common.Flag{
		{Name: "title", Desc: "document title"},
		{Name: "markdown", Desc: "Markdown content (Lark-flavored)", Required: true},
		{Name: "folder-token", Desc: "parent folder token"},
		{Name: "wiki-node", Desc: "wiki node token"},
		{Name: "wiki-space", Desc: "wiki space ID (use my_library for personal library)"},
		{Name: "base-dir", Desc: "base directory for resolving local image paths (default: current working directory)"},
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

		if dir := runtime.Str("base-dir"); dir != "" {
			info, err := os.Stat(dir)
			if err != nil {
				return output.ErrValidation("--base-dir %q does not exist: %v", dir, err)
			}
			if !info.IsDir() {
				return output.ErrValidation("--base-dir %q is not a directory", dir)
			}
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

		d := common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: create-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "create-doc", "arguments": args}}).
			Set("mcp_tool", "create-doc").Set("args", args)

		if hasLocalImages(runtime.Str("markdown")) {
			d.Desc("Two-phase create: create-doc + upload local images + update-doc (overwrite)")
		}

		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		markdown := runtime.Str("markdown")
		args := map[string]interface{}{
			"markdown": markdown,
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

		// If markdown contains local image paths, use two-phase creation
		if hasLocalImages(markdown) {
			baseDir := runtime.Str("base-dir")
			if baseDir == "" {
				var err error
				baseDir, err = os.Getwd()
				if err != nil {
					return output.ErrValidation("cannot determine working directory: %v", err)
				}
			}

			result, err := processMarkdownImages(ctx, runtime, markdown, baseDir, args)
			if err != nil {
				return err
			}
			runtime.Out(result, nil)
			return nil
		}

		result, err := common.CallMCPTool(runtime, "create-doc", args)
		if err != nil {
			return err
		}

		// Post-process: auto-resize table column widths
		if docID := common.GetString(result, "doc_id"); docID != "" {
			if warn := autoResizeTableColumns(runtime, docID); warn != "" {
				fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", warn)
			}
		}

		runtime.Out(result, nil)
		return nil
	},
}
