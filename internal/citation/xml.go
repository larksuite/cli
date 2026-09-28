// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"encoding/xml"
	"fmt"
	"strings"
)

// EncodeXML renders each citation as an XML <document> string and returns nil
// for an empty input so the envelope omits the citations key entirely.
//
// Child order (title, source_type, snippet, url, publish_time) follows the
// consumer's format; snippet and publish_time are omitted when empty, title
// and source_type are always present. The reference_id attribute duplicates
// the entry's URL — the consumer keys references by URL.
//
// URL fields (reference_id and <url>) are emitted RAW, without XML escaping.
// The consumer's render pipeline matches the model's RichMediaRef url against
// this text by exact string comparison without XML-unescaping first, so a
// standards-escaped "&amp;" in a query string breaks the match and the
// reference silently fails to render (2026-08-28 joint-debugging finding;
// the same de-facto contract as the platform's existing search tools). Text
// fields (title, snippet, publish_time) keep standard XML escaping.
func EncodeXML(items []Citation) []string {
	var encoded []string
	for _, item := range items {
		var b strings.Builder
		b.WriteString(`<document reference_id="`)
		b.WriteString(item.URL)
		b.WriteString(`">`)
		b.WriteString("<title>")
		xmlEscape(&b, item.Title)
		b.WriteString("</title>")
		fmt.Fprintf(&b, "<source_type>%d</source_type>", item.SourceType)
		if item.Snippet != "" {
			b.WriteString("<snippet>")
			xmlEscape(&b, item.Snippet)
			b.WriteString("</snippet>")
		}
		b.WriteString("<url>")
		b.WriteString(item.URL)
		b.WriteString("</url>")
		if item.PublishTime != "" {
			b.WriteString("<publish_time>")
			xmlEscape(&b, item.PublishTime)
			b.WriteString("</publish_time>")
		}
		b.WriteString("</document>")
		encoded = append(encoded, b.String())
	}
	return encoded
}

// xmlEscape writes s with standard XML escaping. xml.EscapeText cannot fail
// on a strings.Builder (its writer never errors), so the error is ignored.
func xmlEscape(b *strings.Builder, s string) {
	_ = xml.EscapeText(b, []byte(s))
}
