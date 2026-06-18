// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"strings"
	"testing"
)

func TestConvertToIMMarkdownDowngradesRegisteredFragments(t *testing.T) {
	t.Parallel()

	ctx := imMarkdownContext{baseURL: "https://bytedance.larkoffice.com"}
	input := strings.Join([]string{
		`<title>Roadmap</title>`,
		`<grid><column width-ratio="0.5">### Left</column><column width-ratio="0.5">Right</column></grid>`,
		`<table><thead><tr><th>A</th><th>B</th></tr></thead><tbody><tr><td>1</td><td><b>two</b><br/>lines</td></tr></tbody></table>`,
		`<cite type="user" user-id="ou_abc" user-name="Alice"></cite>`,
		`<cite type="doc" doc-id="doc_token" file-type="docx" title="Spec"></cite>`,
		`<cite type="citation"><a href="https://example.com/ref">Ref</a></cite>`,
		`<sheet token="sht_token" sheet-id="S1"></sheet>`,
		`<figure view-type="Preview"><source href="https://example.com/a.md"/></figure>`,
	}, "\n")

	got := convertToIMMarkdown(input, ctx)

	for _, want := range []string{
		"# Roadmap",
		"### Left",
		"Right",
		"| A | B |\n| - | - |\n| 1 | two<br>lines |",
		`<at user_id="ou_abc">Alice</at>`,
		"[Spec](https://bytedance.larkoffice.com/docx/doc_token)",
		"[Ref](https://example.com/ref)",
		"[sheet S1](https://bytedance.larkoffice.com/sheets/sht_token)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("converted content missing %q:\n%s", want, got)
		}
	}
	for _, dropped := range []string{"<grid", "<column", "<table", "<cite", "<sheet", "<figure", "<source"} {
		if strings.Contains(got, dropped) {
			t.Fatalf("converted content still contains %q:\n%s", dropped, got)
		}
	}
}

func TestConvertToIMMarkdownPreservesNestedCalloutContent(t *testing.T) {
	t.Parallel()

	got := convertToIMMarkdown(`<callout emoji="✅"><p>Done</p><callout emoji="💡">Nested</callout></callout>`, imMarkdownContext{})
	for _, want := range []string{
		"**✅ 说明**",
		"Done",
		"**💡 说明**",
		"Nested",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("converted nested callout missing %q:\n%s", want, got)
		}
	}
}

func TestConvertToIMMarkdownFallsBackToInlineCodeForOpaqueResources(t *testing.T) {
	t.Parallel()

	got := convertToIMMarkdown(`<whiteboard token="wb_token"></whiteboard>`, imMarkdownContext{})
	if want := "`<whiteboard token=\"wb_token\"></whiteboard>`"; got != want {
		t.Fatalf("whiteboard fallback = %q, want %q", got, want)
	}
}
