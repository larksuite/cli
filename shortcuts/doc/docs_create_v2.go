// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// v2CreateFlags returns the flag definitions for the v2 (OpenAPI) create path.
func v2CreateFlags() []common.Flag {
	return []common.Flag{
		{Name: "content", Desc: "document content (XML or Markdown)", Hidden: true, Input: []string{common.File, common.Stdin}},
		{Name: "doc-format", Desc: "content format (prefer XML)", Hidden: true, Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "parent-token", Desc: "parent folder or wiki-node token", Hidden: true},
		{Name: "parent-position", Desc: "parent position (e.g. my_library)", Hidden: true},
	}
}

func validateCreateV2(_ context.Context, runtime *common.RuntimeContext) error {
	if runtime.Str("content") == "" {
		return common.FlagErrorf("--content is required")
	}
	if runtime.Str("parent-token") != "" && runtime.Str("parent-position") != "" {
		return common.FlagErrorf("--parent-token and --parent-position are mutually exclusive")
	}
	return nil
}

func dryRunCreateV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body := buildCreateBody(runtime)
	desc := "OpenAPI: create document"
	if runtime.IsBot() {
		desc += ". After document creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new document."
	}
	return common.NewDryRunAPI().
		POST("/open-apis/docs_ai/v1/documents").
		Desc(desc).
		Body(body)
}

func executeCreateV2(_ context.Context, runtime *common.RuntimeContext) error {
	body := buildCreateBody(runtime)

	data, err := doDocAPI(runtime, "POST", "/open-apis/docs_ai/v1/documents", body)
	if err != nil {
		return err
	}

	augmentDocsCreatePermission(runtime, data)
	fallbackDocsCreateURLV2(runtime, data)
	runtime.OutRaw(data, nil)
	return nil
}

func buildCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"format":  runtime.Str("doc-format"),
		"content": runtime.Str("content"),
	}
	if v := runtime.Str("parent-token"); v != "" {
		body["parent_token"] = v
	}
	if v := runtime.Str("parent-position"); v != "" {
		body["parent_position"] = v
	}
	injectDocsScene(runtime, body)
	return body
}

// augmentDocsCreatePermission grants full_access to the current CLI user when
// the document was created with bot identity.
func augmentDocsCreatePermission(runtime *common.RuntimeContext, data map[string]interface{}) {
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return
	}
	docID := strings.TrimSpace(common.GetString(doc, "document_id"))
	if docID == "" {
		return
	}
	if grant := common.AutoGrantCurrentUserDrivePermission(runtime, docID, "docx"); grant != nil {
		data["permission_grant"] = grant
	}
}

// fallbackDocsCreateURLV2 fills data.document.url with a brand-standard URL
// when the OpenAPI response did not include one. Backfills only when missing,
// so any tenant-specific URL the backend returned is preserved.
func fallbackDocsCreateURLV2(runtime *common.RuntimeContext, data map[string]interface{}) {
	doc, _ := data["document"].(map[string]interface{})
	if doc == nil {
		return
	}
	if strings.TrimSpace(common.GetString(doc, "url")) != "" {
		return
	}
	docID := strings.TrimSpace(common.GetString(doc, "document_id"))
	if docID == "" {
		return
	}
	if u := common.BuildResourceURL(runtime.Config.Brand, "docx", docID); u != "" {
		doc["url"] = u
	}
}
