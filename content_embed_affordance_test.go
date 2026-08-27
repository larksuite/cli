// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/meta"
	docshortcuts "github.com/larksuite/cli/shortcuts/doc"
)

// The shortcut owns its concise description, while affordance/docs.md owns
// additive when-to-use guidance, tips, prerequisites, and skill references.
// Keep those roles distinct: copying the description into the lead renders two
// near-identical descriptions, while duplicating Tips in Go and Markdown lets
// the two sources drift because the Markdown overlay wins at help time.
func TestEmbeddedDocsAffordanceComplementsShortcutMetadata(t *testing.T) {
	const contentGuideTip = "Before authoring `--content`, read the matching XML or Markdown guide under Related skills when available, unless already read. For XML, use only documented DocxXML tags."

	type expectation struct {
		description   string
		useWhen       []string
		tips          []string
		prerequisites []string
		skills        []string
	}
	want := map[string]expectation{
		"+create": {
			description: "Create a Lark document",
			useWhen: []string{
				"Create a new Lark document from DocxXML or Markdown, optionally in a folder or Wiki node.",
			},
			tips: []string{
				"Match `--doc-format` to `--content`: XML is the default for rich DocxXML; use `--doc-format markdown` for Markdown input.",
				contentGuideTip,
				"For multiline `--content`, prefer `@file` or `-` (stdin) to avoid shell-escaping damage.",
				"Large XML and Markdown inputs are automatically written as one create followed by serial append batches. The reliable target is 2,000 blocks per batch, the hard operation limit is 5,000, and the document limit is 40,000; one top-level list, table, quote, code fence, or DocxXML container is never split internally. Markdown batching mirrors the DocxXML SDK parser and create-title analysis, including unique/ambiguous top-level ATX H1 behavior.",
				"Before resource conversion or any API call, create rejects a materialized block over 100,000 UTF-16 code units, a table over 2,000 effective cells, or a table over 100 columns; the structured limit error leaves no partial document.",
				"Batched create is fail-fast and is not transactional: successful batches remain when a later append fails. Read `data.create_batches` from the `ok:false` partial result and do not resend completed batches.",
			},
			skills: []string{
				"lark-doc",
				"lark-doc/references/lark-doc-create-workflow.md",
				"lark-doc/references/lark-doc-create.md",
				"lark-doc/references/lark-doc-xml.md",
				"lark-doc/references/lark-doc-md.md",
			},
		},
		"+fetch": {
			description: "Fetch Lark document content",
			useWhen: []string{
				"Read an entire Lark document, or limit the result to an outline, section, block range, or keyword match.",
			},
		},
		"+update": {
			description: "Update a Lark document",
			useWhen: []string{
				"Apply targeted text or block edits, append content, or deliberately replace an entire Lark document.",
			},
			tips: []string{
				"Prefer `str_replace` or `block_*` commands for targeted edits. Use `overwrite` only when replacing the entire document is intended; it can discard unrelated rich content.",
				"Before a `block_*` edit, fetch the target with `lark-cli docs +fetch --detail with-ids` and a narrow `--scope`; refetch after structural changes before reusing block IDs.",
				contentGuideTip,
				"Match `--doc-format` to `--content`; for multiline content, prefer `@file` or `-` (stdin).",
			},
		},
		"+history-list": {
			description: "List Lark document history versions",
		},
		"+history-revert": {
			description: "Revert a Lark document to a historical version",
			prerequisites: []string{
				"`history_version_id` from `+history-list`",
			},
		},
		"+history-revert-status": {
			description: "Get Lark document history revert task status",
			prerequisites: []string{
				"`task_id` from `+history-revert`",
			},
		},
	}

	shortcuts := make(map[string]struct {
		description string
		tips        []string
	})
	for _, shortcut := range docshortcuts.Shortcuts() {
		shortcuts[shortcut.Command] = struct {
			description string
			tips        []string
		}{description: shortcut.Description, tips: shortcut.Tips}
	}

	for command, expected := range want {
		t.Run(command, func(t *testing.T) {
			shortcut, ok := shortcuts[command]
			if !ok {
				t.Fatalf("docs shortcut %s is not registered", command)
			}
			if shortcut.description != expected.description {
				t.Fatalf("shortcut description = %q, want %q", shortcut.description, expected.description)
			}
			if len(shortcut.tips) != 0 {
				t.Fatalf("shortcut Tips = %q; docs affordance owns these tips and must remain the single source", shortcut.tips)
			}

			raw, ok := affordance.For("docs", command)
			if !ok {
				t.Fatalf("embedded affordance entry docs/%s is missing", command)
			}
			parsed, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatalf("embedded affordance entry docs/%s is invalid: %s", command, raw)
			}
			if !slices.Equal(parsed.UseWhen, expected.useWhen) {
				t.Errorf("use_when = %q, want %q", parsed.UseWhen, expected.useWhen)
			}
			if !slices.Equal(parsed.Tips, expected.tips) {
				t.Errorf("tips = %q, want %q", parsed.Tips, expected.tips)
			}
			if !slices.Equal(parsed.Prerequisites, expected.prerequisites) {
				t.Errorf("prerequisites = %q, want %q", parsed.Prerequisites, expected.prerequisites)
			}
			if expected.skills != nil && !slices.Equal(parsed.Skills, expected.skills) {
				t.Errorf("skills = %q, want %q", parsed.Skills, expected.skills)
			}
			for _, lead := range parsed.UseWhen {
				if normalizedCopy(lead) == normalizedCopy(shortcut.description) {
					t.Errorf("use_when repeats the shortcut description instead of adding a decision boundary: %q", lead)
				}
			}
		})
	}
}

func normalizedCopy(value string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(value), "."))
}
