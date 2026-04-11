// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var DocsCreate = common.Shortcut{
	Service:     "docs",
	Command:     "+create",
	Description: "Create a Lark document",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"docx:document:create"},
	Flags: []common.Flag{
		{Name: "content", Desc: "document content (XML or Markdown)", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "doc-format", Desc: "content format（prefer XML）", Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "parent-token", Desc: "parent folder or wiki-node token"},
		{Name: "parent-position", Desc: "parent position (e.g. my_library)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Str("parent-token") != "" && runtime.Str("parent-position") != "" {
			return common.FlagErrorf("--parent-token and --parent-position are mutually exclusive")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := buildCreateBody(runtime)
		d := common.NewDryRunAPI().
			POST("/open-apis/docs_ai/v1/documents").
			Desc("OpenAPI: create document").
			Body(body)
		if runtime.IsBot() {
			d.Desc("After document creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new document.")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := buildCreateBody(runtime)

		data, err := doDocAPI(runtime, "POST", "/open-apis/docs_ai/v1/documents", body)
		if err != nil {
			return err
		}

		stripBlockIDs(data)
		augmentDocsCreatePermission(runtime, data)
		runtime.OutRaw(data, nil)
		return nil
	},
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
