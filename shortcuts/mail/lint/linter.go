// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lint

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// MaxExcerptBytes caps the raw-HTML excerpt embedded in a Finding.Excerpt so
// a single offending tag with megabyte content can't bloat the envelope JSON.
// Lint operates on bytes only, but the excerpt representation must not be
// size-amplifying.
const MaxExcerptBytes = 200

// Run lints the given HTML body and returns a structured Report.
// Report.CleanedHTML contains the rewritten HTML (warnings rewritten + errors
// deleted) — the autofix is unconditional.
//
// IMPORTANT: when the input is empty or plain-text (no HTML markup detected
// by the cli's existing `bodyIsHTML` heuristic), callers should short-circuit
// with EmptyReport(html) instead of paying the parse cost. Run still handles
// this gracefully — html.Parse on plain text wraps the input in
// <html><head></head><body>...</body></html>, and the lib's pass-through
// rendering will reproduce the original text — but the round-trip is wasteful
// and produces no findings.
func Run(html string, opts Options) Report {
	if html == "" {
		return EmptyReport("")
	}

	rep := Report{
		Applied: []Finding{},
		Blocked: []Finding{},
	}

	// We use html.ParseFragment so users authoring fragment-style snippets
	// (the canonical compose-5 input shape — `<div>...</div>` rather than a
	// full document) don't get implicit <html><head><body> wrappers
	// re-rendered. The "body" insertion mode matches what html.Parse would
	// have done internally for a fragment but skips the structural wrappers
	// at render time.
	bodyContext := &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Body, Data: "body"}
	nodes, err := xhtml.ParseFragment(strings.NewReader(html), bodyContext)
	if err != nil {
		// Parser failure is exceptional (the parser is permissive by design);
		// fall back to the original input so we don't lose user content.
		return EmptyReport(html)
	}

	// Wrap fragment nodes in a synthetic root so the recursive walker has a
	// uniform parent pointer to mutate.
	root := &xhtml.Node{Type: xhtml.DocumentNode}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	walk(root, &rep)
	// nativeCtx tracks per-Run() state so positional ids (e.g. data-ol-id)
	// are deterministic across multiple Run() calls on the same input —
	// keying off the document-traversal order rather than heap pointers,
	// so cleaned_html is byte-stable and amenable to golden-file tests / CI
	// diff / cache-key reuse.
	nctx := &nativeCtx{olIDs: map[*xhtml.Node]string{}}
	applyFeishuNativeStyles(root, &rep, nctx)

	rep.HasErrorFindings = len(rep.Blocked) > 0
	rep.HasWarningFindings = len(rep.Applied) > 0
	rep.CleanedHTML = renderFragment(root)

	return rep
}

// walk visits every element node under parent, applying tag/attr/style
// classification. Children are iterated via the next-sibling pointer because
// we mutate the tree in place (replace / remove nodes).
//
// The walker is iterative-style via explicit recursion because the html
// parser's typical nesting depth (≤ 256 by default) is well below Go's
// goroutine stack limit; the existing draft package's plainTextFromHTML
// (mail/draft/htmltext.go) similarly recurses for the same reason.
func walk(parent *xhtml.Node, rep *Report) {
	child := parent.FirstChild
	for child != nil {
		next := child.NextSibling
		if child.Type == xhtml.ElementNode {
			processElement(parent, child, rep)
		}
		// child may have been removed/replaced by processElement; recurse
		// only if it still has the original parent (i.e. wasn't deleted).
		// The html parser sets Parent on every node, so a removed-then-
		// reattached node still recurses correctly via its new Parent.
		if child.Parent != nil {
			walk(child, rep)
		}
		child = next
	}
}

// processElement applies the element-level classification cascade:
//  1. tag → allow / warn-rewrite / error-delete
//  2. attributes → on*-handlers, URL-bearing attrs (scheme allow-list),
//     style attribute (CSS property allow-list)
func processElement(parent, n *xhtml.Node, rep *Report) {
	tagName := strings.ToLower(n.Data)
	kind, ruleID := classifyTag(tagName)

	switch kind {
	case "error":
		rep.Blocked = append(rep.Blocked, Finding{
			RuleID:    ruleID,
			Severity:  SeverityError,
			TagOrAttr: tagName,
			Excerpt:   excerptOf(n),
			Hint:      hintForBlockedTag(tagName),
		})
		// Always remove blocked tags — the writing-path safety floor has no
		// opt-out; `--no-lint` is not provided.
		parent.RemoveChild(n)
		return

	case "warn":
		// Always rewrite (e.g. <font>→<span style>) and surface the finding.
		rep.Applied = append(rep.Applied, Finding{
			RuleID:    ruleID,
			Severity:  SeverityWarning,
			TagOrAttr: tagName,
			Excerpt:   excerptOf(n),
			Hint:      hintForWarnTag(tagName),
		})
		rewriteWarnTag(n, tagName)
		// Recurse into the rewritten node by falling through; the rewrite
		// preserved children as-is.
		// fall through to attribute scan
	case "allow":
		// no-op
	}

	// Attribute scan: build a new attribute slice, dropping/sanitising as we
	// go and surfacing findings.
	if len(n.Attr) > 0 {
		processAttributes(n, rep)
	}
}

