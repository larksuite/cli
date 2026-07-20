// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const docsFetchExtraParam = `{"enable_user_cite_reference_map":true,"return_html5_block_data":true}`

// v2FetchFlags returns the flag definitions for the v2 (OpenAPI) fetch path.
func v2FetchFlags() []common.Flag {
	return []common.Flag{
		{Name: "doc-format", Desc: "output content format; xml keeps DocxXML structure and optional block ids, markdown is plain export, im-markdown downgrades residual DocxXML fragments for IM messages", Default: "xml", Enum: []string{"xml", "markdown", "im-markdown"}},
		{Name: "detail", Desc: "detail level; simple for reading, with-ids for block references, full for styles and edit metadata", Default: "simple", Enum: []string{"simple", "with-ids", "full"}},
		{Name: "lang", Desc: "user cite display language, e.g. en-US, zh-CN, ja-JP"},
		{Name: "revision-id", Desc: "document revision id; -1 means latest", Type: "int", Default: "-1"},
		{Name: "scope", Desc: "read scope; full reads whole doc, outline lists headings, section expands from heading anchor, range uses block ids, keyword searches text", Default: "full", Enum: []string{"full", "outline", "range", "keyword", "section"}},
		{Name: "start-block-id", Desc: "range/section anchor block id; required for section and optional start for range"},
		{Name: "end-block-id", Desc: "range end block id; -1 means through document end"},
		{Name: "keyword", Desc: "keyword scope query; supports case-insensitive substring/regex fallback and '|' OR branches, e.g. foo|bar or bug|error"},
		{Name: "context-before", Desc: "range/keyword/section context: sibling blocks before selected top-level blocks", Type: "int", Default: "0"},
		{Name: "context-after", Desc: "range/keyword/section context: sibling blocks after selected top-level blocks", Type: "int", Default: "0"},
		{Name: "max-depth", Desc: "outline heading level cap; other scopes subtree depth where -1 is unlimited and 0 is block only", Type: "int", Default: "-1"},
	}
}

// validateFetchV2 is the Validate hook for the v2 fetch path. It runs before
// --dry-run so that invalid input fails with a structured exit code (2) and
// JSON envelope instead of slipping through dry-run as a "success".
func validateFetchV2(_ context.Context, runtime *common.RuntimeContext) error {
	if err := validateDocsV2Only(runtime, "+fetch", docsFetchLegacyFlags()); err != nil {
		return err
	}
	if _, err := parseDocumentRef(runtime.Str("doc")); err != nil {
		return err
	}
	if err := validateReadModeFlags(runtime); err != nil {
		return err
	}
	return nil
}

func dryRunFetchV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	// Validate has already accepted --doc; parseDocumentRef cannot fail here.
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	body := buildFetchBody(runtime)
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	return common.NewDryRunAPI().
		POST(apiPath).
		Desc("OpenAPI: fetch document").
		Body(body).
		Set("document_id", ref.Token)
}

func executeFetchV2(_ context.Context, runtime *common.RuntimeContext) error {
	ref, _ := parseDocumentRef(runtime.Str("doc"))

	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	body := buildFetchBody(runtime)

	data, err := doDocAPI(runtime, "POST", apiPath, body)
	if err != nil {
		return err
	}
	if err := processHTML5BlockReferenceMapForFetch(runtime, effectiveFetchFormat(runtime), ref.Token, data); err != nil {
		return err
	}
	if warning := addFetchDetailDowngradeWarning(runtime, data); warning != "" && runtime.Format == "pretty" {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", warning)
	}
	if isIMMarkdownFetch(runtime) {
		applyFetchIMMarkdown(data, runtime.Str("doc"))
	}

	runtime.OutFormatRaw(data, nil, func(w io.Writer) {
		if doc, ok := data["document"].(map[string]interface{}); ok {
			if content, ok := doc["content"].(string); ok {
				fmt.Fprintln(w, content)
			}
		}
	})
	return nil
}

func buildFetchBody(runtime *common.RuntimeContext) map[string]interface{} {
	body := map[string]interface{}{
		"format":      effectiveFetchFormat(runtime),
		"extra_param": docsFetchExtraParam,
	}
	if v := runtime.Int("revision-id"); v > 0 {
		body["revision_id"] = v
	}
	if lang := resolveFetchLang(runtime); lang != "" {
		body["lang"] = lang
	}

	detail := effectiveFetchDetail(runtime)
	switch detail {
	case "", "simple":
		body["export_option"] = map[string]interface{}{
			"export_block_id":        false,
			"export_style_attrs":     false,
			"export_cite_extra_data": false,
		}
	case "with-ids":
		body["export_option"] = map[string]interface{}{
			"export_block_id": true,
		}
	case "full":
		body["export_option"] = map[string]interface{}{
			"export_block_id":        true,
			"export_style_attrs":     true,
			"export_cite_extra_data": true,
		}
	}

	if ro := buildReadOption(runtime); ro != nil {
		body["read_option"] = ro
	}
	injectDocsScene(runtime, body)

	return body
}

func effectiveFetchFormat(runtime *common.RuntimeContext) string {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	if format == "im-markdown" {
		return "markdown"
	}
	return format
}

func resolveFetchLang(runtime *common.RuntimeContext) string {
	if runtime.Changed("lang") {
		return strings.TrimSpace(runtime.Str("lang"))
	}
	if runtime.Config == nil {
		return ""
	}
	return strings.TrimSpace(string(runtime.Config.Lang))
}

