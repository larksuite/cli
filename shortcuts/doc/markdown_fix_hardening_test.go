// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

// TestFixExportedMarkdownIdempotent asserts the core promise of the exported
// markdown pipeline: applying the fixes twice produces the same result as
// applying them once. Round-trip formatting relies on this invariant, so any
// transform that keeps rewriting its own output would break fetch → edit →
// update → fetch stability.
func TestFixExportedMarkdownIdempotent(t *testing.T) {
	fixtures := map[string]string{
		"kitchen sink": strings.Join([]string{
			"# **Title**",
			"paragraph one",
			"paragraph two",
			"**bold ** and * italic*",
			"",
			"> q1",
			"> q2",
			"",
			"1. parent",
			"  1. child",
			"    1. grandchild",
			"",
			"<callout emoji=\"warning\">",
			"callout body line 1",
			"callout body line 2",
			"</callout>",
			"",
			"some text",
			"---",
			"",
			"```go",
			"// code content with markdown-like shapes must survive as-is",
			"**foo **",
			"* hello*",
			"  1. nested",
			"> q",
			"---",
			"```",
			"",
		}, "\n"),

		"cjk content": strings.Join([]string{
			"# **测试标题**",
			"段落一",
			"段落二",
			"**有用性 ** and * 关键 *",
			"",
			"1. 父项",
			"  1. 子项",
			"",
		}, "\n"),

		"nested containers": strings.Join([]string{
			"<callout emoji=\"info\">",
			"line a",
			"line b",
			"</callout>",
			"",
			"<quote-container>",
			"quoted 1",
			"quoted 2",
			"</quote-container>",
			"",
		}, "\n"),
	}

	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			once := fixExportedMarkdown(fixture)
			twice := fixExportedMarkdown(once)
			if once != twice {
				t.Errorf("fixExportedMarkdown is not idempotent for %q\nfirst pass:\n%s\nsecond pass:\n%s",
					name, once, twice)
			}
		})
	}
}

// TestFixExportedMarkdownPreservesFencedCodeByteForByte packs a fenced code
// block with content that every individual transform in the pipeline would
// normally rewrite, and asserts the fence content comes out byte-for-byte
// identical. This is the pipeline's strongest invariant — users' code samples
// must never be silently modified by a formatting pass.
func TestFixExportedMarkdownPreservesFencedCodeByteForByte(t *testing.T) {
	// Every line below is something at least one transform would touch if it
	// appeared outside a fence. None of it must change.
	dangerous := strings.Join([]string{
		"**foo **",      // fixBoldSpacing — trailing space bold
		"* hello*",      // fixBoldSpacing — leading space italic
		"# **heading**", // fixBoldSpacing — redundant heading bold
		"para1",         // fixTopLevelSoftbreaks — adjacent paragraphs
		"para2",
		"> q1", // fixBlockquoteHardBreaks — blockquote pair
		"> q2",
		"some text", // fixSetextAmbiguity — text before ---
		"---",
		"  1. nested",               // normalizeNestedListIndentation
		`<callout emoji="warning">`, // fixCalloutEmoji — emoji alias
	}, "\n")

	// Wrap the dangerous content in a triple-backtick fence and surround with
	// content so the pipeline has adjacent regions to potentially touch.
	input := "before\n\n```\n" + dangerous + "\n```\n\nafter\n"

	got := fixExportedMarkdown(input)

	// Extract the fence content from the output and compare to the input fence
	// content byte-for-byte.
	gotFence, ok := extractFirstFenceContent(got)
	if !ok {
		t.Fatalf("fixExportedMarkdown output lost its fenced code block:\n%s", got)
	}
	if gotFence != dangerous {
		t.Errorf("fenced code content was modified\nwant (bytes): %q\ngot  (bytes): %q",
			dangerous, gotFence)
	}
}

// extractFirstFenceContent returns the inner text of the first triple-backtick
// fenced code block it finds, or ("", false) if none is present.
func extractFirstFenceContent(md string) (string, bool) {
	const fence = "```"
	open := strings.Index(md, fence)
	if open < 0 {
		return "", false
	}
	// Skip the fence marker and its info-string line.
	rest := md[open+len(fence):]
	lineEnd := strings.Index(rest, "\n")
	if lineEnd < 0 {
		return "", false
	}
	rest = rest[lineEnd+1:]
	close := strings.Index(rest, "\n"+fence)
	if close < 0 {
		return "", false
	}
	return rest[:close], true
}

