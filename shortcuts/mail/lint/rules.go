// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lint

import "strings"

// Rule IDs surfaced through Finding.RuleID. UPPER_SNAKE_CASE naming is the
// contract for the stdout envelope. New rules MUST keep this naming convention
// so AI / test consumers can pattern-match reliably.
const (
	// Tag-level rules.
	RuleTagFontToSpan      = "TAG_FONT_TO_SPAN"
	RuleTagCenterToDiv     = "TAG_CENTER_TO_DIV"
	RuleTagMarqueeToText   = "TAG_MARQUEE_TO_TEXT"
	RuleTagBlinkToText     = "TAG_BLINK_TO_TEXT"
	RuleTagScriptBlocked   = "TAG_SCRIPT_BLOCKED"
	RuleTagIframeBlocked   = "TAG_IFRAME_BLOCKED"
	RuleTagObjectBlocked   = "TAG_OBJECT_BLOCKED"
	RuleTagEmbedBlocked    = "TAG_EMBED_BLOCKED"
	RuleTagFormBlocked     = "TAG_FORM_BLOCKED"
	RuleTagInputBlocked    = "TAG_INPUT_BLOCKED"
	RuleTagLinkBlocked     = "TAG_LINK_BLOCKED"
	RuleTagMetaBlocked     = "TAG_META_BLOCKED"
	RuleTagBaseBlocked     = "TAG_BASE_BLOCKED"
	RuleTagUnknownStripped = "TAG_UNKNOWN_STRIPPED"

	// Attribute-level rules.
	RuleAttrEventHandlerBlocked = "ATTR_EVENT_HANDLER_BLOCKED"
	RuleAttrJSURLBlocked        = "ATTR_JS_URL_BLOCKED"
	RuleAttrUnsafeSchemeBlocked = "ATTR_UNSAFE_SCHEME_BLOCKED"

	// Style-level rules.
	RuleStylePropertyDropped = "STYLE_PROPERTY_DROPPED"

	// Feishu-native autofix rules. These autofix the inline style /
	// class / nesting shape of common elements so AI-authored HTML
	// matches what Feishu mail-editor itself emits, fixing the visual
	// "extra blank line between blocks", "list bullets/numbers missing",
	// "link color wrong" etc. classes of issues. The rewrite is purely
	// additive — user-supplied inline styles take precedence; the lib
	// only fills the missing properties.
	RuleStyleListNative       = "STYLE_LIST_NATIVE_INLINE_APPLIED"
	RuleStyleListItemNative   = "STYLE_LIST_ITEM_NATIVE_INLINE_APPLIED"
	RuleStyleBlockquoteNative = "STYLE_BLOCKQUOTE_NATIVE_INLINE_APPLIED"
	RuleStyleLinkNative       = "STYLE_LINK_NATIVE_INLINE_APPLIED"
	RuleStyleParaWrapper      = "STYLE_PARA_WRAPPER_REWRITTEN"

	// RuleListDirectChildNonLI fires when a <ul> or <ol> has a non-<li>
	// element child (e.g. nested <ul><ul>). HTML spec requires list children
	// to be <li>; browsers silently hoist the nested list out and the visual
	// nesting falls apart. The lib autofixes by wrapping the offending child
	// in a synthetic <li>.
	RuleListDirectChildNonLI = "LIST_DIRECT_CHILD_NON_LI"
)

// Tag classification ----------------------------------------------------------

// allowedTags enumerates tags that pass through verbatim (tag classification row "通过").
// Lower-case canonical names; the parser normalises tag names so we don't need
// case-insensitive comparison at lookup time.
var allowedTags = map[string]bool{
	"p":          true,
	"div":        true,
	"span":       true,
	"br":         true,
	"hr":         true,
	"a":          true,
	"img":        true,
	"table":      true,
	"thead":      true,
	"tbody":      true,
	"tfoot":      true,
	"tr":         true,
	"td":         true,
	"th":         true,
	"ul":         true,
	"ol":         true,
	"li":         true,
	"blockquote": true,
	"pre":        true,
	"code":       true,
	"b":          true,
	"i":          true,
	"em":         true,
	"strong":     true,
	"u":          true,
	"s":          true,
	"strike":     true,
	"h1":         true,
	"h2":         true,
	"h3":         true,
	"h4":         true,
	"h5":         true,
	"h6":         true,
	"sub":        true,
	"sup":        true,
	"section":    true,
	"article":    true,
	"header":     true,
	"footer":     true,
	"nav":        true,
	"main":       true,
	"figure":     true,
	"figcaption": true,
	"caption":    true,
	"colgroup":   true,
	"col":        true,
	// Document structural tags (golang.org/x/net/html always wraps fragments
	// in <html><head><body>); we treat them as transparent so the wrapper
	// nodes the parser inserts don't generate spurious findings.
	"html": true,
	"head": true,
	"body": true,
}