// buildReadOption 拼装 read_option JSON；full/空模式返回 nil，让服务端走默认全文路径。
func buildReadOption(runtime *common.RuntimeContext) map[string]interface{} {
	mode := effectiveFetchReadMode(runtime)
	if mode == "" || mode == "full" {
		return nil
	}
	ro := map[string]interface{}{"read_mode": mode}
	if v := effectiveFetchStartBlockID(runtime, mode); v != "" {
		ro["start_block_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("end-block-id")); v != "" {
		ro["end_block_id"] = v
	}
	if v := strings.TrimSpace(runtime.Str("keyword")); v != "" {
		ro["keyword"] = v
	}
	if v := runtime.Int("context-before"); v > 0 {
		ro["context_before"] = strconv.Itoa(v)
	}
	if v := runtime.Int("context-after"); v > 0 {
		ro["context_after"] = strconv.Itoa(v)
	}
	if v := runtime.Int("max-depth"); v >= 0 {
		ro["max_depth"] = strconv.Itoa(v)
	}
	return ro
}

func effectiveFetchReadMode(runtime *common.RuntimeContext) string {
	mode := rawFetchReadMode(runtime)
	if shouldUseDocSelectionAnchor(runtime, mode) {
		if anchor := docSelectionAnchorStartBlockID(runtime); anchor != "" {
			return "range"
		}
	}
	return mode
}

func rawFetchReadMode(runtime *common.RuntimeContext) string {
	mode := strings.TrimSpace(runtime.Str("scope"))
	if mode == "" {
		return "full"
	}
	return mode
}

func effectiveFetchStartBlockID(runtime *common.RuntimeContext, mode string) string {
	if v := strings.TrimSpace(runtime.Str("start-block-id")); v != "" {
		return v
	}
	if mode == "range" && shouldUseDocSelectionAnchor(runtime, rawFetchReadMode(runtime)) {
		if anchor := docSelectionAnchorStartBlockID(runtime); anchor != "" {
			return anchor
		}
	}
	return ""
}

func shouldUseDocSelectionAnchor(runtime *common.RuntimeContext, mode string) bool {
	if runtime.Changed("start-block-id") || runtime.Changed("end-block-id") {
		return false
	}
	if runtime.Changed("scope") {
		return mode == "range"
	}
	return mode == "" || mode == "full"
}

func docSelectionAnchorStartBlockID(runtime *common.RuntimeContext) string {
	ref, err := parseDocumentRef(runtime.Str("doc"))
	if err != nil {
		return ""
	}
	anchor, ok := parseDocShareSelectionAnchor(ref.Fragment)
	if !ok {
		return ""
	}
	return anchor
}

func parseDocShareSelectionAnchor(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "#")
	const prefix = "share-"
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	anchorID := strings.TrimSpace(strings.TrimPrefix(value, prefix))
	if anchorID == "" {
		return "", false
	}
	return prefix + anchorID, true
}

// effectiveFetchDetail degrades detail options that cannot be represented by
// non-XML exports. The original flag value is left intact so callers can still
// surface an explicit warning in execute output.
func effectiveFetchDetail(runtime *common.RuntimeContext) string {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	detail := strings.TrimSpace(runtime.Str("detail"))
	if format == "" || format == "xml" {
		return detail
	}
	if detail == "with-ids" || detail == "full" {
		return "simple"
	}
	return detail
}

func addFetchDetailDowngradeWarning(runtime *common.RuntimeContext, data map[string]interface{}) string {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	detail := strings.TrimSpace(runtime.Str("detail"))
	if format == "" || format == "xml" {
		return ""
	}
	if detail != "with-ids" && detail != "full" {
		return ""
	}
	warning := fmt.Sprintf("--detail %s is only supported with --doc-format xml; returning %s output and ignoring the unsupported detail option", detail, format)
	appendDocWarning(data, warning)
	return warning
}

// validateReadModeFlags 客户端前置校验，服务端也会再校验一次。
func validateReadModeFlags(runtime *common.RuntimeContext) error {
	mode := effectiveFetchReadMode(runtime)
	if mode == "" || mode == "full" {
		return nil
	}

	if v := runtime.Int("context-before"); v < 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--context-before must be >= 0, got %d", v).WithParam("--context-before")
	}
	if v := runtime.Int("context-after"); v < 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--context-after must be >= 0, got %d", v).WithParam("--context-after")
	}
	if v := runtime.Int("max-depth"); v < -1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--max-depth must be >= -1, got %d", v).WithParam("--max-depth")
	}

	switch mode {
	case "outline":
		return nil
	case "range":
		if effectiveFetchStartBlockID(runtime, mode) == "" &&
			strings.TrimSpace(runtime.Str("end-block-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "range mode requires --start-block-id or --end-block-id").WithParams(
				errs.InvalidParam{Name: "--start-block-id", Reason: "provide --start-block-id or --end-block-id for range mode"},
				errs.InvalidParam{Name: "--end-block-id", Reason: "provide --start-block-id or --end-block-id for range mode"},
			)
		}
		return nil
	case "keyword":
		if strings.TrimSpace(runtime.Str("keyword")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "keyword mode requires --keyword").WithParam("--keyword")
		}
		return nil
	case "section":
		if strings.TrimSpace(runtime.Str("start-block-id")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "section mode requires --start-block-id").WithParam("--start-block-id")
		}
		return nil
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --scope %q", mode).WithParam("--scope")
	}
}
