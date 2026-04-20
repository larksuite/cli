// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestCheckDocsUpdateReplaceMultilineMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     string
		markdown string
		wantHint bool
	}{
		{
			name:     "replace_range with blank line emits hint",
			mode:     "replace_range",
			markdown: "new paragraph\n\nsecond paragraph",
			wantHint: true,
		},
		{
			name:     "replace_all with blank line emits hint",
			mode:     "replace_all",
			markdown: "first\n\nsecond",
			wantHint: true,
		},
		{
			name:     "replace_range single paragraph is fine",
			mode:     "replace_range",
			markdown: "just a single paragraph of text",
			wantHint: false,
		},
		{
			name:     "single newline is not a paragraph break",
			mode:     "replace_range",
			markdown: "line one\nline two",
			wantHint: false,
		},
		{
			name:     "crlf paragraph break is also detected",
			mode:     "replace_range",
			markdown: "first\r\n\r\nsecond",
			wantHint: true,
		},
		{
			name:     "other modes are not flagged",
			mode:     "insert_before",
			markdown: "first\n\nsecond",
			wantHint: false,
		},
		{
			name:     "append mode is not flagged",
			mode:     "append",
			markdown: "first\n\nsecond",
			wantHint: false,
		},
		{
			name:     "empty markdown is fine",
			mode:     "replace_range",
			markdown: "",
			wantHint: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkDocsUpdateReplaceMultilineMarkdown(tt.mode, tt.markdown)
			hasHint := got != ""
			if hasHint != tt.wantHint {
				t.Fatalf("checkDocsUpdateReplaceMultilineMarkdown(%q, %q) = %q, wantHint=%v",
					tt.mode, tt.markdown, got, tt.wantHint)
			}
			if tt.wantHint && !strings.Contains(got, "delete_range") {
				t.Errorf("hint should suggest delete_range/insert_before remediation, got: %s", got)
			}
		})
	}
}

func TestCheckDocsUpdateBoldItalic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		wantHint bool
	}{
		{
			name:     "triple asterisks flagged",
			input:    "a ***key insight*** here",
			wantHint: true,
		},
		{
			name:     "triple asterisks single char flagged",
			input:    "a ***X*** here",
			wantHint: true,
		},
		{
			name:     "bold wrapping underscore italic flagged",
			input:    "note: **_important_** detail",
			wantHint: true,
		},
		{
			name:     "underscore wrapping double asterisk flagged",
			input:    "note: _**important**_ detail",
			wantHint: true,
		},
		{
			name:     "plain bold is fine",
			input:    "this is **bold** text",
			wantHint: false,
		},
		{
			name:     "plain italic is fine",
			input:    "this is *italic* or _italic_ text",
			wantHint: false,
		},
		{
			name:     "horizontal rule is not flagged",
			input:    "paragraph\n\n---\n\nnext",
			wantHint: false,
		},
		{
			name:     "bold followed by italic with space is not flagged",
			input:    "**bold** and *italic*",
			wantHint: false,
		},
		{
			name:     "empty input is fine",
			input:    "",
			wantHint: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := checkDocsUpdateBoldItalic(tt.input)
			hasHint := got != ""
			if hasHint != tt.wantHint {
				t.Fatalf("checkDocsUpdateBoldItalic(%q) = %q, wantHint=%v", tt.input, got, tt.wantHint)
			}
		})
	}
}

func TestDocsUpdateWarningsAggregates(t *testing.T) {
	t.Parallel()

	// Both flags trigger: replace_range with blank line AND triple-asterisk.
	warnings := docsUpdateWarnings("replace_range", "***opening***\n\nsecond paragraph")
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestDocsUpdateWarningsEmpty(t *testing.T) {
	t.Parallel()

	// Clean markdown in a non-replace mode produces zero warnings.
	warnings := docsUpdateWarnings("insert_before", "plain paragraph text")
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}