// processAttributes walks the attribute list and:
//   - drops on*-handlers (always; surfaced as error)
//   - drops URL-bearing attrs whose value uses a forbidden scheme
//   - filters the `style` attribute property-by-property against the allow-list
//
// Other attributes pass through unchanged. The cli's existing
// `validateInlineCIDs` (helpers.go:2226) handles `cid:`-specific checks;
// the lint must not duplicate that responsibility.
func processAttributes(n *xhtml.Node, rep *Report) {
	keep := n.Attr[:0]
	for _, attr := range n.Attr {
		name := strings.ToLower(attr.Key)

		// 1. on*-handlers → always drop, error-tier.
		if isEventHandlerAttr(name) {
			rep.Blocked = append(rep.Blocked, Finding{
				RuleID:    RuleAttrEventHandlerBlocked,
				Severity:  SeverityError,
				TagOrAttr: name,
				Excerpt:   truncateExcerpt(attr.Key + "=\"" + attr.Val + "\""),
				Hint:      "Removed event handler attribute (on*)",
			})
			continue
		}

		// 2. URL-bearing attrs → check scheme allow-list.
		if urlAttributes[name] {
			kind, ruleID := classifyURLValue(attr.Val)
			switch kind {
			case "error":
				rep.Blocked = append(rep.Blocked, Finding{
					RuleID:    ruleID,
					Severity:  SeverityError,
					TagOrAttr: name,
					Excerpt:   truncateExcerpt(attr.Key + "=\"" + attr.Val + "\""),
					Hint:      "Removed dangerous URL scheme (allowed: http/https/mailto/cid/data:image/*)",
				})
				continue
			case "warn":
				rep.Blocked = append(rep.Blocked, Finding{
					RuleID:    ruleID,
					Severity:  SeverityError,
					TagOrAttr: name,
					Excerpt:   truncateExcerpt(attr.Key + "=\"" + attr.Val + "\""),
					Hint:      "Removed URL with unrecognised scheme (allowed: http/https/mailto/cid/data:image/*)",
				})
				// Always drop the attribute — writing-path safety floor (the
				// URL would not render correctly anyway).
				continue
			}
		}

		// 3. `style` attribute → property-by-property allow-list.
		if name == "style" {
			cleaned, dropped := sanitiseStyleAttr(attr.Val)
			for _, prop := range dropped {
				rep.Applied = append(rep.Applied, Finding{
					RuleID:    RuleStylePropertyDropped,
					Severity:  SeverityWarning,
					TagOrAttr: "style." + prop,
					Excerpt:   truncateExcerpt(prop),
					Hint:      "Removed CSS property not in allowlist (see references/lark-mail-html.md)",
				})
			}
			if len(dropped) == 0 {
				// Byte-stable when no property was dropped: leave the
				// attribute exactly as authored so lint round-trips are
				// idempotent on clean input.
				keep = append(keep, attr)
				continue
			}
			if cleaned == "" {
				// All properties dropped — remove the attribute entirely.
				continue
			}
			attr.Val = cleaned
			keep = append(keep, attr)
			continue
		}

		// 4. Pass-through.
		keep = append(keep, attr)
	}
	n.Attr = keep
}

// rewriteWarnTag replaces a warning-tier tag with its Feishu-native
// equivalent in place: <font> → <span style="..."> with color/face/size
// distilled into inline style; <center> → <div style="text-align:center">;
// <marquee>/<blink> → <span> (text-only, animation discarded — collapsing
// to a span keeps the children but drops the deprecated animation effect).
func rewriteWarnTag(n *xhtml.Node, tagName string) {
	switch tagName {
	case "font":
		// Distill <font color="..." face="..." size="...">.
		var styles []string
		var keepAttrs []xhtml.Attribute
		for _, attr := range n.Attr {
			switch strings.ToLower(attr.Key) {
			case "color":
				if v := strings.TrimSpace(attr.Val); v != "" {
					styles = append(styles, "color:"+v)
				}
			case "face":
				if v := strings.TrimSpace(attr.Val); v != "" {
					styles = append(styles, "font-family:"+v)
				}
			case "size":
				if v := mapFontSize(attr.Val); v != "" {
					styles = append(styles, "font-size:"+v)
				}
			default:
				keepAttrs = append(keepAttrs, attr)
			}
		}
		// Merge any existing style attribute already present on the <font>
		// (rare but possible).
		if len(styles) > 0 {
			merged := strings.Join(styles, ";")
			styleIdx := -1
			for i, attr := range keepAttrs {
				if strings.ToLower(attr.Key) == "style" {
					styleIdx = i
					break
				}
			}
			if styleIdx >= 0 {
				existing := strings.TrimRight(keepAttrs[styleIdx].Val, "; ")
				if existing != "" {
					merged = existing + ";" + merged
				}
				keepAttrs[styleIdx].Val = merged
			} else {
				keepAttrs = append(keepAttrs, xhtml.Attribute{Key: "style", Val: merged})
			}
		}
		n.Data = "span"
		n.DataAtom = atom.Span
		n.Attr = keepAttrs

	case "center":
		// <center> → <div style="text-align:center">. Existing style attr
		// (if any) is merged with text-align prepended.
		styleIdx := -1
		for i, attr := range n.Attr {
			if strings.ToLower(attr.Key) == "style" {
				styleIdx = i
				break
			}
		}
		newStyle := "text-align:center"
		if styleIdx >= 0 {
			existing := strings.TrimRight(n.Attr[styleIdx].Val, "; ")
			if existing != "" {
				newStyle = newStyle + ";" + existing
			}
			n.Attr[styleIdx].Val = newStyle
		} else {
			n.Attr = append(n.Attr, xhtml.Attribute{Key: "style", Val: newStyle})
		}
		n.Data = "div"
		n.DataAtom = atom.Div

	case "marquee", "blink":
		// Both deprecated; collapse to <span> so children survive.
		n.Data = "span"
		n.DataAtom = atom.Span
		// Strip marquee-specific attributes (direction, scrollamount, ...)
		// so the rewritten span is plain.
		var keepAttrs []xhtml.Attribute
		for _, attr := range n.Attr {
			if strings.ToLower(attr.Key) == "style" || strings.ToLower(attr.Key) == "class" || strings.ToLower(attr.Key) == "id" {
				keepAttrs = append(keepAttrs, attr)
			}
		}
		n.Attr = keepAttrs
	}
}

