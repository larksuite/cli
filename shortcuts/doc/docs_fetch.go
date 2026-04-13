// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/shortcuts/common"
)

var DocsFetch = common.Shortcut{
	Service:     "docs",
	Command:     "+fetch",
	Description: "Fetch Lark document content",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "offset", Desc: "pagination offset"},
		{Name: "limit", Desc: "pagination limit"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
		}
		if v := runtime.Str("offset"); v != "" {
			n, _ := strconv.Atoi(v)
			args["offset"] = n
		}
		if v := runtime.Str("limit"); v != "" {
			n, _ := strconv.Atoi(v)
			args["limit"] = n
		}
		return common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: fetch-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "fetch-doc", "arguments": args}}).
			Set("mcp_tool", "fetch-doc").Set("args", args)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
		}
		if v := runtime.Str("offset"); v != "" {
			n, _ := strconv.Atoi(v)
			args["offset"] = n
		}
		if v := runtime.Str("limit"); v != "" {
			n, _ := strconv.Atoi(v)
			args["limit"] = n
		}

		result, err := common.CallMCPTool(runtime, "fetch-doc", args)
		if err != nil {
			return err
		}

		if md, ok := result["markdown"].(string); ok {
			result["markdown"] = fixExportedMarkdown(md)
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			if title, ok := result["title"].(string); ok && title != "" {
				fmt.Fprintf(w, "# %s\n\n", title)
			}
			if md, ok := result["markdown"].(string); ok {
				fmt.Fprintln(w, md)
			}
			if hasMore, ok := result["has_more"].(bool); ok && hasMore {
				fmt.Fprintln(w, "\n--- more content available, use --offset and --limit to paginate ---")
			}
		})
		return nil
	},
}
