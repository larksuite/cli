// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"fmt"
	"sort"
	"strings"
)

type textConverter struct{}

// textConverter converts a text message raw content into a human-readable line.
func (textConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("text")
	}
	text, _ := parsed["text"].(string)
	if text == "" {
		return ctx.RawContent
	}
	return ResolveMentionKeys(text, ctx.MentionMap)
}

type postConverter struct{}

// postConverter converts a post message raw content (locales, blocks, attachment zone) into human-readable lines.
func (postConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil || parsed == nil {
		return invalidJSONPlaceholder("rich text")
	}
	body := unwrapPostLocale(parsed)

	var parts []string
	if body != nil {
		if title, _ := body["title"].(string); title != "" {
			parts = append(parts, title)
		}
		// Prefer content_v2 blocks; fallback to content blocks
		blocks := selectContentBlocks(body)
		for _, para := range blocks {
			elems, _ := para.([]interface{})
			var line strings.Builder
			for _, el := range elems {
				elem, _ := el.(map[string]interface{})
				line.WriteString(renderPostElem(elem))
			}
			parts = append(parts, line.String())
		}
	}

	result := strings.TrimSpace(strings.Join(parts, "\n"))
	if result == "" {
		result = "[Rich text message]"
	}
	result = ResolveMentionKeys(result, ctx.MentionMap)
	// Attachment zone (top-level "files" array, sibling of the locale keys) is
	// rendered as trailing lines so agents can see file/folder keys for download.
	if attachments := renderPostAttachments(ctx, parsed); attachments != "" {
		result += "\n" + attachments
	}
	return result
}

// renderPostAttachments renders a post message's attachment zone to
// human-readable lines, one per attachment. It uses the same <file>/<folder>
// tag style as standalone file/folder messages, so attachment keys render
// consistently across the output and stay extractable.
//
// The input is the parsed post content JSON as returned by the message API
// (get/mget/list/search/threads, merge_forward sub-items), e.g.:
//
//	"files":[{"file_key":"file_xxx","file_name":"report.pdf"},
//	         {"file_key":"file_yyy","file_name":"assets","is_folder":true}]
//
// renders as:
//
//	<file key="file_xxx" name="report.pdf"/>
//	<folder key="file_yyy" name="assets"/>
func renderPostAttachments(ctx *ConvertContext, parsed map[string]interface{}) string {
	rawFiles, ok := parsed["files"].([]interface{})
	if !ok || len(rawFiles) == 0 {
		return ""
	}
	var lines []string
	for _, raw := range rawFiles {
		f, _ := raw.(map[string]interface{})
		if f == nil {
			continue
		}
		key, _ := f["file_key"].(string)
		if key == "" {
			continue
		}
		name, _ := f["file_name"].(string)
		tag := "file"
		if isFolder, _ := f["is_folder"].(bool); isFolder {
			tag = "folder"
			// Expand folder attachments one level (same as folder messages):
			// 1) prefetch cache hit (message_id + folder key) -> use cached XML;
			// 2) "" cached value = prefetch already tried & failed -> degrade to
			//    the single-line tag, no inline retry (page-wide failures must
			//    not double GETs);
			// 3) cache miss (or no cache) + Runtime + MessageID -> inline fetch;
			// 4) otherwise fall back to the single-line tag below.
			expanded := ""
			if ctx != nil && ctx.MessageID != "" && ctx.FolderChildren != nil {
				if xml, ok := ctx.FolderChildren[ctx.MessageID+folderCacheKeySep+key]; ok {
					if xml != "" {
						expanded = xml
					}
					// "" sentinel: leave expanded empty -> single-line, no retry
				} else if ctx.Runtime != nil {
					expanded = fetchFolderChildrenTree(ctx.Runtime, key, name, ctx.MessageID)
				}
			} else if ctx != nil && ctx.Runtime != nil && ctx.MessageID != "" {
				expanded = fetchFolderChildrenTree(ctx.Runtime, key, name, ctx.MessageID)
			}
			if expanded != "" {
				lines = append(lines, expanded)
				continue
			}
		}
		if name != "" {
			lines = append(lines, fmt.Sprintf(`<%s key="%s" name="%s"/>`, tag, cardEscapeAttr(key), cardEscapeAttr(name)))
		} else {
			lines = append(lines, fmt.Sprintf(`<%s key="%s"/>`, tag, cardEscapeAttr(key)))
		}
	}
	return strings.Join(lines, "\n")
}