// mapFontSize maps the legacy <font size="N"> values (1..7) to a CSS px
// equivalent, matching the mapping used by Feishu mail-editor's renderer.
// Out-of-range values fall through to the empty string so the property is
// dropped (better than emitting an arbitrary value).
func mapFontSize(raw string) string {
	switch strings.TrimSpace(raw) {
	case "1":
		return "10px"
	case "2":
		return "13px"
	case "3":
		return "16px"
	case "4":
		return "18px"
	case "5":
		return "24px"
	case "6":
		return "32px"
	case "7":
		return "48px"
	default:
		return ""
	}
}

// sanitiseStyleAttr filters a `style="prop1:val; prop2:val"` declaration
// against the property allow-list. Returns the cleaned style text (joined
// with "; " separators) and a slice of dropped property names (lower-case)
// so the caller can surface STYLE_PROPERTY_DROPPED findings.
//
// NOTE: We do NOT validate property values — only property names. The style
// attribute is filtered by CSS property allow-list; value-level validation
// (e.g. URL safety inside `background-image: url(...)`) is delegated to the
// urlAttributes path because such values typically appear in `src` / `href`
// attrs in compose-5 templates. Users authoring `background-image: url(http:...)`
// in inline style will see the property pass — the URL inside is not a
// security concern at the inline-style level since URL fetching from style
// is restricted by the rendering layer's CSP regardless.
func sanitiseStyleAttr(raw string) (cleaned string, dropped []string) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	parts := strings.Split(raw, ";")
	keep := make([]string, 0, len(parts))
	for _, part := range parts {
		decl := strings.TrimSpace(part)
		if decl == "" {
			continue
		}
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			// Malformed declaration; drop and surface as a finding so the
			// user notices.
			dropped = append(dropped, decl)
			continue
		}
		name := strings.ToLower(strings.TrimSpace(decl[:colon]))
		if !classifyStyleProperty(name) {
			dropped = append(dropped, name)
			continue
		}
		keep = append(keep, decl)
	}
	cleaned = strings.Join(keep, "; ")
	return cleaned, dropped
}

// hintForBlockedTag returns a hint for an error-blocked tag.
func hintForBlockedTag(tag string) string {
	switch tag {
	case "script":
		return "Removed whole tag (XSS risk)"
	case "iframe", "object", "embed":
		return "Removed whole tag (external embeds not allowed; use <img> or a body link for rich media)"
	case "form", "input", "select", "option", "button":
		return "Removed whole tag (forms not allowed in email body)"
	case "link":
		return "Removed <link> (external CSS / resources not allowed)"
	case "meta":
		return "Removed <meta> (viewport / refresh declarations not allowed)"
	case "base":
		return "Removed <base> (URL base rewrites not allowed)"
	default:
		return "Removed whole tag (tag not allowed)"
	}
}

// hintForWarnTag returns a hint for a warning-tier tag.
func hintForWarnTag(tag string) string {
	switch tag {
	case "font":
		return "Rewritten as <span style=\"...\"> (modern HTML expresses size / color via inline style)"
	case "center":
		return "Rewritten as <div style=\"text-align:center\"> (deprecated <center> tag)"
	case "marquee", "blink":
		return "Rewritten as <span> (animations not supported; text preserved)"
	default:
		return "Rewritten in modern HTML shape"
	}
}

// excerptOf renders the offending node's open-tag header into a short string
// suitable for surfacing in a Finding.Excerpt. We render only the tag header
// (not the full subtree) so a single offending <script> with megabytes of
// content doesn't bloat the envelope JSON. truncateExcerpt enforces the cap.
func excerptOf(n *xhtml.Node) string {
	if n == nil {
		return ""
	}
	var buf bytes.Buffer
	buf.WriteByte('<')
	buf.WriteString(n.Data)
	for _, attr := range n.Attr {
		buf.WriteByte(' ')
		buf.WriteString(attr.Key)
		if attr.Val != "" {
			buf.WriteString(`="`)
			buf.WriteString(attr.Val)
			buf.WriteByte('"')
		}
	}
	buf.WriteString(`...>`)
	return truncateExcerpt(buf.String())
}

