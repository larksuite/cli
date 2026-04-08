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

		if shouldAutoResizeAfterUpdate(runtime.Str("mode"), runtime.Str("markdown")) {
			docID := common.GetString(result, "doc_id")
			if docID == "" {
				resolvedDocID, resolveErr := resolveDocxDocumentID(runtime, runtime.Str("doc"), "table auto-width")
				if resolveErr != nil {
					fmt.Fprintf(runtime.IO().ErrOut, "warning: table auto-width skipped: %v\n", resolveErr)
				} else {
					docID = resolvedDocID
				}
			}
			if docID != "" {
				if warn := autoResizeTableColumns(runtime, docID); warn != "" {
					fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", warn)
				}
			}
		}

		runtime.Out(result, nil)
		return nil
	},
}

// normalizeDocsUpdateResult normalizes tool output for markdown that creates whiteboards.
func normalizeDocsUpdateResult(result map[string]interface{}, markdown string) {
	if !isWhiteboardCreateMarkdown(markdown) {
		return
	}
	result["board_tokens"] = normalizeBoardTokens(result["board_tokens"])
}

// isWhiteboardCreateMarkdown reports whether markdown requests whiteboard creation semantics.
func isWhiteboardCreateMarkdown(markdown string) bool {
	lower := strings.ToLower(markdown)
	if strings.Contains(lower, "```mermaid") || strings.Contains(lower, "```plantuml") {
		return true
	}
	return strings.Contains(lower, "<whiteboard") &&
		(strings.Contains(lower, `type="blank"`) || strings.Contains(lower, `type='blank'`))
}

// normalizeBoardTokens converts the tool result field to a stable []string shape.
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

// shouldAutoResizeAfterUpdate decides whether an update operation should run table auto-width.
func shouldAutoResizeAfterUpdate(mode, markdown string) bool {
	if mode == "delete_range" || strings.TrimSpace(markdown) == "" {
		return false
	}
	return markdownLikelyContainsTable(markdown)
}

// markdownLikelyContainsTable uses lightweight heuristics to detect markdown or HTML tables.
func markdownLikelyContainsTable(markdown string) bool {
	filtered := stripFencedCodeBlocks(markdown)
	lower := strings.ToLower(filtered)
	if strings.Contains(lower, "<table") {
		return true
	}

	lines := strings.Split(filtered, "\n")
	for i := 0; i < len(lines)-1; i++ {
		if isMarkdownTableRow(lines[i]) && isMarkdownTableSeparator(lines[i+1]) {
			return true
		}
	}
	return false
}

// stripFencedCodeBlocks removes fenced code blocks so table heuristics ignore code samples.
func stripFencedCodeBlocks(markdown string) string {
	lines := strings.Split(markdown, "\n")
	kept := make([]string, 0, len(lines))
	inFence := false
	fenceMarker := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if marker, ok := fencedCodeMarker(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
				continue
			}
			if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
				continue
			}
		}
		if !inFence {
			kept = append(kept, line)
		}
	}

	return strings.Join(kept, "\n")
}

// fencedCodeMarker returns the fence marker used to enter or exit a fenced code block.
func fencedCodeMarker(trimmed string) (string, bool) {
	for _, base := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, base) {
			marker := base
			for i := len(base); i < len(trimmed) && trimmed[i] == base[0]; i++ {
				marker += string(base[0])
			}
			return marker, true
		}
	}
	return "", false
}

// isMarkdownTableRow checks whether a line looks like a markdown table header or row.
func isMarkdownTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	if strings.Count(trimmed, "|") < 1 {
		return false
	}
	return strings.Trim(trimmed, "| :-\t") != ""
}

// isMarkdownTableSeparator checks whether a line looks like a markdown table separator row.
func isMarkdownTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") || !strings.Contains(trimmed, "-") {
		return false
	}
	for _, r := range trimmed {
		switch r {
		case '|', ':', '-', ' ', '\t':
		default:
			return false
		}
	}
	return true
}