// blockedTags enumerates tags whose content is removed in full and a
// SeverityError finding is emitted (tag classification row "错误（删除）"). Each entry
// maps to the rule id surfaced in Finding.RuleID.
var blockedTags = map[string]string{
	"script": RuleTagScriptBlocked,
	"iframe": RuleTagIframeBlocked,
	"object": RuleTagObjectBlocked,
	"embed":  RuleTagEmbedBlocked,
	"form":   RuleTagFormBlocked,
	"input":  RuleTagInputBlocked,
	"select": RuleTagInputBlocked,
	"option": RuleTagInputBlocked,
	"button": RuleTagInputBlocked,
	"link":   RuleTagLinkBlocked,
	"meta":   RuleTagMetaBlocked,
	"base":   RuleTagBaseBlocked,
}

// warnAutofixTags enumerates tags rewritten when AutoFix is true (tag
// classification row "警告 + 自动修复"). The replacement strategy is per-tag.
var warnAutofixTags = map[string]string{
	"font":    RuleTagFontToSpan,
	"center":  RuleTagCenterToDiv,
	"marquee": RuleTagMarqueeToText,
	"blink":   RuleTagBlinkToText,
}

// classifyTag returns the rule kind for the given lower-case tag name.
//
// kind is one of "allow", "warn", "error", "unknown". For "warn" / "error",
// ruleID names the firing rule; for "unknown", the caller falls back to
// allow-list-by-default but emits a hint via RuleTagUnknownStripped only when
// the tag is structurally suspect (e.g. <object>-like). The cli's existing
// `htmlTagRe` regex is the de-facto allow-list shipping with the codebase, so
// we don't aggressively flag anything outside `allowedTags` — drop-through
// preserves user intent for niche tags (e.g. `<details>` / `<summary>`) that
// browsers + Feishu native renderer already handle.
func classifyTag(tag string) (kind, ruleID string) {
	tag = strings.ToLower(tag)
	if allowedTags[tag] {
		return "allow", ""
	}
	if id, ok := blockedTags[tag]; ok {
		return "error", id
	}
	if id, ok := warnAutofixTags[tag]; ok {
		return "warn", id
	}
	// Unknown / niche tags: pass through silently. The cli's existing
	// `htmlTagRe` (mail_quote.go:333) tolerates them too. Users authoring
	// HTML in Feishu native classes (`adit-html-block*`, `history-quote-*`,
	// `lark-mail-doc-quote`) hit this path — they MUST pass through unchanged
	// so reply / forward quote markup survives lint round-trips.
	return "allow", ""
}

// Attribute / URL / style classification --------------------------------------

// allowedURLSchemes lists URL schemes that pass through hyperlink-bearing
// attrs (`href`, `src`, `cite`, `formaction` etc.). Allowed: http(s), mailto,
// cid, data:image/*; everything else (notably javascript: and vbscript:) is
// blocked. Empty / relative URLs (no scheme) are always
// allowed because they resolve relatively at render time and pose no
// injection vector.
var allowedURLSchemes = map[string]bool{
	"http":   true,
	"https":  true,
	"mailto": true,
	"cid":    true,
}

// blockedURLSchemes is the explicit deny-list. data:image/* is special-cased
// in classifyURLValue.
var blockedURLSchemes = map[string]bool{
	"javascript": true,
	"vbscript":   true,
	"file":       true,
}

