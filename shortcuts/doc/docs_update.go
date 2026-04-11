// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"regexp"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var docsCallMCPTool = common.CallMCPTool

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
		args := buildDocsUpdateArgs(runtime)
		return common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: update-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "update-doc", "arguments": args}}).
			Set("mcp_tool", "update-doc").Set("args", args)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args, err := resolveTitleSelection(runtime, buildDocsUpdateArgs(runtime))
		if err != nil {
			return err
		}

		result, err := docsCallMCPTool(runtime, "update-doc", args)
		if err != nil {
			return err
		}

		normalizeDocsUpdateResult(result, runtime.Str("markdown"))
		runtime.Out(result, nil)
		return nil
	},
}

func buildDocsUpdateArgs(runtime *common.RuntimeContext) map[string]interface{} {
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
	return args
}

func resolveTitleSelection(runtime *common.RuntimeContext, args map[string]interface{}) (map[string]interface{}, error) {
	rawSelection, ok := args["selection_by_title"].(string)
	if !ok || strings.TrimSpace(rawSelection) == "" {
		return args, nil
	}

	docID, _ := args["doc_id"].(string)
	if strings.TrimSpace(docID) == "" {
		return args, nil
	}

	result, err := docsCallMCPTool(runtime, "fetch-doc", map[string]interface{}{"doc_id": docID})
	if err != nil {
		// Keep the existing server-side title path if the fetch workaround is unavailable.
		return args, nil
	}

	markdown, _ := result["markdown"].(string)
	section, ok := sectionMarkdownByTitle(markdown, rawSelection)
	if !ok {
		return args, nil
	}

	rewritten := make(map[string]interface{}, len(args))
	for k, v := range args {
		if k == "selection_by_title" {
			continue
		}
		rewritten[k] = v
	}
	rewritten["selection_with_ellipsis"] = escapeSelectionLiteral(section)
	return rewritten, nil
}

type markdownHeading struct {
	level int
	title string
	start int
	end   int
}

var headingLinePattern = regexp.MustCompile(`^\s{0,3}(#{1,6})[ \t]+(.+?)[ \t]*#*[ \t]*$`)

func sectionMarkdownByTitle(markdown, selection string) (string, bool) {
	headings := collectMarkdownHeadings(markdown)
	if len(headings) == 0 {
		return "", false
	}

	title, level, exactLevel := parseTitleSelection(selection)
	if title == "" {
		return "", false
	}

	for _, heading := range headings {
		if heading.title != title {
			continue
		}
		if exactLevel && heading.level != level {
			continue
		}
		return markdown[heading.start:heading.end], true
	}
	return "", false
}

func collectMarkdownHeadings(markdown string) []markdownHeading {
	var headings []markdownHeading
	inFence := false
	fenceMarker := ""

	for start, line := range iterateMarkdownLines(markdown) {
		trimmed := strings.TrimSpace(line)
		if marker, ok := markdownFenceMarker(trimmed); ok {
			if !inFence {
				inFence = true
				fenceMarker = marker
			} else if marker == fenceMarker {
				inFence = false
				fenceMarker = ""
			}
			continue
		}
		if inFence {
			continue
		}
		matches := headingLinePattern.FindStringSubmatch(trimmed)
		if len(matches) != 3 {
			continue
		}
		title := strings.TrimSpace(matches[2])
		level := len(matches[1])
		if title == "" {
			continue
		}
		headings = append(headings, markdownHeading{
			level: level,
			title: title,
			start: start,
			end:   len(markdown),
		})
	}

	for i := range headings {
		for j := i + 1; j < len(headings); j++ {
			if headings[j].level <= headings[i].level {
				headings[i].end = headings[j].start
				break
			}
		}
	}
	return headings
}

func iterateMarkdownLines(markdown string) func(func(int, string) bool) {
	return func(yield func(int, string) bool) {
		start := 0
		for start < len(markdown) {
			end := start
			for end < len(markdown) && markdown[end] != '\n' {
				end++
			}
			if end < len(markdown) {
				end++
			}
			if !yield(start, markdown[start:end]) {
				return
			}
			start = end
		}
	}
}

func markdownFenceMarker(trimmedLine string) (string, bool) {
	switch {
	case strings.HasPrefix(trimmedLine, "```"):
		return "```", true
	case strings.HasPrefix(trimmedLine, "~~~"):
		return "~~~", true
	default:
		return "", false
	}
}

func parseTitleSelection(selection string) (title string, level int, exactLevel bool) {
	trimmed := strings.TrimSpace(selection)
	if trimmed == "" {
		return "", 0, false
	}
	matches := headingLinePattern.FindStringSubmatch(trimmed)
	if len(matches) == 3 {
		return strings.TrimSpace(matches[2]), len(matches[1]), true
	}
	return trimmed, 0, false
}

func escapeSelectionLiteral(section string) string {
	return strings.ReplaceAll(section, "...", `\.\.\.`)
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