// truncateExcerpt enforces MaxExcerptBytes; longer excerpts are truncated and
// suffixed with " ...". We measure bytes (not runes) because the cap is about
// envelope size, not character count — multibyte UTF-8 in an excerpt is
// uncommon in HTML markup excerpts.
func truncateExcerpt(s string) string {
	if len(s) <= MaxExcerptBytes {
		return s
	}
	return s[:MaxExcerptBytes-4] + " ..."
}

// renderFragment serialises a fragment-rooted html tree to a string. We use
// the html package's Render which always emits the document-style markup;
// for fragment input we strip the implicit <html><head></head><body>...</body></html>
// wrapper that html.Parse adds.
func renderFragment(root *xhtml.Node) string {
	var buf bytes.Buffer
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		_ = xhtml.Render(&buf, child)
	}
	return buf.String()
}

// applyFeishuNativeStyles walks the tree (post-tag-pass) and ensures elements
// that have known Feishu mail-editor native inline styles get them autofilled.
// User-supplied inline styles always take precedence — this pass is purely
// additive (only fills in missing properties). The pass always runs; there
// is no opt-out.
//
// Rules (visual parity with Feishu mail-editor's own native inline styles):
//
//	<ol> / <ul>     → margin-top:0;margin-bottom:0;margin-left:0;padding-left:0;list-style-position:inside
//	<li>            → line-height:1.6;margin-top:0px;margin-bottom:0px;padding-left:0;
//	                  margin-left:0;display:list-item;list-style-position:inside;
//	                  list-style-type:decimal (ol parent) | disc (ul parent);
//	                  font-family:inherit;font-size:14px
//	<blockquote>    → class=lark-mail-doc-quote + padding-left:0;color:rgb(100,106,115);
//	                  border-left:2px solid rgb(187,191,196);margin:0
//	                  direct children wrapped in <div dir=auto style="font-size:14px;padding-left:12px">
//	<a>             → class="not-doclink" + cursor:pointer;text-decoration:none;
//	                  color:rgb(20,86,240)
//	<p>             → rewritten to
//	                  <div style="margin-top:4px;margin-bottom:4px;line-height:1.6">
//	                    <div dir="auto" style="font-size:14px">...children...</div>
//	                  </div>
//	                  This is the only rewrite that changes tree shape; the
//	                  others only touch attributes.
//
// nativeCtx accumulates per-Run() state shared across the recursive
// applyFeishuNativeStyles walk. The only field today is the <ol>→id map
// used to seed `data-ol-id` deterministically (see nodeShortID).
type nativeCtx struct {
	olIDs map[*xhtml.Node]string
}

// olID returns the deterministic id for the given <ol> node, allocating a
// fresh positional id on first sight. Each call within the same Run() that
// targets the same node returns the same id. Different inputs may collide
// (the map resets per Run()), but `data-ol-id` is per-document so cross-
// document equality is irrelevant.
func (c *nativeCtx) olID(n *xhtml.Node) string {
	if id, ok := c.olIDs[n]; ok {
		return id
	}
	id := nodeShortID(len(c.olIDs))
	c.olIDs[n] = id
	return id
}

func applyFeishuNativeStyles(parent *xhtml.Node, rep *Report, nctx *nativeCtx) {
	child := parent.FirstChild
	for child != nil {
		next := child.NextSibling
		if child.Type == xhtml.ElementNode {
			switch strings.ToLower(child.Data) {
			case "ol", "ul":
				wrapNonLIListChildren(child, rep)
				ensureFeishuListStyle(child, rep)
			case "li":
				ensureFeishuListItemStyle(child, parent, rep, nctx)
			case "blockquote":
				ensureFeishuBlockquoteStyle(child, rep)
			case "a":
				ensureFeishuLinkStyle(child, rep)
			case "p":
				rewritePToFeishuDiv(child, rep)
			}
		}
		// child may have been mutated (children moved into a wrapper);
		// recurse only if it's still attached.
		if child.Parent != nil {
			applyFeishuNativeStyles(child, rep, nctx)
		}
		child = next
	}
}

// wrapNonLIListChildren scans the direct children of an <ol>/<ul> and wraps
// any element that is not <li> in a fresh <li>. HTML spec is explicit that
// `<ul>` / `<ol>` may only contain `<li>` (plus optional `<script>` /
// `<template>`); browsers parse a non-<li> child by silently hoisting it out
// of the list, which destroys the intended visual nesting in mail clients.
//
// Whitespace-only text nodes are left in place (they were already going to
// be stripped by stripWhitespaceTextChildren in ensureFeishuListStyle).
// Non-element / non-whitespace text nodes are preserved as-is — the spec
// allows them inside `<li>` only, but most clients render them fine inline
// and we don't want to silently mutate user-authored content beyond the
// minimum needed to satisfy the structural rule.
func wrapNonLIListChildren(list *xhtml.Node, rep *Report) {
	child := list.FirstChild
	for child != nil {
		next := child.NextSibling
		if child.Type == xhtml.ElementNode && strings.ToLower(child.Data) != "li" {
			// Wrap the offending element in a fresh <li>. The <li> inherits
			// no attributes — the wrapped element keeps its own styling so
			// nested-list indentation declared on the inner <ul>/<ol>
			// survives. The recursive walk re-enters the new <li> later to
			// stamp the native list-item style on it.
			li := &xhtml.Node{
				Type:     xhtml.ElementNode,
				Data:     "li",
				DataAtom: atom.Li,
			}
			rep.Applied = append(rep.Applied, Finding{
				RuleID:    RuleListDirectChildNonLI,
				Severity:  SeverityWarning,
				TagOrAttr: child.Data,
				Excerpt:   excerptOf(child),
				Hint:      "Wrapped non-<li> child of <" + list.Data + "> in a synthetic <li> (HTML spec requires <ul>/<ol> children to be <li>)",
			})
			list.InsertBefore(li, child)
			list.RemoveChild(child)
			li.AppendChild(child)
		}
		child = next
	}
}

