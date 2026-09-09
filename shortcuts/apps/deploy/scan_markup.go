// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// parseError marks a file that could not be understood. It does not stop the
// publish: the file still ships, it just is not searched for further
// references, exactly as the web client behaves.
type parseError struct{ code string }

func (e *parseError) Error() string { return "parse failed: " + e.code }

// markupRefAttrs lists the attributes that carry a subresource the document
// needs in order to render. Navigation attributes (a[href], form[action]) are
// deliberately absent: following them would pull an entire site in behind one
// page, which is what --dir is for.
//
// Matching is by local attribute name, so `xlink:href` and a plain `href` on
// <use> and <image> both hit the same entry.
var markupRefAttrs = map[string][]string{
	"audio":  {"src"},
	"embed":  {"src"},
	"iframe": {"src"},
	"img":    {"src", "srcset"},
	"image":  {"href"},
	"object": {"data"},
	"source": {"src", "srcset"},
	"track":  {"src"},
	"use":    {"href"},
	"video":  {"src", "poster"},
}

// resourceLinkRels are the only <link> relations whose href names a file the
// page needs. rel="canonical" and friends point at documents, not resources.
var resourceLinkRels = map[string]bool{
	"icon": true, "manifest": true, "modulepreload": true, "preload": true, "stylesheet": true,
}

// inlineScriptTypes are the script types whose body is JavaScript. An empty
// type means JavaScript too; anything else (importmap, application/json, a
// template language) is data and must not be parsed as code.
var inlineScriptTypes = map[string]bool{
	"module": true, "text/javascript": true, "application/javascript": true,
}

// scanHTML collects the references written in an HTML document.
func scanHTML(raw []byte) ([]string, int, error) {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		// x/net/html implements the HTML5 recovery rules and does not reject
		// documents, so this only fires on a read error.
		return nil, 0, &parseError{code: "invalid_html"}
	}
	// A <base href> re-points every relative reference in the document at a
	// location the payload has no way to model, so the file is left unexpanded
	// rather than expanded against the wrong directory.
	if findBaseHref(doc) {
		return nil, 0, &parseError{code: "unsupported_base"}
	}
	var refs []string
	unsupported := 0
	walkHTML(doc, func(n *html.Node) {
		r, u := elementRefs(n.Data, htmlAttrs(n), textOf(n))
		refs = append(refs, r...)
		unsupported += u
	})
	return refs, unsupported, nil
}

// scanSVG collects the references written in an SVG document. SVG is parsed as
// XML, so unlike HTML it can be rejected -- an undeclared namespace prefix or a
// stray tag makes the file unreadable to a browser too.
func scanSVG(raw []byte) ([]string, int, error) {
	els, err := parseXMLElements(raw)
	if err != nil {
		return nil, 0, &parseError{code: "invalid_html"}
	}
	var refs []string
	unsupported := 0
	for _, el := range els {
		r, u := elementRefs(el.name, el.attrs, el.text)
		refs = append(refs, r...)
		unsupported += u
	}
	return refs, unsupported, nil
}

// attrPair is one attribute reduced to the form both parsers agree on: the
// local name lowercased, and the raw value.
type attrPair struct{ name, value string }

// elementRefs applies the subresource rules to one element.
func elementRefs(tag string, attrs []attrPair, text string) ([]string, int) {
	tag = strings.ToLower(tag)
	var refs []string
	unsupported := 0

	for _, want := range markupRefAttrs[tag] {
		if v := attrValue(attrs, want); v != "" {
			if want == "srcset" {
				refs = append(refs, splitSrcset(v)...)
			} else {
				refs = append(refs, v)
			}
		}
	}
	if tag == "input" && strings.EqualFold(attrValue(attrs, "type"), "image") {
		if v := attrValue(attrs, "src"); v != "" {
			refs = append(refs, v)
		}
	}
	if tag == "link" {
		if v := attrValue(attrs, "href"); v != "" && hasResourceRel(attrValue(attrs, "rel")) {
			refs = append(refs, v)
		}
	}
	if tag == "script" {
		if v := attrValue(attrs, "src"); v != "" {
			refs = append(refs, v)
		} else if inlineScriptTypes[strings.ToLower(strings.TrimSpace(attrValue(attrs, "type")))] ||
			strings.TrimSpace(attrValue(attrs, "type")) == "" {
			r, u, err := scanJS([]byte(text))
			if err == nil {
				refs = append(refs, r...)
				unsupported += u
			}
		}
	}
	if tag == "style" {
		refs = append(refs, scanCSSIgnoringErrors([]byte(text), false)...)
	}
	// A style attribute holds a declaration list rather than a full stylesheet.
	if v := attrValue(attrs, "style"); v != "" {
		refs = append(refs, scanCSSIgnoringErrors([]byte(v), true)...)
	}
	return refs, unsupported
}

