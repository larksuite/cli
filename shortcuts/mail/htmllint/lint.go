// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package htmllint

import (
	"bytes"
	"fmt"
	"strings"

	xhtml "golang.org/x/net/html"
)

type Finding struct {
	RuleID    string `json:"rule_id"`
	Severity  string `json:"severity"`
	TagOrAttr string `json:"tag_or_attr"`
	Excerpt   string `json:"excerpt,omitempty"`
	Hint      string `json:"hint"`
}

type Result struct {
	Warnings    []Finding `json:"warnings"`
	Errors      []Finding `json:"errors"`
	CleanedHTML string    `json:"cleaned_html,omitempty"`
}

func (r Result) HasFindings() bool {
	return len(r.Warnings) > 0 || len(r.Errors) > 0
}

func Lint(raw string, autoFix bool) (Result, error) {
	result := Result{}
	nodes, err := xhtml.ParseFragment(strings.NewReader(raw), &xhtml.Node{Type: xhtml.ElementNode, Data: "body"})
	if err != nil {
		result.Errors = append(result.Errors, Finding{
			RuleID: "HTML_PARSE_FAILED", Severity: "error", TagOrAttr: "html",
			Excerpt: truncate(raw), Hint: "HTML cannot be parsed reliably",
		})
		if autoFix {
			result.CleanedHTML = raw
		}
		return result, nil
	}
	var out bytes.Buffer
	for _, n := range nodes {
		renderNode(&out, n, &result, autoFix)
	}
	if autoFix {
		result.CleanedHTML = out.String()
	}
	return result, nil
}

func renderNode(out *bytes.Buffer, n *xhtml.Node, result *Result, autoFix bool) {
	if n == nil {
		return
	}
	switch n.Type {
	case xhtml.TextNode:
		_ = xhtml.Render(out, n)
	case xhtml.CommentNode, xhtml.DoctypeNode:
		return
	case xhtml.ElementNode:
		tag := strings.ToLower(n.Data)
		if blockedTags[tag] {
			result.Errors = append(result.Errors, Finding{
				RuleID: "TAG_BLOCKED", Severity: "error", TagOrAttr: tag,
				Excerpt: "<" + tag + ">", Hint: "removed before writing mail HTML",
			})
			return
		}
		convertedCenter := false
		if tag == "font" {
			result.Warnings = append(result.Warnings, Finding{
				RuleID: "TAG_FONT_TO_SPAN", Severity: "warning", TagOrAttr: "font",
				Excerpt: "<font>", Hint: "converted to <span>",
			})
			tag = "span"
		} else if tag == "center" {
			result.Warnings = append(result.Warnings, Finding{
				RuleID: "TAG_CENTER_TO_DIV", Severity: "warning", TagOrAttr: "center",
				Excerpt: "<center>", Hint: "converted to <div style=\"text-align:center\">",
			})
			tag = "div"
			convertedCenter = true
		} else if tag == "marquee" || tag == "blink" {
			result.Warnings = append(result.Warnings, Finding{
				RuleID: "TAG_DEPRECATED_TO_TEXT", Severity: "warning", TagOrAttr: tag,
				Excerpt: "<" + tag + ">", Hint: "kept inner text and removed decorative tag",
			})
			renderChildren(out, n, result, autoFix)
			return
		}
		attrs := cleanAttrs(n.Attr, result, convertedCenter)
		out.WriteByte('<')
		out.WriteString(tag)
		for _, a := range attrs {
			out.WriteByte(' ')
			out.WriteString(a.Key)
			out.WriteString(`="`)
			out.WriteString(xhtml.EscapeString(a.Val))
			out.WriteByte('"')
		}
		out.WriteByte('>')
		renderChildren(out, n, result, autoFix)
		out.WriteString("</")
		out.WriteString(tag)
		out.WriteByte('>')
	default:
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(out, c, result, autoFix)
		}
	}
}

func renderChildren(out *bytes.Buffer, n *xhtml.Node, result *Result, autoFix bool) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(out, c, result, autoFix)
	}
}

func cleanAttrs(attrs []xhtml.Attribute, result *Result, convertedCenter bool) []xhtml.Attribute {
	cleaned := make([]xhtml.Attribute, 0, len(attrs))
	needsCenterStyle := convertedCenter
	for _, a := range attrs {
		key := strings.ToLower(strings.TrimSpace(a.Key))
		val := strings.TrimSpace(a.Val)
		if key == "" {
			continue
		}
		if strings.HasPrefix(key, "on") {
			result.Errors = append(result.Errors, Finding{
				RuleID: "ATTR_EVENT_BLOCKED", Severity: "error", TagOrAttr: key,
				Excerpt: fmt.Sprintf("%s=%q", key, truncate(val)), Hint: "removed event handler attribute",
			})
			continue
		}
		if (key == "href" || key == "src") && unsafeURL(val) {
			result.Errors = append(result.Errors, Finding{
				RuleID: "URL_SCHEME_BLOCKED", Severity: "error", TagOrAttr: key,
				Excerpt: fmt.Sprintf("%s=%q", key, truncate(val)), Hint: "removed unsafe URL scheme",
			})
			continue
		}
		if key == "style" {
			style := cleanStyle(val, result)
			if needsCenterStyle {
				style = mergeStyle("text-align:center", style)
				needsCenterStyle = false
			}
			if style == "" {
				continue
			}
			val = style
		}
		cleaned = append(cleaned, xhtml.Attribute{Key: key, Val: val})
	}
	if needsCenterStyle {
		cleaned = append(cleaned, xhtml.Attribute{Key: "style", Val: "text-align:center"})
	}
	return cleaned
}

func cleanStyle(raw string, result *Result) string {
	parts := strings.Split(raw, ";")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		name, val, ok := strings.Cut(p, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		val = strings.TrimSpace(val)
		if !allowedCSS[name] {
			result.Warnings = append(result.Warnings, Finding{
				RuleID: "CSS_PROPERTY_REMOVED", Severity: "warning", TagOrAttr: name,
				Excerpt: truncate(p), Hint: "removed unsupported inline style property",
			})
			continue
		}
		if unsafeURL(val) {
			result.Errors = append(result.Errors, Finding{
				RuleID: "CSS_URL_BLOCKED", Severity: "error", TagOrAttr: name,
				Excerpt: truncate(p), Hint: "removed unsafe CSS URL",
			})
			continue
		}
		kept = append(kept, name+":"+val)
	}
	return strings.Join(kept, ";")
}

func unsafeURL(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	return strings.HasPrefix(s, "javascript:") ||
		strings.HasPrefix(s, "vbscript:") ||
		strings.Contains(s, "javascript:") ||
		strings.Contains(s, "vbscript:")
}

func mergeStyle(first, second string) string {
	first = strings.TrimSpace(strings.TrimSuffix(first, ";"))
	second = strings.TrimSpace(strings.Trim(second, ";"))
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + ";" + second
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) <= 80 {
		return s
	}
	r := []rune(s)
	return string(r[:80]) + "..."
}

var blockedTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"form": true, "input": true, "link": true, "meta": true, "base": true,
}

var allowedCSS = map[string]bool{
	"color": true, "background-color": true, "font-size": true, "font-weight": true,
	"font-style": true, "text-align": true, "text-decoration": true, "line-height": true,
	"padding": true, "margin": true, "border": true, "border-top": true,
	"border-right": true, "border-bottom": true, "border-left": true, "width": true,
	"height": true, "display": true, "text-indent": true,
}