// classifyURLValue returns ("ok", "") if the URL value is acceptable, or
// ("error", ruleID) when it must be removed (javascript:/vbscript:/file:),
// or ("warn", ruleID) when the scheme is unrecognised but not actively
// dangerous. Empty values pass through (browsers ignore them).
func classifyURLValue(raw string) (kind, ruleID string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "ok", ""
	}
	// Strip leading whitespace + control bytes that could obscure the
	// scheme (e.g. "java\tscript:..."). The html-parser already strips
	// stray whitespace at attribute boundaries; this is defence-in-depth
	// for older clients that paste from Word with U+0009 / U+0020 inside
	// the scheme prefix.
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, value)

	// Find the colon delimiter; everything before it is the scheme.
	colon := strings.IndexByte(value, ':')
	if colon < 0 {
		// No scheme → relative URL → allow.
		return "ok", ""
	}
	scheme := strings.ToLower(value[:colon])
	rest := value[colon+1:]

	switch {
	case allowedURLSchemes[scheme]:
		return "ok", ""
	case scheme == "data":
		// data:image/* is whitelisted; anything else (e.g. data:text/html;...)
		// is rejected. The check tolerates any subtype under image/* (png /
		// jpeg / gif / svg+xml / webp) so users embedding base64 thumbnails
		// don't trip the rule.
		rest = strings.TrimSpace(rest)
		if strings.HasPrefix(strings.ToLower(rest), "image/") {
			return "ok", ""
		}
		return "error", RuleAttrJSURLBlocked
	case blockedURLSchemes[scheme]:
		return "error", RuleAttrJSURLBlocked
	default:
		// Unknown scheme: surface a warning so users see it but don't
		// drop legitimate webcal:/tel: / similar in case downstream
		// renders eventually support them.
		return "warn", RuleAttrUnsafeSchemeBlocked
	}
}

// urlAttributes lists attributes whose value is a URL and must therefore
// pass classifyURLValue. Lower-case canonical names.
var urlAttributes = map[string]bool{
	"href":       true,
	"src":        true,
	"cite":       true,
	"formaction": true,
	"action":     true,
	"background": true,
	"poster":     true,
}

// allowedStyleProps enumerates CSS property names that pass through the
// inline `style="..."` attribute. Everything else is removed from the
// property list and surfaced via STYLE_PROPERTY_DROPPED.
//
// `border-*` / `padding-*` / `margin-*` are treated as prefix matches by
// classifyStyleProperty so the four directional variants (border-top etc.)
// are all admitted without enumerating each.
var allowedStyleProps = map[string]bool{
	"color":            true,
	"background-color": true,
	"font-size":        true,
	"font-weight":      true,
	"font-style":       true,
	"text-align":       true,
	"text-decoration":  true,
	"line-height":      true,
	"padding":          true,
	"margin":           true,
	"border":           true,
	"width":            true,
	"height":           true,
	"display":          true,
	"text-indent":      true,
	// Quote-block / native Feishu styles (tag classification "通过").
	// Whitespace + word-break are part of the existing `<pre>` / quote
	// wrapper styles in mail_quote.go (e.g. `bodyDivStyle`).
	"white-space":         true,
	"word-break":          true,
	"word-wrap":           true,
	"overflow":            true,
	"overflow-wrap":       true,
	"vertical-align":      true,
	"list-style":          true,
	"list-style-type":     true,
	"list-style-position": true,
	"transition":          true,
	"font-family":         true,
	"text-transform":      true,
	"hyphens":             true,
	"max-width":           true,
	"min-width":           true,
	"max-height":          true,
	"min-height":          true,
	"border-radius":       true,
	"box-sizing":          true,
	"opacity":             true,
	"cursor":              true,
}

// stylePropAllowedPrefixes enumerates property name prefixes treated as
// allowed regardless of suffix (e.g. "border-*"). A trailing "-" makes the
// prefix self-documenting.
var stylePropAllowedPrefixes = []string{
	"border-",
	"padding-",
	"margin-",
}

// classifyStyleProperty reports whether the given lower-case property name
// is in the allow-list (incl. prefix matches).
func classifyStyleProperty(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	if allowedStyleProps[name] {
		return true
	}
	for _, p := range stylePropAllowedPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// isEventHandlerAttr reports whether the attribute name is a DOM event
// handler (`on*`). The lib removes every such attribute regardless of its
// value (tag classification row "错误（删除）" + the well-known XSS vector).
func isEventHandlerAttr(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "on") {
		return false
	}
	if len(name) <= 2 {
		return false
	}
	// Defence-in-depth: avoid matching legitimate attrs whose name happens
	// to begin with "on" (e.g. `onerror`-like attrs all start "on" + ascii
	// letter). The `>= 'a'` check filters out "on-something" with hyphens.
	c := name[2]
	return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}
