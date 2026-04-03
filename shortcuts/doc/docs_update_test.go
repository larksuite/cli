// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"reflect"
	"testing"
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

func TestShouldAutoResizeAfterUpdate(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		markdown string
		want     bool
	}{
		{
			name:     "append markdown table",
			mode:     "append",
			markdown: "| A | B |\n| --- | --- |\n| 1 | 2 |",
			want:     true,
		},
		{
			name:     "replace range html table",
			mode:     "replace_range",
			markdown: "<table><tr><td>A</td></tr></table>",
			want:     true,
		},
		{
			name:     "plain markdown",
			mode:     "append",
			markdown: "## plain text",
			want:     false,
		},
		{
			name:     "delete range",
			mode:     "delete_range",
			markdown: "| A |",
			want:     false,
		},
		{
			name:     "pipe command not mistaken for table",
			mode:     "append",
			markdown: "run cmd1 | grep foo | wc -l",
			want:     false,
		},
		{
			name:     "borderless markdown table",
			mode:     "append",
			markdown: "A | B | C\n--|--|--\n1 | 2 | 3",
			want:     true,
		},
		{
			name:     "pipe command still not mistaken for table",
			mode:     "append",
			markdown: "run cmd1 | grep foo | wc -l",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldAutoResizeAfterUpdate(tt.mode, tt.markdown); got != tt.want {
				t.Fatalf("shouldAutoResizeAfterUpdate(%q, %q) = %v, want %v", tt.mode, tt.markdown, got, tt.want)
			}
		})
	}
}
