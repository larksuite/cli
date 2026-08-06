// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"encoding/json"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Path is the content-read route (materialized Markdown, or for
// docx the XML-with-block-id payload). Exported so drive/docs dry-run can report
// the POST target without re-typing the literal.
const Path = "/open-apis/search/v2/knowledge_qa/fetch_doc_info"

// FetchDocInfo posts the fetch request and decodes the response. Transport
// failures and non-zero API codes come back already typed from CallAPITyped.
//
// It does NOT call EnsureScopes: the doc-read scope is declared on each entry
// point's Shortcut.Scopes and enforced by the pre-flight. Conditional scopes
// (Wiki unwrap, Minutes note documents) are ensured by the dispatch path that
// needs them.
func FetchDocInfo(ctx context.Context, runtime *common.RuntimeContext, req Request) (*Response, error) {
	_ = ctx
	data, err := runtime.CallAPITyped("POST", Path, nil, req)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "marshal fetch data: %s", err).WithCause(err)
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "decode fetch response: %s", err).WithCause(err)
	}
	return &resp, nil
}
