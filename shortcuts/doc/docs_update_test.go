// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"reflect"
	"strings"
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

func TestValidateSelectionByTitle(t *testing.T) {
	t.Run("empty title passes", func(t *testing.T) {
		if err := validateSelectionByTitle(""); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("heading style title passes", func(t *testing.T) {
		if err := validateSelectionByTitle("## 第二章"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("plain text title fails with guidance", func(t *testing.T) {
		err := validateSelectionByTitle("第二章")
		if err == nil {
			t.Fatalf("expected validation error")
		}
		if got := err.Error(); got == "" || !containsAll(got, "selection-by-title", "heading prefix") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("multi-line heading still fails", func(t *testing.T) {
		err := validateSelectionByTitle("## 第二章\n## 第三章")
		if err == nil {
			t.Fatalf("expected validation error")
		}
		if got := err.Error(); got == "" || !containsAll(got, "single heading line") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("multi-line title fails", func(t *testing.T) {
		err := validateSelectionByTitle("第二章\n第三章")
		if err == nil {
			t.Fatalf("expected validation error")
		}
		if got := err.Error(); got == "" || !containsAll(got, "single heading line") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func containsAll(s string, tokens ...string) bool {
	for _, token := range tokens {
		if !strings.Contains(s, token) {
			return false
		}
	}
	return true
}
