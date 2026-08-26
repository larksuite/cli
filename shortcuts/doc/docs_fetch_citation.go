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
		Title:      common.GetString(document, "title"),
		ResourceID: common.GetString(document, "document_id"),
	}}
}
