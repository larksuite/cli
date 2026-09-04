// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestContainsUnsafeMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		unsafe bool
	}{
		{
			name:   "fenced code block with backticks",
			input:  "```go\nfmt.Println()\n```",
			unsafe: true,
		},
		{
			name:   "fenced code block with tildes",
			input:  "~~~\ncode\n~~~",
			unsafe: true,
		},
		{
			name:   "blockquote prefix",
			input:  "> This is a blockquote",
			unsafe: true,
		},
		{
			name:   "nested blockquotes",
			input:  "> outer\n> > inner",
			unsafe: true,
		},
		{
			name:   "HTML div tag",
			input:  "<div>content</div>",
			unsafe: true,
		},
		{
			name:   "HTML closing tag",
			input:  "text</p>",
			unsafe: true,
		},
		{
			name:   "HTML comment",
			input:  "<!-- comment -->",
			unsafe: true,
		},
		{
			name:   "Lark callout tag",
			input:  `<callout type="warning">`,
			unsafe: true,
		},
		{
			name:   "real markdown table with leading/trailing pipes",
			input:  "| a | b |\n|---|---|\n| 1 | 2 |",
			unsafe: true,
		},
		{
			name:   "real markdown table WITHOUT leading/trailing pipes",
			input:  "Column A | Column B\n---|---\nValue 1 | Value 2",
			unsafe: true,
		},
		{
			name:   "real markdown table with colons alignment",
			input:  "| name | age |\n|:---|---:|\n| Bob | 30 |",
			unsafe: true,
		},
		{
			name:   "table WITHOUT leading/trailing pipes with alignment",
			input:  "name | age\n:---|---:|\nBob | 30",
			unsafe: true,
		},
		{
			name:   "alignment row alone is detected",
			input:  "some text\n---|---\nmore text",
			unsafe: true,
		},
		{
			name:   "casual pipe use is NOT a table",
			input:  "选项 A | 选项 B",
			unsafe: false,
		},
		{
			name:   "single pipe in sentence is NOT a table",
			input:  "Use pipe | in prose",
			unsafe: false,
		},
		{
			name:   "angle bracket in comparison is NOT HTML",
			input:  "if a < b && b < c",
			unsafe: false,
		},
		{
			name:   "angle bracket followed by digit is safe",
			input:  "value <3 things",
			unsafe: false,
		},
		{
			name:   "HTML after comparison operator is caught",
			input:  "if x < 10 <div>content</div>",
			unsafe: true,
		},
		{
			name:   "multiple HTML tags are caught",
			input:  "<div>first</div> text <span>second</span>",
			unsafe: true,
		},
		{
			name:   "heading h1 is safe",
			input:  "# Heading",
			unsafe: false,
		},
		{
			name:   "heading h6 is safe",
			input:  "###### Sub Sub Sub",
			unsafe: false,
		},
		{
			name:   "plain paragraph is safe",
			input:  "Just a plain paragraph of text.",
			unsafe: false,
		},
		{
			name:   "bold and italic are safe",
			input:  "This is **bold** and *italic* text.",
			unsafe: false,
		},
		{
			name:   "inline code is safe",
			input:  "Use `fmt.Println()` to print.",
			unsafe: false,
		},
		{
			name:   "unordered list is safe",
			input:  "- item 1\n- item 2\n- item 3",
			unsafe: false,
		},
		{
			name:   "ordered list is safe",
			input:  "1. first\n2. second\n3. third",
			unsafe: false,
		},
		{
			name:   "empty string is safe",
			input:  "",
			unsafe: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsUnsafeMarkdown(tt.input)
			if got != tt.unsafe {
				t.Errorf("containsUnsafeMarkdown(%q) = %v, want %v", tt.input, got, tt.unsafe)
			}
		})
	}
}