func attrValue(attrs []attrPair, name string) string {
	for _, a := range attrs {
		if a.name == name {
			return a.value
		}
	}
	return ""
}

func hasResourceRel(rel string) bool {
	for _, token := range strings.Fields(strings.ToLower(rel)) {
		if resourceLinkRels[token] {
			return true
		}
	}
	return false
}

// splitSrcset pulls the URLs out of a candidate list such as
// "a.png 1x, b@2x.png 2x".
//
// A list mentioning data: anywhere yields nothing at all, rather than the
// candidates around it. Splitting a data: URI on commas produces fragments that
// are not paths, and the web client takes the same all-or-nothing route, so a
// payload must not disagree about which images it contains.
func splitSrcset(v string) []string {
	if dataURIInSrcsetRe.MatchString(v) {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if i := strings.IndexAny(part, " \t\n\r\f"); i >= 0 {
			part = part[:i]
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// ── HTML tree helpers ────────────────────────────────────────────────────

func htmlAttrs(n *html.Node) []attrPair {
	out := make([]attrPair, 0, len(n.Attr))
	for _, a := range n.Attr {
		out = append(out, attrPair{name: strings.ToLower(a.Key), value: a.Val})
	}
	return out
}

// textOf returns the raw text directly inside an element, which is all that
// <style> and an inline <script> can hold.
func textOf(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
	}
	return b.String()
}

func walkHTML(n *html.Node, visit func(*html.Node)) {
	if n.Type == html.ElementNode {
		visit(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, visit)
	}
}

func findBaseHref(n *html.Node) bool {
	found := false
	walkHTML(n, func(el *html.Node) {
		if !found && strings.EqualFold(el.Data, "base") && attrValue(htmlAttrs(el), "href") != "" {
			found = true
		}
	})
	return found
}

// ── XML (SVG) ────────────────────────────────────────────────────────────

type xmlElement struct {
	name  string
	attrs []attrPair
	text  string
}

// parseXMLElements decodes an XML document into a flat element list, rejecting
// the documents a browser's XML parser rejects. Go's decoder is lenient about a
// namespace prefix that was never declared, so that check is made here: a
// browser reports it as a parse error, and the two sides have to agree on which
// files expand.
func parseXMLElements(raw []byte) ([]xmlElement, error) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	// Prefixes declared by enclosing elements, innermost last.
	declared := map[string]bool{"xml": true, "xmlns": true}
	var els []xmlElement
	var stack []*xmlElement

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			for _, a := range t.Attr {
				// Go reports xmlns declarations with Space "xmlns" (prefixed)
				// or Local "xmlns" (default namespace); either way the value is
				// the namespace URI, which is what a resolved name carries.
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					declared[a.Value] = true
				}
			}
			if err := checkPrefix(t.Name.Space, declared); err != nil {
				return nil, err
			}
			el := xmlElement{name: t.Name.Local}
			for _, a := range t.Attr {
				if err := checkPrefix(a.Name.Space, declared); err != nil {
					return nil, err
				}
				el.attrs = append(el.attrs, attrPair{name: strings.ToLower(a.Name.Local), value: a.Value})
			}
			els = append(els, el)
			stack = append(stack, &els[len(els)-1])
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text += string(t)
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return els, nil
}

// checkPrefix rejects a name whose namespace was never declared. A resolved
// name carries the declared URI; an unresolved one carries the raw prefix,
// which will not be in the set.
func checkPrefix(space string, declared map[string]bool) error {
	if space == "" || declared[space] {
		return nil
	}
	return &parseError{code: "invalid_html"}
}
