// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var validModes = map[string]bool{
	"append":        true,
	"overwrite":     true,
	"replace_range": true,
	"replace_all":   true,
	"insert_before": true,
	"insert_after":  true,
	"delete_range":  true,
}

var needsSelection = map[string]bool{
	"replace_range": true,
	"replace_all":   true,
	"insert_before": true,
	"insert_after":  true,
	"delete_range":  true,
}

var DocsUpdate = common.Shortcut{
	Service:     "docs",
	Command:     "+update",
	Description: "Update a Lark document",
	Risk:        "write",
	Scopes:      []string{"docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "mode", Desc: "update mode: append | overwrite | replace_range | replace_all | insert_before | insert_after | delete_range", Required: true},
		{Name: "markdown", Desc: "new content (Lark-flavored Markdown; create blank whiteboards with <whiteboard type=\"blank\"></whiteboard>, repeat to create multiple boards)", Input: []string{common.File, common.Stdin}},
		{Name: "selection-with-ellipsis", Desc: "content locator (e.g. 'start...end')"},
		{Name: "selection-by-title", Desc: "title locator (e.g. '## Section')"},
		{Name: "new-title", Desc: "also update document title"},
		{Name: "auto-table-widths", Type: "bool", Default: "true", Desc: "for --mode overwrite, adjust Markdown pipe-table column widths based on cell content (use --auto-table-widths=false to keep equal widths; ignored for other modes)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		mode := runtime.Str("mode")
		if !validModes[mode] {
			return common.FlagErrorf("invalid --mode %q, valid: append | overwrite | replace_range | replace_all | insert_before | insert_after | delete_range", mode)
		}

		if mode != "delete_range" && runtime.Str("markdown") == "" {
			return common.FlagErrorf("--%s mode requires --markdown", mode)
		}

		selEllipsis := runtime.Str("selection-with-ellipsis")
		selTitle := runtime.Str("selection-by-title")
		if selEllipsis != "" && selTitle != "" {
			return common.FlagErrorf("--selection-with-ellipsis and --selection-by-title are mutually exclusive")
		}

		if needsSelection[mode] && selEllipsis == "" && selTitle == "" {
			return common.FlagErrorf("--%s mode requires --selection-with-ellipsis or --selection-by-title", mode)
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
			"mode":   runtime.Str("mode"),
		}
		if v := runtime.Str("markdown"); v != "" {
			args["markdown"] = v
		}
		if v := runtime.Str("selection-with-ellipsis"); v != "" {
			args["selection_with_ellipsis"] = v
		}
		if v := runtime.Str("selection-by-title"); v != "" {
			args["selection_by_title"] = v
		}
		if v := runtime.Str("new-title"); v != "" {
			args["new_title"] = v
		}
		return common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: update-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "update-doc", "arguments": args}}).
			Set("mcp_tool", "update-doc").Set("args", args)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
			"mode":   runtime.Str("mode"),
		}
		if v := runtime.Str("markdown"); v != "" {
			args["markdown"] = v
		}
		if v := runtime.Str("selection-with-ellipsis"); v != "" {
			args["selection_with_ellipsis"] = v
		}
		if v := runtime.Str("selection-by-title"); v != "" {
			args["selection_by_title"] = v
		}
		if v := runtime.Str("new-title"); v != "" {
			args["new_title"] = v
		}

		result, err := common.CallMCPTool(runtime, "update-doc", args)
		if err != nil {
			return err
		}

		normalizeDocsUpdateResult(result, runtime.Str("markdown"))
		if runtime.Str("mode") == "overwrite" && runtime.Bool("auto-table-widths") {
			if docID, ok := docxTokenForUpdate(runtime.Str("doc")); ok {
				applyMarkdownTableColumnWidths(runtime, docID, runtime.Str("markdown"))
			} else {
				fmt.Fprintln(
					runtime.IO().ErrOut,
					"auto-table-widths skipped: --doc is not a docx token/URL (wiki-backed docs require an extra resolve step that is not implemented yet)",
				)
			}
		}
		runtime.Out(result, nil)
		return nil
	},
}

func normalizeDocsUpdateResult(result map[string]interface{}, markdown string) {
	if !isWhiteboardCreateMarkdown(markdown) {
		return
	}
	result["board_tokens"] = normalizeBoardTokens(result["board_tokens"])
}

func isWhiteboardCreateMarkdown(markdown string) bool {
	lower := strings.ToLower(markdown)
	if strings.Contains(lower, "```mermaid") || strings.Contains(lower, "```plantuml") {
		return true
	}
	return strings.Contains(lower, "<whiteboard") &&
		(strings.Contains(lower, `type="blank"`) || strings.Contains(lower, `type='blank'`))
}

func normalizeBoardTokens(raw interface{}) []string {
	switch v := raw.(type) {
	case nil:
		return []string{}
	case []string:
		return v
	case []interface{}:
		tokens := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				tokens = append(tokens, s)
			}
		}
		return tokens
	case string:
		if v == "" {
			return []string{}
		}
		return []string{v}
	default:
		return []string{}
	}
}