func TestIsTableAlignmentRow(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"basic dashes with pipes", "|---|", true},
		{"dashes without pipes", "---|---", true},
		{"dashes with spaces", "| --- |", true},
		{"dashes with multiple spaces", "|    ---    |", true},
		{"left align colon", "| :--- |", true},
		{"right align colon", "| ---: |", true},
		{"center align colon", "| :---: |", true},
		{"mixed alignments", "| :--- | ---: |", true},
		{"mixed alignments without outer pipes", ":--- | ---:", true},
		{"no leading pipe", "---|---|", true},
		{"no trailing pipe", "|---|---", true},
		{"no pipe at all", "--- ---", true},
		{"text instead of dashes", "| abc |", false},
		{"colons only", "| :: |", false},
		{"plain text line", "some text", false},
		{"empty string", "", false},
		{"just pipes", "||", false},
		{"table row without alignment", "| cell | cell |", false},
		{"tabs allowed", "---\t|---", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTableAlignmentRow(tt.input); got != tt.want {
				t.Errorf("isTableAlignmentRow(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestChunkMarkdownForUpload_Idempotency(t *testing.T) {
	t.Run("short content with code block passes unchanged", func(t *testing.T) {
		shortWithCode := "```go\nfmt.Println()\n```\n\nshort text"
		result, err := chunkMarkdownForUpload(shortWithCode)
		if err != nil {
			t.Errorf("expected no error for short content, got %v", err)
		}
		if result != shortWithCode {
			t.Errorf("expected unchanged output, got %q", result)
		}
	})

	t.Run("short content with table passes unchanged", func(t *testing.T) {
		shortWithTable := "| a | b |\n|---|---|\n| 1 | 2 |"
		result, err := chunkMarkdownForUpload(shortWithTable)
		if err != nil {
			t.Errorf("expected no error for short content, got %v", err)
		}
		if result != shortWithTable {
			t.Errorf("expected unchanged output, got %q", result)
		}
	})

	t.Run("short content with blockquote passes unchanged", func(t *testing.T) {
		shortWithQuote := "> This is a quote\n\nshort text"
		result, err := chunkMarkdownForUpload(shortWithQuote)
		if err != nil {
			t.Errorf("expected no error for short content, got %v", err)
		}
		if result != shortWithQuote {
			t.Errorf("expected unchanged output, got %q", result)
		}
	})

	t.Run("exactly at limit passes unchanged", func(t *testing.T) {
		exactLimit := strings.Repeat("a", SafeParagraphLimit)
		result, err := chunkMarkdownForUpload(exactLimit)
		if err != nil {
			t.Errorf("expected no error for exact limit, got %v", err)
		}
		if result != exactLimit {
			t.Errorf("expected unchanged output")
		}
	})

	t.Run("one under limit passes unchanged", func(t *testing.T) {
		underLimit := strings.Repeat("a", SafeParagraphLimit-1)
		result, err := chunkMarkdownForUpload(underLimit)
		if err != nil {
			t.Errorf("expected no error for under limit, got %v", err)
		}
		if result != underLimit {
			t.Errorf("expected unchanged output")
		}
	})
}

func TestChunkMarkdownForUpload_Rejection(t *testing.T) {
	t.Run("oversized content with code block is rejected", func(t *testing.T) {
		oversizedWithCode := strings.Repeat("word ", 3000) + "\n\n```go\ncode\n```"
		_, err := chunkMarkdownForUpload(oversizedWithCode)
		if !errors.Is(err, ErrUnsafeMarkdown) {
			t.Errorf("expected ErrUnsafeMarkdown, got %v", err)
		}
	})

	t.Run("oversized content with table is rejected", func(t *testing.T) {
		oversizedWithTable := strings.Repeat("word ", 3000) + "\n\n| a | b |\n|---|---|\n| 1 | 2 |"
		_, err := chunkMarkdownForUpload(oversizedWithTable)
		if !errors.Is(err, ErrUnsafeMarkdown) {
			t.Errorf("expected ErrUnsafeMarkdown, got %v", err)
		}
	})

	t.Run("oversized content with table without outer pipes is rejected", func(t *testing.T) {
		oversizedWithTable := strings.Repeat("word ", 3000) + "\n\nA | B\n---|---\n1 | 2"
		_, err := chunkMarkdownForUpload(oversizedWithTable)
		if !errors.Is(err, ErrUnsafeMarkdown) {
			t.Errorf("expected ErrUnsafeMarkdown, got %v", err)
		}
	})

	t.Run("oversized content with blockquote is rejected", func(t *testing.T) {
		oversizedWithQuote := strings.Repeat("word ", 3000) + "\n\n> This is a quote"
		_, err := chunkMarkdownForUpload(oversizedWithQuote)
		if !errors.Is(err, ErrUnsafeMarkdown) {
			t.Errorf("expected ErrUnsafeMarkdown, got %v", err)
		}
	})

	t.Run("oversized content with HTML is rejected", func(t *testing.T) {
		oversizedWithHTML := strings.Repeat("word ", 3000) + "\n\n<div>content</div>"
		_, err := chunkMarkdownForUpload(oversizedWithHTML)
		if !errors.Is(err, ErrUnsafeMarkdown) {
			t.Errorf("expected ErrUnsafeMarkdown, got %v", err)
		}
	})
}

func TestSplitPlainParagraphs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single paragraph",
			input: "hello world",
			want:  []string{"hello world"},
		},
		{
			name:  "two paragraphs separated by blank line",
			input: "paragraph one\n\nparagraph two",
			want:  []string{"paragraph one", "paragraph two"},
		},
		{
			name:  "list items as single paragraph (no blank lines)",
			input: "- item 1\n- item 2\n- item 3",
			want:  []string{"- item 1\n- item 2\n- item 3"},
		},
		{
			name:  "list with blank line between items becomes two paragraphs",
			input: "- item 1\n\n- item 2",
			want:  []string{"- item 1", "- item 2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPlainParagraphs(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("splitPlainParagraphs(%q) returned %d paragraphs, want %d", tt.input, len(got), len(tt.want))
				return
			}
			for i, p := range got {
				if p != tt.want[i] {
					t.Errorf("splitPlainParagraphs(%q)[%d] = %q, want %q", tt.input, i, p, tt.want[i])
				}
			}
		})
	}
}

