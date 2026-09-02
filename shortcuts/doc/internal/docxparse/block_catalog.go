// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package docxparse

type tagLayout string

type tagDefinition struct {
	layout       tagLayout
	presentation bool
}

const (
	layoutBlock      tagLayout = "block"
	layoutInline     tagLayout = "inline"
	layoutDual       tagLayout = "dual"
	layoutStructural tagLayout = "structural"
	layoutCommand    tagLayout = "command"
)

// blockCatalog is a profiling catalog, not an XML schema. Unknown elements
// remain valid XML containers and are intentionally absent from block counts.
var blockCatalog = map[string]tagDefinition{}

func init() {
	registerTags(layoutBlock,
		"title", "h1", "h2", "h3", "h4", "h5", "h6", "h7", "h8", "h9", "p",
		"div", "ul", "ol", "li", "blockquote", "column", "thead", "tbody", "tfoot",
		"tr", "hr", "source", "base_refer", "synced_reference", "isv", "view",
		"synced-source", "readonly-block", "checkbox", "okr-objective", "okr-key-result",
		"okr-progress", "task", "append",
	)
	registerPresentationTags(layoutBlock,
		"grid", "table", "pre", "img", "bitable", "sheet", "mindnote", "whiteboard",
		"html5-block", "figure", "callout", "chat_card", "okr", "poll", "agenda",
		"folder-manager", "sub-page-list", "wiki_catalog", "wiki_recent_update",
		"chart-embedded", "chart-refer-host-perm", "chart_embedded", "chart_refer_host_perm",
		"bookmark", "vc-tabs", "vc-summary-tab", "vc-transcribe-tab",
	)
	registerTags(layoutInline, "b", "em", "u", "del", "i", "span", "br", "inline-file", "mention-date", "cite", "button", "time", "a")
	registerTags(layoutDual, "latex", "code")
	registerTags(layoutStructural, "th", "td", "colgroup", "col", "sub-page")
	registerTags(layoutCommand,
		"comment", "block_delete", "str_delete", "str_replace", "block_replace", "block_insert",
		"block_move", "block_copy_insert_after", "src_block_ids", "create", "answer", "response",
		"identifier", "genre", "anchor", "type", "revision", "pattern", "replacement",
		"replace_content", "action", "content", "parameter", "generation", "block_id",
	)
}

func registerTags(layout tagLayout, tags ...string) {
	for _, tag := range tags {
		blockCatalog[tag] = tagDefinition{layout: layout}
	}
}

func registerPresentationTags(layout tagLayout, tags ...string) {
	for _, tag := range tags {
		blockCatalog[tag] = tagDefinition{layout: layout, presentation: true}
	}
}

func layoutOf(tag string) tagLayout {
	return blockCatalog[tag].layout
}

func isKnownTag(tag string) bool { _, ok := blockCatalog[tag]; return ok }

// IsPresentationBlockType reports whether tag is both counted by the profile
// and suitable for a Presentation Decision block plan. The centralized
// catalog keeps planning policy independent from individual component fields.
func IsPresentationBlockType(tag string) bool {
	definition, ok := blockCatalog[tag]
	profiled := definition.layout == layoutBlock || definition.layout == layoutDual
	return ok && profiled && definition.presentation
}

var voidTags = map[string]bool{
	"br":       true,
	"col":      true,
	"hr":       true,
	"img":      true,
	"source":   true,
	"sub-page": true,
}

func isVoidTag(tag string) bool { return voidTags[tag] }

var preserveSpaceTags = map[string]bool{
	"title": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "h7": true, "h8": true, "h9": true,
	"p": true, "i": true, "b": true, "em": true, "u": true, "del": true,
	"code": true, "li": true, "a": true, "span": true,
}

var strictPhrasingTags = map[string]bool{
	"title": true, "span": true, "b": true, "em": true, "i": true,
	"u": true, "del": true, "a": true,
}

var autoCloseTags = map[string]map[string]bool{
	"li":     {"li": true},
	"tr":     {"tr": true},
	"td":     {"td": true, "th": true, "tr": true, "tbody": true, "tfoot": true},
	"th":     {"th": true, "td": true, "tr": true, "tbody": true, "tfoot": true},
	"tbody":  {"tbody": true, "tfoot": true},
	"thead":  {"tbody": true, "tfoot": true},
	"column": {"column": true},
}

func shouldAutoClose(openTag, nextTag string) bool {
	if strictPhrasingTags[openTag] && layoutOf(nextTag) == layoutBlock {
		return true
	}
	return autoCloseTags[openTag] != nil && autoCloseTags[openTag][nextTag]
}
