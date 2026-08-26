// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/xml"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// v2CreateFlags returns the flag definitions for the v2 (OpenAPI) create path.
func v2CreateFlags() []common.Flag {
	return []common.Flag{
		{Name: "title", Desc: "document title; when provided, the CLI prepends it to --content as <title>...</title> so the title wins over later content titles"},
		{Name: "content", Desc: docsCreateContentFlagBase, Input: []string{common.File, common.Stdin}},
		{Name: "reference-map", Desc: docsReferenceMapFlagDesc, Input: []string{common.File, common.Stdin}},
		{Name: "doc-format", Desc: "content format; xml is default and supports richer DocxXML blocks, markdown imports plain Markdown", Default: "xml", Enum: []string{"xml", "markdown"}},
		{Name: "parent-token", Desc: "parent folder token or wiki node token; mutually exclusive with --parent-position"},
		{Name: "parent-position", Desc: "parent position such as my_library; mutually exclusive with --parent-token"},
	}
}

func validateCreateV2(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateDocsV2Only(runtime, "+create", docsCreateLegacyFlags()); err != nil {
		return err
	}
	title := strings.TrimSpace(runtime.Str("title"))
	if runtime.Changed("title") && title == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--title must not be empty").WithParam("--title")
	}
	if err := validateDocsV2ReferenceMapFlags(runtime); err != nil {
		return err
	}
	if runtime.Str("parent-token") != "" && runtime.Str("parent-position") != "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--parent-token and --parent-position are mutually exclusive").WithParams(
			errs.InvalidParam{Name: "--parent-token", Reason: "mutually exclusive with --parent-position"},
			errs.InvalidParam{Name: "--parent-position", Reason: "mutually exclusive with --parent-token"},
		)
	}
	content := runtime.Str("content")
	if err := validateDocsWriteContentEncoding(content); err != nil {
		return err
	}
	if strings.TrimSpace(content) == "" && title == "" {
		if path, ok := runtime.Cmd.Annotations[docsContentPathAnnotation]; ok {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--content file %q is empty", path).
				WithParam("--content").
				WithHint("write non-empty XML or Markdown to this file; if the path was reserved by init-draft, use the exact data.draft_path returned by that command, then retry with --content \"@./<data.draft_path>\"")
		}
		if runtime.Changed("content") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--content was provided but is empty; an @file input may point to an empty draft").
				WithParam("--content").
				WithHint("write non-empty XML or Markdown to the draft and retry with --content \"@./<data.draft_path>\", or omit --content and pass --title to create a title-only document")
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--content is required unless --title is provided").
			WithParam("--content").
			WithHint("provide XML or Markdown directly, use --content \"@relative/path\", or pass --title to create a title-only document")
	}
	if content != "" {
		input, err := resolveDocsV2ContentReferenceMap(runtime)
		if err != nil {
			return err
		}
		if len(input.LocalResources) > 0 {
			return runtime.EnsureScopes(docsCreateLocalResourceScopes)
		}
	}
	return nil
}

func dryRunCreateV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	body, resources, err := buildCreateBodyWithPreparedInput(runtime)
	if err != nil {
		return common.NewDryRunAPI().Set("error", err.Error())
	}
	desc := "OpenAPI: create document"
	if runtime.IsBot() {
		desc += ". After document creation succeeds in bot mode, the CLI will also try to grant the current CLI user full_access on the new document."
	}
	dry := common.NewDryRunAPI().
		POST("/open-apis/docs_ai/v1/documents").
		Desc(desc).
		Body(body)
	dry = appendRemoteDocImageDownloadsDryRun(dry, resources)
	return appendLocalDocResourcesDryRun(dry, "<created_document_id>", resources)
}

func executeCreateV2(_ context.Context, runtime *common.RuntimeContext) error {
	body, resources, err := buildCreateBodyWithPreparedInput(runtime)
	if err != nil {
		return err
	}
	if err := validateRemoteDocImageSources(runtime.Ctx(), resources); err != nil {
		return err
	}

	data, err := doDocAPI(runtime, "POST", "/open-apis/docs_ai/v1/documents", body)
	if err != nil {
		return err
	}
	if docsAPIOperationFailed(data) {
		return runtime.OutPartialFailure(data, nil)
	}

	augmentDocsCreatePermission(runtime, data)
	fallbackDocsCreateURLV2(runtime, data)
	if len(resources) > 0 {
		doc, _ := data["document"].(map[string]interface{})
		if err := finalizeLocalDocResources(runtime, strings.TrimSpace(common.GetString(doc, "document_id")), data, resources); err != nil {
			return err
		}
	}
	runtime.OutRaw(data, nil)
	return nil
}

func buildCreateBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"format":  runtime.Str("doc-format"),
		"content": buildCreateContent(runtime),
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

func buildCreateContent(runtime *common.RuntimeContext) string {
	return buildCreateContentWithBody(runtime, runtime.Str("content"))
}

func buildCreateContentWithBody(runtime *common.RuntimeContext, content string) string {
	title := strings.TrimSpace(runtime.Str("title"))
	if title == "" {
		return content
	}

	titleTag := "<title>" + escapeDocTitleText(title) + "</title>"
	if content == "" {
		return titleTag
	}
	return titleTag + "\n" + content
}

func escapeDocTitleText(title string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(title))
	return buf.String()
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