func TestSplitOversizedParagraph(t *testing.T) {
	t.Run("paragraph under limit is not split", func(t *testing.T) {
		para := strings.Repeat("a", SafeParagraphLimit)
		chunks := splitOversizedParagraph(para)
		if len(chunks) != 1 {
			t.Errorf("expected 1 chunk for exact limit, got %d", len(chunks))
		}
	})

	t.Run("paragraph over limit is split", func(t *testing.T) {
		para := strings.Repeat("word ", 3000)
		chunks := splitOversizedParagraph(para)
		if len(chunks) < 2 {
			t.Errorf("expected multiple chunks, got %d", len(chunks))
		}
		for i, chunk := range chunks {
			if utf8.RuneCountInString(chunk) > SafeParagraphLimit {
				t.Errorf("chunk %d exceeds limit: %d runes", i, utf8.RuneCountInString(chunk))
			}
		}
	})
}

func TestChunkMarkdownChineseUTF8(t *testing.T) {
	t.Run("Chinese text is split correctly", func(t *testing.T) {
		longChinese := strings.Repeat("你好世界", 3000)
		result, err := chunkMarkdownForUpload(longChinese)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !utf8.ValidString(result) {
			t.Error("result is not valid UTF-8")
		}
	})

	t.Run("mixed Chinese and ASCII content", func(t *testing.T) {
		mixed := strings.Repeat("Hello 你好 ", 2000)
		result, err := chunkMarkdownForUpload(mixed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !utf8.ValidString(result) {
			t.Error("result is not valid UTF-8")
		}
	})
}

func TestChunkMarkdownEmojiSafety(t *testing.T) {
	t.Run("emoji in text does not corrupt UTF-8", func(t *testing.T) {
		emojiText := strings.Repeat("Hello 🌍🌎🌏 ", 2000)
		result, err := chunkMarkdownForUpload(emojiText)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !utf8.ValidString(result) {
			t.Error("result is not valid UTF-8")
		}
	})
}

func TestChunkMarkdownMultiParagraph(t *testing.T) {
	t.Run("short paragraphs are preserved", func(t *testing.T) {
		short1 := "Short paragraph one."
		short2 := "Short paragraph two."
		longPara := strings.Repeat("word ", 3000)

		md := short1 + "\n\n" + longPara + "\n\n" + short2
		result, err := chunkMarkdownForUpload(md)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result, short1) {
			t.Error("short paragraph 1 was lost")
		}
		if !strings.Contains(result, short2) {
			t.Error("short paragraph 2 was lost")
		}
	})

	t.Run("all paragraphs under limit pass through unchanged", func(t *testing.T) {
		md := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
		result, err := chunkMarkdownForUpload(md)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != md {
			t.Errorf("expected unchanged output, got %q", result)
		}
	})
}

