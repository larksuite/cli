// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import "encoding/xml"

// xmlDocument is the wire form of one citation: an XML <document> element,
// serialized to a string and carried in the envelope's citations array. The
// reference_id attribute duplicates the entry's URL — the consumer keys
// references by URL. Child order (title, source_type, snippet, url,
// publish_time) follows the consumer's format; snippet and publish_time are
// omitted when empty, title and source_type are always present.
type xmlDocument struct {
	XMLName     xml.Name   `xml:"document"`
	ReferenceID string     `xml:"reference_id,attr"`
	Title       string     `xml:"title"`
	SourceType  SourceType `xml:"source_type"`
	Snippet     string     `xml:"snippet,omitempty"`
	URL         string     `xml:"url"`
	PublishTime string     `xml:"publish_time,omitempty"`
}

// EncodeXML renders each citation as an XML <document> string with standard
// XML escaping. It returns nil for an empty input so the envelope omits the
// citations key entirely. Encoding failure for one entry drops that entry
// rather than failing the caller: citations are strictly additive.
func EncodeXML(items []Citation) []string {
	var encoded []string
	for _, item := range items {
		b, err := xml.Marshal(xmlDocument{
			ReferenceID: item.URL,
			Title:       item.Title,
			SourceType:  item.SourceType,
			Snippet:     item.Snippet,
			URL:         item.URL,
			PublishTime: item.PublishTime,
		})
		if err != nil {
			continue
		}
		encoded = append(encoded, string(b))
	}
	return encoded
}
