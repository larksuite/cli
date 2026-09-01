// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/xml"
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

// docsFetchCitations builds the fetched document's citation from the final
// response payload. The URL is server-resolved because it may carry tenant and
// geo routing that cannot be reconstructed safely from the input token. The
// fetch extra_param opts into that field only while citation output is enabled.
func docsFetchCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	document, ok := out["document"].(map[string]interface{})
	if !ok {
		return nil
	}
	return []citation.Citation{{
		SourceType: citation.SourceDoc,
		URL:        common.GetString(document, "url"),
		Title:      docsFetchCitationTitle(common.GetString(document, "content")),
	}}
}

func docsFetchCitationTitle(content string) string {
	decoder := xml.NewDecoder(strings.NewReader(content))
	for {
		token, err := decoder.Token()
		if err != nil {
			return ""
		}
		start, ok := token.(xml.StartElement)
		if !ok || !strings.EqualFold(start.Name.Local, "title") {
			continue
		}

		var title strings.Builder
		depth := 1
		for depth > 0 {
			token, err = decoder.Token()
			if err != nil {
				return ""
			}
			switch current := token.(type) {
			case xml.StartElement:
				depth++
			case xml.EndElement:
				depth--
			case xml.CharData:
				title.Write(current)
			}
		}
		return strings.Join(strings.Fields(title.String()), " ")
	}
}