// hasInlineStyleProp reports whether the given style="..." string already
// declares the named property. Lookup is case-insensitive on the property
// name; whitespace around `:` and `;` is tolerated.
//
// Shorthand expansion: `margin-*` is also considered set when shorthand
// `margin:` is present (same for `padding-*` / `padding:`, and the four
// `border-*-style/color/width` longhands when `border:` shorthand is set).
// Without this, ensureInlineStyleProps would append e.g. `margin-left:0`
// onto a `margin:0 0 0 24px` shorthand and clobber the user-authored
// 24px (mail-editor's native nested-list indent uses this exact shape).
func hasInlineStyleProp(style, prop string) bool {
	prop = strings.ToLower(strings.TrimSpace(prop))
	if prop == "" {
		return false
	}
	var shorthand string
	switch {
	case strings.HasPrefix(prop, "margin-"):
		shorthand = "margin"
	case strings.HasPrefix(prop, "padding-"):
		shorthand = "padding"
	case strings.HasPrefix(prop, "border-"):
		shorthand = "border"
	}
	for _, decl := range strings.Split(style, ";") {
		decl = strings.TrimSpace(decl)
		if decl == "" {
			continue
		}
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(decl[:colon]))
		if name == prop {
			return true
		}
		if shorthand != "" && name == shorthand {
			return true
		}
	}
	return false
}

// ensureInlineStyleProps appends each (prop, val) pair to the element's style
// attribute *only if* the property is not already declared. Returns true if
// any property was added (so callers can record an Applied finding).
//
// User-authored values take precedence — we never overwrite existing
// declarations even if they differ from our default, because the user may
// have an intentional reason (e.g. wider list-margin for an outline-style
// document).
func ensureInlineStyleProps(n *xhtml.Node, propsInOrder [][2]string) bool {
	styleIdx := -1
	existing := ""
	for i, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "style" {
			styleIdx = i
			existing = attr.Val
			break
		}
	}
	additions := make([]string, 0, len(propsInOrder))
	for _, kv := range propsInOrder {
		if !hasInlineStyleProp(existing, kv[0]) {
			additions = append(additions, kv[0]+":"+kv[1])
		}
	}
	if len(additions) == 0 {
		return false
	}
	merged := strings.Join(additions, ";")
	if existing != "" {
		merged = strings.TrimRight(existing, "; ") + ";" + merged
	}
	if styleIdx >= 0 {
		n.Attr[styleIdx].Val = merged
	} else {
		n.Attr = append(n.Attr, xhtml.Attribute{Key: "style", Val: merged})
	}
	return true
}

// stripWhitespaceTextChildren removes direct text-node children of n that
// contain only whitespace (newlines, indentation). On <ol> / <ul> these
// inter-<li> whitespace nodes are rendered by Feishu mail-editor as
// extra inline text — visible as a blank line between list items even
// when every <li> has 0 margin and the full Feishu-native marker set.
// AI-authored HTML almost always contains pretty-printed indentation
// inside lists, so this strip is essential to get gap-free rendering.
func stripWhitespaceTextChildren(n *xhtml.Node) bool {
	changed := false
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) == "" {
			n.RemoveChild(child)
			changed = true
		}
		child = next
	}
	return changed
}

// wrapTextChildrenInFeishuSpans wraps each direct text-node child of n in
// the canonical Feishu mail-editor inline shape:
//
//	<span style="font-family:inherit"><span style="color:rgb(0,0,0)">text</span></span>
//
// This is what every text leaf produced by the Feishu mail-editor itself
// looks like (Quill-like editor model — every text leaf carries default
// font / color override spans). Without these wrapper spans the renderer
// falls back to its "untracked text" path, which inserts default-line
// spacing between siblings — visually a blank line between list items.
//
// Whitespace-only text nodes are left untouched so we don't bloat the DOM
// with wrapper spans around indentation / line breaks. Only direct text
// children are wrapped; nested element subtrees (e.g. <b>text</b>) are
// left alone (the user has already structured them; deeper traversal here
// risks breaking intentional nesting).
func wrapTextChildrenInFeishuSpans(n *xhtml.Node) bool {
	changed := false
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		if child.Type == xhtml.TextNode && strings.TrimSpace(child.Data) != "" {
			text := child.Data
			outer := &xhtml.Node{
				Type:     xhtml.ElementNode,
				Data:     "span",
				DataAtom: atom.Span,
				Attr:     []xhtml.Attribute{{Key: "style", Val: "font-family:inherit"}},
			}
			inner := &xhtml.Node{
				Type:     xhtml.ElementNode,
				Data:     "span",
				DataAtom: atom.Span,
				Attr:     []xhtml.Attribute{{Key: "style", Val: "color:rgb(0,0,0)"}},
			}
			inner.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: text})
			outer.AppendChild(inner)
			n.RemoveChild(child)
			if next != nil {
				n.InsertBefore(outer, next)
			} else {
				n.AppendChild(outer)
			}
			changed = true
		}
		child = next
	}
	return changed
}

