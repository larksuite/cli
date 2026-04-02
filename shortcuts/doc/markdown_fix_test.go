// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestFixBoldSpacing(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "trailing space before closing bold",
			input: "**hello **",
			want:  "**hello**",
		},
		{
			name:  "trailing space before closing italic",
			input: "*hello *",
			want:  "*hello*",
		},
		{
			name:  "redundant bold in h1",
			input: "# **Title**",
			want:  "# Title",
		},
		{
			name:  "redundant bold in h2",
			input: "## **Section**",
			want:  "## Section",
		},
		{
			name:  "no change needed for clean bold",
			input: "**bold**",
			want:  "**bold**",
		},
		{
			name:  "multiple lines processed independently",
			input: "**foo **\n**bar **",
			want:  "**foo**\n**bar**",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixBoldSpacing(tt.input)
			if got != tt.want {
				t.Errorf("fixBoldSpacing(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixSetextAmbiguity(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "paragraph followed by ---",
			input: "some text\n---",
			want:  "some text\n\n---",
		},
		{
			name:  "blank line before --- already",
			input: "some text\n\n---",
			want:  "some text\n\n---",
		},
		{
			name:  "heading not affected",
			input: "# Heading\n---",
			want:  "# Heading\n\n---",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixSetextAmbiguity(tt.input)
			if got != tt.want {
				t.Errorf("fixSetextAmbiguity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixBlockquoteHardBreaks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "two consecutive blockquote lines",
			input: "> line1\n> line2",
			want:  "> line1\n>\n> line2",
		},
		{
			name:  "three consecutive blockquote lines",
			input: "> a\n> b\n> c",
			want:  "> a\n>\n> b\n>\n> c",
		},
		{
			name:  "single blockquote line unchanged",
			input: "> only one",
			want:  "> only one",
		},
		{
			name:  "non-blockquote not affected",
			input: "line1\nline2",
			want:  "line1\nline2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixBlockquoteHardBreaks(tt.input)
			if got != tt.want {
				t.Errorf("fixBlockquoteHardBreaks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixTopLevelSoftbreaks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "adjacent top-level lines get blank line",
			input: "paragraph one\nparagraph two",
			want:  "paragraph one\n\nparagraph two",
		},
		{
			name:  "lines inside code block not modified",
			input: "```\nline1\nline2\n```",
			want:  "```\nline1\nline2\n```",
		},
		{
			// callout is a content container: blank lines are inserted between inner lines.
			name:  "lines inside callout get blank line between them",
			input: "<callout>\nline1\nline2\n</callout>",
			want:  "<callout>\n\nline1\n\nline2\n</callout>",
		},
		{
			name:  "lark-td cell content gets blank line",
			input: "<lark-td>\nline1\nline2\n</lark-td>",
			want:  "<lark-td>\nline1\n\nline2\n</lark-td>",
		},
		{
			name:  "structural lark-table tags not separated",
			input: "<lark-table>\n<lark-tr>\n<lark-td>\ncontent\n</lark-td>\n</lark-tr>\n</lark-table>",
			want:  "<lark-table>\n<lark-tr>\n<lark-td>\ncontent\n</lark-td>\n</lark-tr>\n</lark-table>",
		},
		{
			name:  "blockquote lines not split",
			input: "> line1\n> line2",
			want:  "> line1\n> line2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixTopLevelSoftbreaks(tt.input)
			if got != tt.want {
				t.Errorf("fixTopLevelSoftbreaks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixExportedMarkdown(t *testing.T) {
	// End-to-end: all fixes applied together
	input := "# **Title**\nparagraph one\nparagraph two\n**bold **\n> q1\n> q2\nsome text\n---"
	result := fixExportedMarkdown(input)

	if strings.Contains(result, "# **Title**") {
		t.Error("expected heading bold to be stripped")
	}
	if !strings.Contains(result, "paragraph one\n\nparagraph two") {
		t.Error("expected blank line between top-level paragraphs")
	}
	if strings.Contains(result, "**bold **") {
		t.Error("expected trailing space in bold to be fixed")
	}
	if !strings.Contains(result, ">\n> q2") {
		t.Error("expected blockquote hard break inserted")
	}
	if strings.Contains(result, "some text\n---") {
		t.Error("expected blank line before --- to prevent setext heading")
	}
	// Should end with exactly one newline
	if !strings.HasSuffix(result, "\n") || strings.HasSuffix(result, "\n\n") {
		t.Errorf("expected result to end with exactly one newline, got %q", result[len(result)-5:])
	}
	// No triple newlines
	if strings.Contains(result, "\n\n\n") {
		t.Error("expected no triple newlines in output")
	}
}

func TestFixCalloutEmoji(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "warning alias replaced",
			input: `<callout emoji="warning" background-color="light-orange">`,
			want:  `<callout emoji="⚠️" background-color="light-orange">`,
		},
		{
			name:  "tip alias replaced",
			input: `<callout emoji="tip">`,
			want:  `<callout emoji="💡">`,
		},
		{
			name:  "actual emoji unchanged",
			input: `<callout emoji="⚠️">`,
			want:  `<callout emoji="⚠️">`,
		},
		{
			name:  "unknown alias unchanged",
			input: `<callout emoji="unicorn">`,
			want:  `<callout emoji="unicorn">`,
		},
		{
			name:  "non-callout tag unchanged",
			input: `<div emoji="warning">`,
			want:  `<div emoji="warning">`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixCalloutEmoji(tt.input)
			if got != tt.want {
				t.Errorf("fixCalloutEmoji(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixCodeBlockTrailingBlanks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "blank before closing fence removed",
			input: "```\ncode\n\n```",
			want:  "```\ncode\n```",
		},
		{
			name:  "no trailing blank unchanged",
			input: "```\ncode\n```",
			want:  "```\ncode\n```",
		},
		{
			// Inside a code block, ```go is just content; only plain ``` closes.
			// This handles create-doc's malformed output where ``` (no lang) closes blocks.
			name:  "language fence inside block is content, plain fence closes",
			input: "```markdown\n```go\nfunc f() {}\n\n```",
			want:  "```markdown\n```go\nfunc f() {}\n```",
		},
		{
			name:  "outside code block blank lines untouched",
			input: "text\n\nmore",
			want:  "text\n\nmore",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixCodeBlockTrailingBlanks(tt.input)
			if got != tt.want {
				t.Errorf("fixCodeBlockTrailingBlanks(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFixTopLevelSoftbreaksQuoteContainer(t *testing.T) {
	input := "<quote-container>\nline1\nline2\n</quote-container>"
	got := fixTopLevelSoftbreaks(input)
	// quote-container is a content container: blank lines inserted between inner lines.
	want := "<quote-container>\n\nline1\n\nline2\n</quote-container>"
	if got != want {
		t.Errorf("fixTopLevelSoftbreaks quote-container = %q, want %q", got, want)
	}
}
