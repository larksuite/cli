// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"

	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

type workbookCitationMetadata struct {
	Title        string
	URL          string
	ModifiedTime any
}

func readWorkbookCitationMetadata(data any) workbookCitationMetadata {
	var meta workbookCitationMetadata
	if out, ok := data.(map[string]interface{}); ok {
		meta.Title, _ = out["title"].(string)
		meta.URL, _ = out["url"].(string)
		meta.ModifiedTime = out["modified_time"]
	}
	return meta
}

// workbookCitations uses the same workbook metadata for structure and CSV
// reads. Prefer a native response URL, then the caller's URL; token-only
// invocations without a native URL degrade to no citation via Normalize.
func workbookCitations(rt *common.RuntimeContext, data any) []citation.Citation {
	token, _ := resolveSpreadsheetToken(rt)
	if token == "" {
		return nil
	}
	meta := readWorkbookCitationMetadata(data)
	url := meta.URL
	if url == "" {
		url = strings.TrimSpace(rt.Str("url"))
	}
	return []citation.Citation{{
		SourceType:  citation.SourceSheet,
		URL:         url,
		Title:       meta.Title,
		PublishTime: citation.Time(meta.ModifiedTime),
	}}
}