func TestRuneOffset(t *testing.T) {
	t.Run("ASCII string", func(t *testing.T) {
		got := runeOffset("hello", 3)
		if got != 3 {
			t.Errorf("runeOffset(\"hello\", 3) = %d, want 3", got)
		}
	})

	t.Run("Chinese string", func(t *testing.T) {
		got := runeOffset("你好世界", 2)
		if got != 6 {
			t.Errorf("runeOffset(\"你好世界\", 2) = %d, want 6", got)
		}
	})
}

func TestFindSplitPoint(t *testing.T) {
	t.Run("finds newline in search range", func(t *testing.T) {
		s := strings.Repeat("a", 8000) + "\n" + strings.Repeat("b", 2000)
		limitByte := runeOffset(s, SafeParagraphLimit)
		splitAt := findSplitPoint(s, limitByte)
		if s[splitAt-1] != '\n' {
			t.Error("expected split at newline")
		}
	})

	t.Run("falls back to limit byte when no break found", func(t *testing.T) {
		s := strings.Repeat("a", SafeParagraphLimit+5000)
		limitByte := runeOffset(s, SafeParagraphLimit)
		splitAt := findSplitPoint(s, limitByte)
		if splitAt != limitByte {
			t.Errorf("findSplitPoint = %d, want %d", splitAt, limitByte)
		}
	})
}

func TestApplyChunkingToBody(t *testing.T) {
	t.Run("non-markdown format is skipped", func(t *testing.T) {
		body := map[string]interface{}{"content": strings.Repeat("a", SafeParagraphLimit+1), "format": "xml"}
		err := applyChunkingToBody(body, "content", "xml")
		if err != nil {
			t.Errorf("expected no error for xml format, got %v", err)
		}
		if body["content"] != strings.Repeat("a", SafeParagraphLimit+1) {
			t.Error("xml content should not be modified")
		}
	})

	t.Run("empty content is skipped", func(t *testing.T) {
		body := map[string]interface{}{"content": "", "format": "markdown"}
		err := applyChunkingToBody(body, "content", "markdown")
		if err != nil {
			t.Errorf("expected no error for empty content, got %v", err)
		}
	})

	t.Run("short markdown content passes unchanged", func(t *testing.T) {
		body := map[string]interface{}{"content": "short text", "format": "markdown"}
		err := applyChunkingToBody(body, "content", "markdown")
		if err != nil {
			t.Errorf("expected no error for short content, got %v", err)
		}
		if body["content"] != "short text" {
			t.Errorf("expected unchanged content, got %q", body["content"])
		}
	})

	t.Run("oversized markdown with unsafe content returns error", func(t *testing.T) {
		body := map[string]interface{}{"content": strings.Repeat("word ", 3000) + "\n\n```go\ncode\n```", "format": "markdown"}
		err := applyChunkingToBody(body, "content", "markdown")
		if err == nil {
			t.Error("expected error for oversized unsafe content")
		}
	})
}