// selectContentBlocks returns content_v2 blocks when present and non-empty;
// otherwise falls back to content blocks. This implements the content_v2
// priority rule for post messages.
func selectContentBlocks(body map[string]interface{}) []interface{} {
	if v2, ok := body["content_v2"].([]interface{}); ok && len(v2) > 0 {
		return v2
	}
	blocks, _ := body["content"].([]interface{})
	return blocks
}

func unwrapPostLocale(parsed map[string]interface{}) map[string]interface{} {
	if _, ok := parsed["content"]; ok {
		return parsed
	}
	if _, ok := parsed["title"]; ok {
		return parsed
	}
	for _, locale := range []string{"zh_cn", "en_us", "ja_jp"} {
		if v, ok := parsed[locale]; ok {
			if m, ok := v.(map[string]interface{}); ok {
				return m
			}
		}
	}
	keys := make([]string, 0, len(parsed))
	for key := range parsed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		v := parsed[key]
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// renderPostElem renders a single post (rich-text) element to its inline text
// form: text/a/at carry their content through applyPostStyle for text.style
// Markdown emphasis, emotion becomes :emoji_type:, md is passed through raw,
// and unknown tags fall back to the element's text.
func renderPostElem(el map[string]interface{}) string {
	tag, _ := el["tag"].(string)
	switch tag {
	case "text":
		text, _ := el["text"].(string)
		return applyPostStyle(text, el["style"])
	case "a":
		text, _ := el["text"].(string)
		href, _ := el["href"].(string)
		var rendered string
		switch {
		case href != "" && text != "":
			rendered = fmt.Sprintf("[%s](%s)", escapeMDLinkText(text), href)
		case href != "":
			rendered = href
		default:
			rendered = text
		}
		return applyPostStyle(rendered, el["style"])
	case "at":
		userId, _ := el["user_id"].(string)
		var rendered string
		switch {
		case userId == "@_all" || userId == "all":
			rendered = `<at user_id="all"></at>`
		default:
			if name, _ := el["user_name"].(string); name != "" {
				if userId != "" && strings.HasPrefix(userId, "ou") {
					rendered = fmt.Sprintf(`<at user_id="%s">%s</at>`, userId, name)
				} else {
					rendered = "@" + name
				}
			} else {
				rendered = "@" + userId
			}
		}
		return applyPostStyle(rendered, el["style"])
	case "emotion":
		// Deliberately not routed through applyPostStyle: an emoji shortcode is
		// an atomic token, not prose, so bold/italic/strike emphasis around
		// ":emoji:" would be meaningless (and emotion elements don't carry style).
		emoji, _ := el["emoji_type"].(string)
		if emoji == "" {
			return ""
		}
		return ":" + emoji + ":"
	case "md":
		text, _ := el["text"].(string)
		return text
	case "img":
		key, _ := el["image_key"].(string)
		if key != "" {
			return fmt.Sprintf("![Image](%s)", key)
		}
		return "[Image]"
	case "media":
		key, _ := el["file_key"].(string)
		if key != "" {
			return fmt.Sprintf("[Media: %s]", key)
		}
		return "[Media]"
	case "code_block":
		lang, _ := el["language"].(string)
		code, _ := el["text"].(string)
		if lang != "" {
			return fmt.Sprintf("\n```%s\n%s\n```\n", lang, code)
		}
		return fmt.Sprintf("\n```\n%s\n```\n", code)
	case "hr":
		return "\n---\n"
	default:
		text, _ := el["text"].(string)
		return text
	}
}

// applyPostStyle wraps text with Markdown emphasis per the post element's
// style array (bold/italic/underline/lineThrough). Styles compose from inner
// to outer in a fixed order so output is deterministic; empty text or no
// styles pass through unchanged.
func applyPostStyle(text string, raw interface{}) string {
	if text == "" {
		return text
	}
	styles, _ := raw.([]interface{})
	if len(styles) == 0 {
		return text
	}
	has := func(name string) bool {
		for _, s := range styles {
			if v, _ := s.(string); v == name {
				return true
			}
		}
		return false
	}
	if has("bold") {
		text = "**" + text + "**"
	}
	if has("italic") {
		text = "*" + text + "*"
	}
	if has("underline") {
		text = "<u>" + text + "</u>"
	}
	if has("lineThrough") {
		text = "~~" + text + "~~"
	}
	return text
}
