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
			name:  "leading space after opening bold",
			input: "** hello**",
			want:  "**hello**",
		},
		{
			name:  "leading space after opening italic",
			input: "* hello*",
			want:  "*hello*",
		},
		{
			name:  "leading and trailing spaces inside bold are collapsed",
			input: "** hello **",
			want:  "**hello**",
		},
		{
			name:  "leading and trailing spaces inside italic are collapsed",
			input: "* hello *",
			want:  "*hello*",
		},
		{
			name:  "multiple spaced italic spans on one line are each collapsed",
			input: "* a* * b*",
			want:  "*a* *b*",
		},
		{
			name:  "ambiguous italic span stays literal",
			input: "2 * x * y",
			want:  "2 * x * y",
		},
		{
			name:  "ambiguous bold span stays literal",
			input: "2 ** x ** y",
			want:  "2 ** x ** y",
		},
		{
			name:  "single-rune italic with spaces on both sides stays literal",
			input: "* x *",
			want:  "* x *",
		},
		{
			name:  "single-rune bold with spaces on both sides stays literal",
			input: "** x **",
			want:  "** x **",
		},
		{
			name:  "triple-asterisk near miss stays literal",
			input: "*** hello**",
			want:  "*** hello**",
		},
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
		{
			name:  "inline code span not modified",
			input: "`**hello **`",
			want:  "`**hello **`",
		},
		{
			name:  "inline code preserved, bold outside fixed",
			input: "**foo ** and `**bar **`",
			want:  "**foo** and `**bar **`",
		},
		{
			name:  "inline code with spaced italic stays literal while outside span is fixed",
			input: "`* hello *` and * hello *",
			want:  "`* hello *` and *hello*",
		},
		{
			name:  "opening space inside text tag fixed",
			input: `<text color="red">** Helpful - 有用性：**</text>`,
			want:  `<text color="red">**Helpful - 有用性：**</text>`,
		},
		{
			name:  "double-backtick inline code not modified",
			input: "``**hello **`` and **world **",
			want:  "``**hello **`` and **world**",
		},
		{
			name:  "double-backtick span containing literal backtick not modified",
			input: "`` a`b `` and **bold **",
			want:  "`` a`b `` and **bold**",
		},
		{
			name:  "heading with multiple bold spans left unchanged",
			input: "# **foo** and **bar**",
			want:  "# **foo** and **bar**",
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
		{
			name:  "consecutive unordered list items not split",
			input: "- item a\n- item b\n- item c",
			want:  "- item a\n- item b\n- item c",
		},
		{
			name:  "consecutive ordered list items not split",
			input: "1. first\n2. second\n3. third",
			want:  "1. first\n2. second\n3. third",
		},
		{
			name:  "list continuation not split from item",
			input: "- item a\n  continuation",
			want:  "- item a\n  continuation",
		},
		{
			name:  "text to list transition gets blank line",
			input: "paragraph\n- list item",
			want:  "paragraph\n\n- list item",
		},
		{
			name:  "adjacent callout blocks get blank line between them",
			input: "<callout>\ncontent1\n</callout>\n<callout>\ncontent2\n</callout>",
			want:  "<callout>\n\ncontent1\n</callout>\n\n<callout>\n\ncontent2\n</callout>",
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

func TestNormalizeNestedListIndentation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "nested ordered list uses tabs instead of space pairs",
			input: "1. parent\n  1. child\n    1. grandchild",
			want:  "1. parent\n\t1. child\n\t\t1. grandchild",
		},
		{
			name:  "nested mixed list markers use tabs instead of space pairs",
			input: "- parent\n  - child\n    1. grandchild",
			want:  "- parent\n\t- child\n\t\t1. grandchild",
		},
		{
			name:  "top-level list unchanged",
			input: "1. parent\n2. sibling",
			want:  "1. parent\n2. sibling",
		},
		{
			name:  "indented top-level marker without parent list stays unchanged",
			input: "paragraph\n\n  1. item",
			want:  "paragraph\n\n  1. item",
		},
		{
			name:  "blank-line-separated loose-list sibling stays unchanged",
			input: "1. a\n\n  1. b",
			want:  "1. a\n\n  1. b",
		},
		{
			name:  "indented code block inside list item stays unchanged",
			input: "- parent\n\n    1. code",
			want:  "- parent\n\n    1. code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNestedListIndentation(tt.input)
			if got != tt.want {
				t.Errorf("normalizeNestedListIndentation(%q) = %q, want %q", tt.input, got, tt.want)
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

func TestWarnCalloutType(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantHint     bool   // whether a hint line is expected
		hintContains string // substring the hint must contain
	}{
		{
			name:         "warning type without background-color emits hint",
			input:        `<callout type="warning" emoji="📝">`,
			wantHint:     true,
			hintContains: `background-color="light-yellow"`,
		},
		{
			name:         "info type without background-color emits hint",
			input:        `<callout type="info" emoji="ℹ️">`,
			wantHint:     true,
			hintContains: `background-color="light-blue"`,
		},
		{
			name:         "single-quoted type attribute emits hint",
			input:        `<callout type='warning' emoji="📝">`,
			wantHint:     true,
			hintContains: `background-color="light-yellow"`,
		},
		{
			name:     "explicit background-color suppresses hint",
			input:    `<callout type="warning" emoji="📝" background-color="light-red">`,
			wantHint: false,
		},
		{
			name:     "whitespace around equals is tolerated in background-color",
			input:    `<callout type="warning" emoji="📝" background-color = "light-red">`,
			wantHint: false,
		},
		{
			name:     "unknown type emits no hint",
			input:    `<callout type="custom" emoji="🔥">`,
			wantHint: false,
		},
		{
			name:     "no type attribute emits no hint",
			input:    `<callout emoji="💡" background-color="light-green">`,
			wantHint: false,
		},
		{
			name:     "non-callout tag emits no hint",
			input:    `<div type="warning">`,
			wantHint: false,
		},
		{
			name:         "hint includes border-color suggestion",
			input:        `<callout type="error" emoji="❌">`,
			wantHint:     true,
			hintContains: `border-color="red"`,
		},
		{
			// Regression: the old `\btype=` regex matched the suffix of
			// `data-type=` because `-` is a non-word character, so a tag
			// carrying only data-attrs would silently get a bogus hint.
			// The (?:^|\s) anchor requires a real attribute separator.
			name:     "data-type attribute does not trigger hint",
			input:    `<callout data-type="warning" emoji="📝">`,
			wantHint: false,
		},
		{
			// Symmetric guard for the background-color regex: a future
			// `data-background-color=` attribute must not be mistaken
			// for a present background-color and silently suppress the
			// hint that the real type= would otherwise produce.
			name:         "data-background-color does not suppress hint",
			input:        `<callout type="warning" data-background-color="anything">`,
			wantHint:     true,
			hintContains: `background-color="light-yellow"`,
		},
		{
			// Regression for the code-fence skip: a documentation sample
			// inside a ``` fence is NOT a real callout the user wants
			// rendered, so it must produce no stderr noise.
			name: "callout inside backtick fence emits no hint",
			input: "```markdown\n" +
				`<callout type="warning" emoji="📝">` + "\n" +
				"```\n",
			wantHint: false,
		},
		{
			// Same skip works for tilde fences (CommonMark §4.5 makes
			// `~~~` an equivalent fence character).
			name: "callout inside tilde fence emits no hint",
			input: "~~~markdown\n" +
				`<callout type="info" emoji="ℹ️">` + "\n" +
				"~~~\n",
			wantHint: false,
		},
		{
			// Closing the fence must restore normal scanning: a real
			// callout that follows a documentation block still gets a
			// hint. Pins that fenceMarker is reset, not stuck.
			name: "callout after fence close still emits hint",
			input: "```markdown\n" +
				`<callout type="warning">sample</callout>` + "\n" +
				"```\n" +
				`<callout type="error" emoji="❌">real</callout>` + "\n",
			wantHint:     true,
			hintContains: `border-color="red"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			WarnCalloutType(tt.input, &buf)
			got := buf.String()
			if tt.wantHint {
				if got == "" {
					t.Errorf("WarnCalloutType(%q): expected hint, got no output", tt.input)
					return
				}
				if tt.hintContains != "" && !strings.Contains(got, tt.hintContains) {
					t.Errorf("WarnCalloutType(%q): hint %q missing %q", tt.input, got, tt.hintContains)
				}
			} else {
				if got != "" {
					t.Errorf("WarnCalloutType(%q): expected no output, got %q", tt.input, got)
				}
			}
		})
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

func TestApplyOutsideCodeFences(t *testing.T) {
	// Transforms should not modify content inside fenced code blocks.
	input := "```md\n**x **\n> a\n> b\nline\n---\n```"

	if got := applyOutsideCodeFences(input, fixBoldSpacing); got != input {
		t.Fatalf("fixBoldSpacing (via applyOutsideCodeFences) modified fenced code:\ngot  %q\nwant %q", got, input)
	}
	if got := applyOutsideCodeFences(input, fixSetextAmbiguity); got != input {
		t.Fatalf("fixSetextAmbiguity (via applyOutsideCodeFences) modified fenced code:\ngot  %q\nwant %q", got, input)
	}
	if got := applyOutsideCodeFences(input, fixBlockquoteHardBreaks); got != input {
		t.Fatalf("fixBlockquoteHardBreaks (via applyOutsideCodeFences) modified fenced code:\ngot  %q\nwant %q", got, input)
	}

	// Content outside the fence should still be transformed.
	mixed := "**foo ** before\n```\n**x **\n```\n**bar ** after"
	got := applyOutsideCodeFences(mixed, fixBoldSpacing)
	if strings.Contains(got, "**foo **") {
		t.Errorf("fixBoldSpacing did not fix bold before fence: %q", got)
	}
	if strings.Contains(got, "**bar **") {
		t.Errorf("fixBoldSpacing did not fix bold after fence: %q", got)
	}
	if !strings.Contains(got, "```\n**x **\n```") {
		t.Errorf("fixBoldSpacing modified content inside fence: %q", got)
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