// reorderAttrs reorders n.Attr so attributes named in `order` come first
// (in that exact sequence); remaining attributes are appended after in
// their original relative order. Lookup is case-insensitive on names.
//
// Feishu mail-editor's renderer is observed to walk attributes in
// declaration order on certain hot paths (notably native list-block /
// paragraph detection); inline-style declaration order matters for the
// same reason. Matching the sequence emitted by Feishu mail-editor
// itself is what flips the renderer onto the native, gap-free path.
func reorderAttrs(n *xhtml.Node, order []string) {
	if len(n.Attr) == 0 {
		return
	}
	sorted := make([]xhtml.Attribute, 0, len(n.Attr))
	used := make([]bool, len(n.Attr))
	// Pass 1: insert attrs whose name is in `order`, in `order` sequence.
	for _, k := range order {
		lk := strings.ToLower(k)
		for i, attr := range n.Attr {
			if used[i] {
				continue
			}
			if strings.ToLower(attr.Key) == lk {
				sorted = append(sorted, attr)
				used[i] = true
				break
			}
		}
	}
	// Pass 2: append remaining attrs (not in `order`) in original order.
	for i, attr := range n.Attr {
		if !used[i] {
			sorted = append(sorted, attr)
		}
	}
	n.Attr = sorted
}

// ensureAttr sets the given attribute to val if absent, leaving an
// existing attribute (even with a different value) untouched. Returns true
// if the attribute was newly added.
func ensureAttr(n *xhtml.Node, key, val string) bool {
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == strings.ToLower(key) {
			return false
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: key, Val: val})
	return true
}

// ensureClass appends the given class name to the element's class attribute
// (creating the attribute if absent). Whitespace-separated tokens are
// preserved. Returns true if the class was newly added.
func ensureClass(n *xhtml.Node, cls string) bool {
	classIdx := -1
	existing := ""
	for i, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "class" {
			classIdx = i
			existing = attr.Val
			break
		}
	}
	for _, c := range strings.Fields(existing) {
		if c == cls {
			return false
		}
	}
	if existing == "" {
		if classIdx >= 0 {
			n.Attr[classIdx].Val = cls
		} else {
			n.Attr = append(n.Attr, xhtml.Attribute{Key: "class", Val: cls})
		}
	} else {
		n.Attr[classIdx].Val = existing + " " + cls
	}
	return true
}

// ensureFeishuListStyle adds Feishu-native inline styles + the data-list-*
// marker that the renderer keys off to recognise the list as a native
// list-block (vs. a fallback ad-hoc list with default browser styling and
// the visual "blank line between items" issue).
//
// For <ol>, also seeds `start="1"` (when absent). The `data-ol-id` value
// is allocated lazily from a per-Run() positional counter held on
// nativeCtx and stamped on each <li> by ensureFeishuListItemStyle — so
// identical input HTML always produces identical cleaned_html (byte
// stable across runs).
func ensureFeishuListStyle(n *xhtml.Node, rep *Report) {
	styleChanged := ensureInlineStyleProps(n, [][2]string{
		{"margin-top", "0px"},
		{"margin-bottom", "0px"},
		{"margin-left", "0px"},
		{"padding-left", "0px"},
		{"list-style-position", "inside"},
	})
	tag := strings.ToLower(n.Data)
	dataAttr := "data-list-bullet"
	if tag == "ol" {
		dataAttr = "data-list-number"
	}
	markerChanged := ensureAttr(n, dataAttr, "true")
	if tag == "ol" {
		if ensureAttr(n, "start", "1") {
			markerChanged = true
		}
		// data-ol-id is intentionally NOT set on the <ol> itself — the
		// mail-editor's own output puts data-ol-id only on <li>
		// children. Putting it on <ol> too triggers a different
		// renderer code path that reintroduces inter-item spacing.
	}
	// Force the canonical attribute order Feishu mail-editor emits.
	// Order matters for the renderer's native-list-block fast path.
	if tag == "ol" {
		reorderAttrs(n, []string{"start", "style", "data-list-number"})
	} else {
		reorderAttrs(n, []string{"style", "data-list-bullet"})
	}
	// Strip inter-<li> whitespace text nodes (newlines / indentation
	// from pretty-printed source HTML). Feishu mail-editor renders
	// these as visible blank lines between list items.
	if stripWhitespaceTextChildren(n) {
		markerChanged = true
	}
	if styleChanged || markerChanged {
		rep.Applied = append(rep.Applied, Finding{
			RuleID:    RuleStyleListNative,
			Severity:  SeverityWarning,
			TagOrAttr: n.Data,
			Excerpt:   excerptOf(n),
			Hint:      "Added native-list inline style + data-list-* marker + canonical attribute order (recognised as native list-block, avoiding fallback-render blank lines)",
		})
	}
}

