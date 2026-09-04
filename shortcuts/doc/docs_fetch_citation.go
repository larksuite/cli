// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"github.com/larksuite/cli/internal/citation"
	"github.com/larksuite/cli/shortcuts/common"
)

// docsFetchCitations builds the fetched document's citation from the final
// response payload. The URL is server-resolved because it may carry tenant and
// geo routing that cannot be reconstructed safely from the input token. The
// title is supplied by the server for the fetched revision, independently of
// content format and read scope. The fetch extra_param opts into both fields
// only while citation output is enabled. Missing titles stay empty; content is
// not a reliable title source for Markdown or partial reads.
func docsFetchCitations(_ *common.RuntimeContext, data any) []citation.Citation {
	out, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	fields, ok := out["document"].(map[string]interface{})
	if !ok {
		return nil
	}
	var document struct {
		URL   string
		Title string
	}
	document.URL, _ = fields["url"].(string)
	document.Title, _ = fields["title"].(string)
	return []citation.Citation{{
		SourceType: citation.SourceDoc,
		URL:        document.URL,
		Title:      document.Title,
	}}
}