// TestFixExportedMarkdownPreservesCRLF feeds CRLF-terminated markdown (Windows
// line endings) through the pipeline and asserts that line endings are
// preserved AND the emphasis/heading transforms still apply — neither
// silently-LF-normalized nor passed through unchanged.
func TestFixExportedMarkdownPreservesCRLF(t *testing.T) {
	lf := "# **Title**\nparagraph one\nparagraph two\n**bold **\n"
	crlf := strings.ReplaceAll(lf, "\n", "\r\n")

	got := fixExportedMarkdown(crlf)

	// Transforms must still fire: heading bold stripped, trailing-space bold trimmed.
	if strings.Contains(got, "**Title**") {
		t.Errorf("heading bold not stripped on CRLF input:\n%q", got)
	}
	if strings.Contains(got, "**bold **") {
		t.Errorf("trailing-space bold not fixed on CRLF input:\n%q", got)
	}
	// CRLF line endings must survive — we don't want to silently normalize a
	// Windows author's document to LF.
	if !strings.Contains(got, "\r\n") {
		t.Errorf("CRLF line endings were normalized away:\n%q", got)
	}
}

// TestFixExportedMarkdownTransformInteractions covers shapes where more than
// one transform fires on the same input. Each transform is individually tested
// elsewhere; these cases guard against composition regressions.
func TestFixExportedMarkdownTransformInteractions(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantContains []string // substrings that must be present after fixes
		wantAbsent   []string // substrings that must be absent after fixes
	}{
		{
			name:  "nested list item with trailing-space bold",
			input: "1. parent\n  1. **child **\n",
			wantContains: []string{
				"\t1.",      // nested indent converted to tab
				"**child**", // trailing space trimmed
			},
			wantAbsent: []string{
				"  1.",       // original two-space indent gone
				"**child **", // original trailing space gone
			},
		},
		{
			name:  "paragraph followed by list",
			input: "paragraph\n- item a\n- item b\n",
			wantContains: []string{
				"paragraph\n\n- item a", // blank line inserted at text-to-list transition
			},
			wantAbsent: []string{
				"\n\n\n", // no triple newline
			},
		},
		{
			name:  "callout containing list with emphasis",
			input: "<callout emoji=\"info\">\n- **item **\n- another\n</callout>\n",
			wantContains: []string{
				"**item**", // trailing-space bold fixed inside callout
			},
			wantAbsent: []string{
				"**item **",
			},
		},
		{
			name:  "heading followed by paragraph with bold",
			input: "# **Title**\nbody **text **\n",
			wantContains: []string{
				"# Title",       // heading bold stripped
				"body **text**", // paragraph bold trimmed, not stripped
			},
			wantAbsent: []string{
				"# **Title**",
				"body **text **",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fixExportedMarkdown(tt.input)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("want substring %q not found in output:\n%s", want, got)
				}
			}
			for _, unwanted := range tt.wantAbsent {
				if strings.Contains(got, unwanted) {
					t.Errorf("unwanted substring %q still present in output:\n%s", unwanted, got)
				}
			}
		})
	}
}

// TestNormalizeNestedListIndentationDocumentedSkips locks in the deliberate
// "do nothing" branches of normalizeNestedListIndentation. Each case below is
// a shape the function intentionally does not rewrite; if a future change to
// the heuristic flips one of these, we want the regression to be visible in
// the test diff rather than silently changing user documents.
func TestNormalizeNestedListIndentationDocumentedSkips(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// want is identical to input — we are asserting "no change".
	}{
		{
			name:  "three-space indent (odd) under list item stays unchanged",
			input: "1. parent\n   1. child",
		},
		{
			name:  "five-space indent (odd) under list item stays unchanged",
			input: "- parent\n     - deep",
		},
		{
			name:  "two-space indent without a parent list item stays unchanged",
			input: "plain paragraph\n  - not nested",
		},
		{
			name:  "blank-line-separated loose-list sibling stays unchanged",
			input: "1. a\n\n  1. b",
		},
		{
			name:  "four-space indented code block under list item stays unchanged",
			input: "- parent\n\n    1. code sample",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeNestedListIndentation(tt.input)
			if got != tt.input {
				t.Errorf("normalizeNestedListIndentation unexpectedly rewrote documented-skip input\ninput: %q\ngot:   %q", tt.input, got)
			}
		})
	}
}