// nodeShortID generates an 8-char deterministic id from the given positional
// index. Identical input HTML produces identical ids on every Run() call,
// so the lib is fully byte-stable — golden-file tests, CI diff, downstream
// caches keyed off cleaned_html all work correctly. The hash is FNV-32a on
// the index's decimal representation; 32 bits of hash space gives ample
// margin for a single document's <ol> count.
func nodeShortID(index int) string {
	h := fnv.New32a()
	fmt.Fprintf(h, "%d", index)
	return fmt.Sprintf("%08x", h.Sum32())
}

// ensureFeishuListItemStyle adds Feishu-native inline styles + class +
// data-* marker to <li>. The list-style-type defaults to decimal/disc
// based on the parent <ol>/<ul> kind; class follows mail-editor
// internal naming (`temp-li number1` for ol-children, `temp-li bullet1`
// for ul-children). data-li-line / data-list mark the node as part of a
// native list-block so the renderer doesn't fall back to default browser
// list styling (which has the "blank line between items" visual).
func ensureFeishuListItemStyle(n, parent *xhtml.Node, rep *Report, nctx *nativeCtx) {
	listType := "disc"
	listKind := "bullet1"
	if parent != nil && strings.ToLower(parent.Data) == "ol" {
		listType = "decimal"
		listKind = "number1"
	}
	// CSS declaration order matches Feishu mail-editor's own output
	// exactly — the renderer's list-item fast-path is observed to
	// require this ordering, in addition to the marker set, for the
	// gap-free native list-block render.
	styleChanged := ensureInlineStyleProps(n, [][2]string{
		{"line-height", "1.6"},
		{"margin-top", "0px"},
		{"margin-bottom", "0px"},
		{"padding-left", "0px"},
		{"display", "list-item"},
		{"list-style-type", listType},
		{"font-family", "inherit"},
		{"font-size", "14px"},
		{"margin-left", "0px"},
		{"list-style-position", "inside"},
	})
	classChanged := false
	if ensureClass(n, "temp-li") {
		classChanged = true
	}
	if ensureClass(n, listKind) {
		classChanged = true
	}
	markerChanged := false
	if ensureAttr(n, "data-li-line", "true") {
		markerChanged = true
	}
	if ensureAttr(n, "data-list", listKind) {
		markerChanged = true
	}
	if ensureAttr(n, "dir", "auto") {
		markerChanged = true
	}
	// For ol-children, derive data-ol-id from the parent <ol>'s positional
	// index (so all siblings under the same <ol> get the same id, but the
	// <ol> itself stays clean — see ensureFeishuListStyle for why). The
	// nativeCtx makes this id document-deterministic across multiple
	// Run() calls on the same input. data-start is the 1-based position
	// among <li> siblings; it renders the visible number prefix.
	if parent != nil && strings.ToLower(parent.Data) == "ol" {
		if ensureAttr(n, "data-ol-id", nctx.olID(parent)) {
			markerChanged = true
		}
		pos := 1
		for c := parent.FirstChild; c != nil && c != n; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && strings.ToLower(c.Data) == "li" {
				pos++
			}
		}
		if ensureAttr(n, "data-start", fmt.Sprintf("%d", pos)) {
			markerChanged = true
		}
	}
	contentChanged := wrapTextChildrenInFeishuSpans(n)
	// Canonical li attribute order from Feishu mail-editor output:
	// class, data-li-line, data-list, data-ol-id, data-start, style, dir
	reorderAttrs(n, []string{"class", "data-li-line", "data-list", "data-ol-id", "data-start", "style", "dir"})
	if styleChanged || classChanged || markerChanged || contentChanged {
		rep.Applied = append(rep.Applied, Finding{
			RuleID:    RuleStyleListItemNative,
			Severity:  SeverityWarning,
			TagOrAttr: "li",
			Excerpt:   excerptOf(n),
			Hint:      "Added native-list-item inline style + class (temp-li " + listKind + ") + data marker + double-span text wrap + canonical attribute order",
		})
	}
}

// ensureFeishuBlockquoteStyle adds Feishu-native inline styles to
// <blockquote>: the iconic 2px left bar in subtle grey + an inner 12px
// content indent matching the Feishu mail editor output.
func ensureFeishuBlockquoteStyle(n *xhtml.Node, rep *Report) {
	styleChanged := ensureInlineStyleProps(n, [][2]string{
		{"padding-left", "0px"},
		{"color", "rgb(100,106,115)"},
		{"border-left", "2px solid rgb(187,191,196)"},
		{"margin", "0px"},
	})
	classChanged := ensureClass(n, "lark-mail-doc-quote")
	contentChanged := ensureBlockquoteContentWrapper(n)
	if styleChanged || classChanged || contentChanged {
		rep.Applied = append(rep.Applied, Finding{
			RuleID:    RuleStyleBlockquoteNative,
			Severity:  SeverityWarning,
			TagOrAttr: "blockquote",
			Excerpt:   excerptOf(n),
			Hint:      "Added blockquote native style (2px left grey bar + 12px content indent)",
		})
	}
}

