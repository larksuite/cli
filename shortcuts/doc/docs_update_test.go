// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestIsWhiteboardCreateMarkdown(t *testing.T) {
	t.Run("blank whiteboard tags", func(t *testing.T) {
		markdown := "<whiteboard type=\"blank\"></whiteboard>\n<whiteboard type=\"blank\"></whiteboard>"
		if !isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("expected blank whiteboard markdown to be treated as whiteboard creation")
		}
	})

	t.Run("mermaid code block", func(t *testing.T) {
		markdown := "```mermaid\ngraph TD\nA-->B\n```"
		if !isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("expected mermaid markdown to be treated as whiteboard creation")
		}
	})

	t.Run("plain markdown", func(t *testing.T) {
		markdown := "## plain text"
		if isWhiteboardCreateMarkdown(markdown) {
			t.Fatalf("did not expect plain markdown to be treated as whiteboard creation")
		}
	})
}

func TestNormalizeDocsUpdateResult(t *testing.T) {
	t.Run("adds empty board_tokens when whiteboard creation response omits it", func(t *testing.T) {
		result := map[string]interface{}{
			"success": true,
		}

		normalizeDocsUpdateResult(result, "<whiteboard type=\"blank\"></whiteboard>")

		got, ok := result["board_tokens"].([]string)
		if !ok {
			t.Fatalf("expected board_tokens to be []string, got %T", result["board_tokens"])
		}
		if len(got) != 0 {
			t.Fatalf("expected empty board_tokens, got %#v", got)
		}
	})

	t.Run("normalizes board_tokens to string slice", func(t *testing.T) {
		result := map[string]interface{}{
			"board_tokens": []interface{}{"board_1", "board_2"},
		}

		normalizeDocsUpdateResult(result, "<whiteboard type=\"blank\"></whiteboard>")

		want := []string{"board_1", "board_2"}
		got, ok := result["board_tokens"].([]string)
		if !ok {
			t.Fatalf("expected board_tokens to be []string, got %T", result["board_tokens"])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("board_tokens mismatch: got %#v want %#v", got, want)
		}
	})

	t.Run("leaves non whiteboard response unchanged", func(t *testing.T) {
		result := map[string]interface{}{
			"success": true,
		}

		normalizeDocsUpdateResult(result, "## plain text")

		if _, ok := result["board_tokens"]; ok {
			t.Fatalf("did not expect board_tokens for non-whiteboard markdown")
		}
	})
}

func TestSectionMarkdownByTitle(t *testing.T) {
	markdown := "## 第一章\n\n第一章内容。\n\n## 第二章\n\n第二章内容。\n\n### 第二章\n\n子章节。\n\n## 第三章\n\n第三章内容。\n"

	t.Run("matches title without heading markers", func(t *testing.T) {
		got, ok := sectionMarkdownByTitle(markdown, "第二章")
		if !ok {
			t.Fatalf("expected title match to succeed")
		}
		want := "## 第二章\n\n第二章内容。\n\n### 第二章\n\n子章节。\n\n"
		if got != want {
			t.Fatalf("sectionMarkdownByTitle() = %q, want %q", got, want)
		}
	})

	t.Run("matches exact level when hashes are provided", func(t *testing.T) {
		got, ok := sectionMarkdownByTitle(markdown, "### 第二章")
		if !ok {
			t.Fatalf("expected exact-level title match to succeed")
		}
		want := "### 第二章\n\n子章节。\n\n"
		if got != want {
			t.Fatalf("sectionMarkdownByTitle() = %q, want %q", got, want)
		}
	})

	t.Run("ignores headings inside fenced code blocks", func(t *testing.T) {
		md := "## 第一章\n\n```markdown\n## 假标题\n```\n\n## 第二章\n\n正文\n"
		got, ok := sectionMarkdownByTitle(md, "第二章")
		if !ok {
			t.Fatalf("expected title match to succeed")
		}
		want := "## 第二章\n\n正文\n"
		if got != want {
			t.Fatalf("sectionMarkdownByTitle() = %q, want %q", got, want)
		}
	})
}

func TestResolveTitleSelection(t *testing.T) {
	t.Run("rewrites title selection using fetched markdown", func(t *testing.T) {
		runtime := newDocsUpdateTestRuntime(t, map[string]string{
			"doc":                "doc_123",
			"mode":               "delete_range",
			"selection-by-title": "第二章",
		})

		restore := stubDocsCallMCPTool(t, func(_ *common.RuntimeContext, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
			switch toolName {
			case "fetch-doc":
				if args["doc_id"] != "doc_123" {
					t.Fatalf("fetch-doc doc_id = %#v, want %q", args["doc_id"], "doc_123")
				}
				return map[string]interface{}{
					"markdown": "## 第一章\n\n第一章内容。\n\n## 第二章\n\n第二章内容。\n",
				}, nil
			default:
				t.Fatalf("unexpected tool call %q", toolName)
				return nil, nil
			}
		})
		defer restore()

		got, err := resolveTitleSelection(runtime, buildDocsUpdateArgs(runtime))
		if err != nil {
			t.Fatalf("resolveTitleSelection() error = %v", err)
		}
		if _, ok := got["selection_by_title"]; ok {
			t.Fatalf("did not expect selection_by_title after rewrite: %#v", got)
		}
		want := "## 第二章\n\n第二章内容。\n"
		if got["selection_with_ellipsis"] != want {
			t.Fatalf("selection_with_ellipsis = %#v, want %#v", got["selection_with_ellipsis"], want)
		}
	})

	t.Run("falls back to original title selection when fetch fails", func(t *testing.T) {
		runtime := newDocsUpdateTestRuntime(t, map[string]string{
			"doc":                "doc_123",
			"mode":               "delete_range",
			"selection-by-title": "第二章",
		})

		restore := stubDocsCallMCPTool(t, func(_ *common.RuntimeContext, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
			if toolName != "fetch-doc" {
				t.Fatalf("unexpected tool call %q", toolName)
			}
			return nil, errors.New("network down")
		})
		defer restore()

		got, err := resolveTitleSelection(runtime, buildDocsUpdateArgs(runtime))
		if err != nil {
			t.Fatalf("resolveTitleSelection() error = %v", err)
		}
		if got["selection_by_title"] != "第二章" {
			t.Fatalf("selection_by_title = %#v, want %#v", got["selection_by_title"], "第二章")
		}
		if _, ok := got["selection_with_ellipsis"]; ok {
			t.Fatalf("did not expect selection_with_ellipsis on fallback: %#v", got)
		}
	})
}

func newDocsUpdateTestRuntime(t *testing.T, flags map[string]string) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	for name := range flags {
		cmd.Flags().String(name, "", "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, val := range flags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	return &common.RuntimeContext{
		Cmd:    cmd,
		Config: &core.CliConfig{Brand: core.BrandLark},
	}
}

func stubDocsCallMCPTool(t *testing.T, stub func(*common.RuntimeContext, string, map[string]interface{}) (map[string]interface{}, error)) func() {
	t.Helper()

	original := docsCallMCPTool
	docsCallMCPTool = stub
	return func() {
		docsCallMCPTool = original
	}
}
