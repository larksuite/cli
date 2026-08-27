// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

import (
	"sort"
	"strings"
	"testing"
)

func TestCreateBatchPlannerCoversEveryRegisteredContentTag(t *testing.T) {
	tags := make([]string, 0, len(blockCatalog))
	for tag, definition := range blockCatalog {
		if definition.layout != layoutCommand {
			tags = append(tags, tag)
		}
	}
	sort.Strings(tags)

	limits := CreateBatchLimits{TargetBlocks: 10_000, OperationBlocks: 10_000, TotalBlocks: 20_000, Content: DefaultContentLimits()}
	for _, tag := range tags {
		source := representativeCatalogTagXML(tag)
		t.Run("xml/"+tag, func(t *testing.T) {
			nodes, err := parseXML(source)
			if err != nil {
				t.Fatalf("parseXML(%q): %v", source, err)
			}
			if len(nodes) != 1 || nodes[0].typ != nodeElement {
				t.Fatalf("parseXML(%q) nodes = %#v", source, nodes)
			}
			assertCatalogTagMaterialization(t, tag, nodes[0])

			plan, err := PlanCreateBatchesWithLimits(source, limits)
			if err != nil {
				t.Fatalf("PlanCreateBatchesWithLimits(%q): %v", source, err)
			}
			if got := strings.Join(plan.Batches, ""); got != source {
				t.Fatalf("XML batches changed source: got %q, want %q", got, source)
			}
		})

		t.Run("markdown/"+tag, func(t *testing.T) {
			markdown := source + "\n"
			plan, err := PlanCreateMarkdownBatchesWithLimits(markdown, limits)
			if err != nil {
				t.Fatalf("PlanCreateMarkdownBatchesWithLimits(%q): %v", markdown, err)
			}
			if got := strings.Join(plan.Batches, ""); got != markdown {
				t.Fatalf("Markdown batches changed source: got %q, want %q", got, markdown)
			}
		})
	}
}

func assertCatalogTagMaterialization(t *testing.T, tag string, node *Node) {
	t.Helper()
	count := materializedBlockCount(node)
	switch layoutOf(tag) {
	case layoutBlock, layoutDual:
		if count <= 0 {
			t.Fatalf("materializedBlockCount(<%s>) = %d, want positive", tag, count)
		}
	case layoutInline:
		if count != 0 {
			t.Fatalf("materializedBlockCount(<%s>) = %d, want 0 for inline tag", tag, count)
		}
	case layoutStructural:
		want := 0
		if tag == "td" || tag == "th" {
			want = 1
		}
		if count != want {
			t.Fatalf("materializedBlockCount(<%s>) = %d, want %d", tag, count, want)
		}
	}
}

func representativeCatalogTagXML(tag string) string {
	switch tag {
	case "ul", "ol":
		return "<" + tag + "><li>x</li></" + tag + ">"
	case "div", "append":
		return "<" + tag + "><p>x</p></" + tag + ">"
	case "thead", "tbody", "tfoot":
		return "<" + tag + "><tr><td>x</td></tr></" + tag + ">"
	case "tr":
		return "<tr><td>x</td></tr>"
	case "table":
		return "<table><tbody><tr><td><p>x</p></td></tr></tbody></table>"
	case "grid":
		return "<grid><column><p>x</p></column></grid>"
	case "pre":
		return "<pre><code>x</code></pre>"
	case "figure":
		return `<figure><img src="image-token"/></figure>`
	}
	if isVoidTag(tag) {
		return "<" + tag + "/>"
	}
	return "<" + tag + ">x</" + tag + ">"
}