// ensureBlockquoteContentWrapper wraps direct blockquote children in the same
// content div shape emitted by the Feishu mail editor. Existing native-shaped
// blockquotes are left unchanged.
func ensureBlockquoteContentWrapper(n *xhtml.Node) bool {
	first := firstNonWhitespaceChild(n)
	if first == nil {
		return false
	}
	if first.Type == xhtml.ElementNode && strings.EqualFold(first.Data, "div") && hasAttrValue(first, "dir", "auto") && hasInlineStyleProp(getStyleAttr(first), "padding-left") {
		return false
	}

	var children []*xhtml.Node
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		n.RemoveChild(c)
		children = append(children, c)
		c = next
	}
	inner := &xhtml.Node{
		Type:     xhtml.ElementNode,
		Data:     "div",
		DataAtom: atom.Div,
		Attr: []xhtml.Attribute{
			{Key: "dir", Val: "auto"},
			{Key: "style", Val: "font-size:14px;padding-left:12px"},
		},
	}
	for _, c := range children {
		inner.AppendChild(c)
	}
	n.AppendChild(inner)
	return true
}

func firstNonWhitespaceChild(n *xhtml.Node) *xhtml.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == xhtml.TextNode && strings.TrimSpace(c.Data) == "" {
			continue
		}
		return c
	}
	return nil
}

func hasAttrValue(n *xhtml.Node, key, val string) bool {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) && attr.Val == val {
			return true
		}
	}
	return false
}

func getStyleAttr(n *xhtml.Node) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, "style") {
			return attr.Val
		}
	}
	return ""
}

// ensureFeishuLinkStyle adds Feishu-native class + inline styles to <a>:
// `class="not-doclink"` is the marker the Feishu mail renderer keys off to
// avoid treating the link as an internal doc-share, and the inline style
// matches the editor's own colour and decoration.
func ensureFeishuLinkStyle(n *xhtml.Node, rep *Report) {
	classChanged := ensureClass(n, "not-doclink")
	styleChanged := ensureInlineStyleProps(n, [][2]string{
		{"cursor", "pointer"},
		{"text-decoration", "none"},
		{"color", "rgb(20,86,240)"},
	})
	if classChanged || styleChanged {
		rep.Applied = append(rep.Applied, Finding{
			RuleID:    RuleStyleLinkNative,
			Severity:  SeverityWarning,
			TagOrAttr: "a",
			Excerpt:   excerptOf(n),
			Hint:      "Added link style (class=not-doclink + brand blue + no underline)",
		})
	}
}

// rewritePToFeishuDiv changes <p>...</p> in place to
//
//	<div style="margin-top:4px;margin-bottom:4px;line-height:1.6">
//	  <div dir="auto" style="font-size:14px">...children...</div>
//	</div>
//
// User-supplied attributes on the original <p> are preserved on the outer
// div. The inner div uses native defaults only when the author did not
// already provide equivalent paragraph direction / font size.
//
// This is the only Feishu-native pass that mutates the tree shape (vs. just
// adding inline styles). Children are re-parented to the inner div so all
// their existing styles / classes survive untouched.
func rewritePToFeishuDiv(n *xhtml.Node, rep *Report) {
	// Detach existing children (kept in order).
	var children []*xhtml.Node
	for c := n.FirstChild; c != nil; {
		nx := c.NextSibling
		n.RemoveChild(c)
		children = append(children, c)
		c = nx
	}

	// Mutate <p> → <div>.
	n.Data = "div"
	n.DataAtom = atom.Div

	// Add outer wrapper inline styles (only fills missing properties).
	ensureInlineStyleProps(n, [][2]string{
		{"margin-top", "4px"},
		{"margin-bottom", "4px"},
		{"line-height", "1.6"},
	})

	// Build inner <div>. Native defaults are additive: an author-supplied
	// dir or font-size on the original <p> wins via the outer div.
	innerDir := attrValueOrDefault(n, "dir", "auto")
	innerAttrs := []xhtml.Attribute{{Key: "dir", Val: innerDir}}
	if !hasInlineStyleProp(getStyleAttr(n), "font-size") {
		innerAttrs = append(innerAttrs, xhtml.Attribute{Key: "style", Val: "font-size:14px"})
	}
	inner := &xhtml.Node{
		Type:     xhtml.ElementNode,
		Data:     "div",
		DataAtom: atom.Div,
		Attr:     innerAttrs,
	}
	for _, c := range children {
		inner.AppendChild(c)
	}
	n.AppendChild(inner)

	rep.Applied = append(rep.Applied, Finding{
		RuleID:    RuleStyleParaWrapper,
		Severity:  SeverityWarning,
		TagOrAttr: "p",
		Excerpt:   excerptOf(n),
		Hint:      "Rewritten as double-wrapped div paragraph (outer margin/line-height + additive inner dir/font-size)",
	})
}

func attrValueOrDefault(n *xhtml.Node, key, fallback string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) && strings.TrimSpace(attr.Val) != "" {
			return attr.Val
		}
	}
	return fallback
}
