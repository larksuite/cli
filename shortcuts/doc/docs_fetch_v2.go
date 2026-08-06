// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/common/contentread"
)

const (
	docsFetchExtraParam    = `{"enable_user_cite_reference_map":true,"return_html5_block_data":true}`
	docsFetchContentJQPath = ".data.document.content"
)

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
		// Whole-document Markdown pagination with block anchors.
		{Name: "full", Type: "bool", Default: "false", Desc: "markdown whole-doc only: return the whole document in one response (disable auto-pagination)"},
		{Name: "page-token", Desc: "markdown whole-doc only: continue a paginated read from a prior next_page_token"},
		{Name: "page-size", Type: "int", Default: "0", Desc: "markdown whole-doc only: per-page token budget hint (0 = server default)"},
		{Name: "embed-max-rows", Type: "int", Default: "50", Desc: "markdown only: cap each rendered table to N data rows (0 = no limit)"},
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
	if _, err := common.ValidatePageSizeTyped(runtime, "page-size", 0, 0, math.MaxInt32); err != nil {
		return err
	}
	if err := validateReadModeFlags(runtime); err != nil {
		return err
	}
	return validatePaginatedReadFlags(runtime)
}

// useAnchoredMarkdownRead reports whether the whole-document Markdown read uses the
// paginated anchored-Markdown path instead of the document API. Only Markdown +
// scope=full qualifies; XML, partial scopes, and im-markdown use the document API.
func useAnchoredMarkdownRead(runtime *common.RuntimeContext) bool {
	if runtime.Str("doc-format") != "markdown" || effectiveFetchReadMode(runtime) != "full" {
		return false
	}
	// The paginated Markdown API has no field for a historical revision or a cite
	// language, so it would silently return the latest revision / default
	// language. Route to the document API (which honors both) when
	// either is explicitly requested, instead of silently dropping the user's intent.
	if runtime.Int("revision-id") > 0 || runtime.Changed("lang") {
		return false
	}
	return true
}

// validatePaginatedReadFlags checks the paginated-read flags (--full/--page-token/
// --page-size) apply only to the markdown whole-doc path.
func validatePaginatedReadFlags(runtime *common.RuntimeContext) error {
	if runtime.Bool("full") && (strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0) {
		return common.ValidationErrorf("--full cannot be combined with --page-token/--page-size").WithParam("--full")
	}
	usePaginatedRead := useAnchoredMarkdownRead(runtime)
	pagination := runtime.Bool("full") || strings.TrimSpace(runtime.Str("page-token")) != "" || runtime.Int("page-size") > 0
	if pagination && !usePaginatedRead {
		// Markdown + full would otherwise enable the paginated read; if it is off here,
		// a historical revision or an explicit --lang forced the document API path, so
		// the pagination-only flags conflict with those (not with format/scope).
		if runtime.Str("doc-format") == "markdown" && effectiveFetchReadMode(runtime) == "full" {
			return common.ValidationErrorf("--full/--page-token/--page-size are not supported together with a historical --revision-id (or an explicit --lang), which use the document API path").WithParam("--full")
		}
		return common.ValidationErrorf("--full/--page-token/--page-size only apply to --doc-format markdown with --scope full").WithParam("--full")
	}
	return nil
}

func dryRunFetchV2(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	if useAnchoredMarkdownRead(runtime) {
		return dryRunAnchoredMarkdownFetch(runtime)
	}
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

func executeFetchV2(ctx context.Context, runtime *common.RuntimeContext) error {
	ref, _ := parseDocumentRef(runtime.Str("doc"))
	var resolution common.FetchURLResolution
	if useAnchoredMarkdownRead(runtime) {
		resolution = resolvedFetchURL(runtime)
	}
	diagnoseWikiType := newWikiFetchTypeGuard(runtime, ref, resolution.WikiProbeAttempted, resolution.WikiNode)

	// Whole-document Markdown reads try the paginated Markdown endpoint first.
	// A first-page failure falls back to the document-fetch API, preserving the
	// behavior available before the paginated path was introduced.
	if useAnchoredMarkdownRead(runtime) {
		handled, fetchErr := runAnchoredMarkdownFetch(ctx, runtime, resolution.URL)
		if handled {
			return fetchErr
		}
		if redirectErr := diagnoseWikiType(fetchErr); redirectErr != nil {
			return redirectErr
		}
		continuation := contentread.IsPageContinuation(strings.TrimSpace(runtime.Str("page-token")))
		if handled, err := handlePaginatedReadFailure(runtime, continuation, fetchErr); handled || err != nil {
			return err
		}
	}

	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", ref.Token)
	body := buildFetchBody(runtime)

	data, err := doDocAPI(runtime, "POST", apiPath, body)
	if err != nil {
		if redirectErr := diagnoseWikiType(err); redirectErr != nil {
			return redirectErr
		}
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

	document, ok := data["document"].(map[string]interface{})
	if !ok {
		runtime.OutFormatRaw(data, nil, nil)
		return nil
	}
	content, ok := document["content"].(string)
	if !ok {
		runtime.OutFormatRaw(data, nil, nil)
		return nil
	}
	delivery, scan, err := common.PrepareFetchContentDelivery(runtime, data, content, docsFetchContentJQPath)
	if err != nil {
		return err
	}
	emitted := cloneFetchDocumentData(data)
	applyFetchContentDelivery(emitted, delivery)
	runtime.OutFormatRawWithSafety(emitted, nil, func(w io.Writer) {
		common.WriteFetchContentPretty(w, delivery)
	}, scan)
	return nil
}

// newWikiFetchTypeGuard lazily resolves a Wiki input only after the Doc read
// has failed. Successful Doc/Docx Wiki reads therefore stay on the fast path
// without an extra get_node call. The probe result is cached so a primary read
// followed by a fallback failure never probes the same node twice.
func newWikiFetchTypeGuard(runtime *common.RuntimeContext, ref documentRef, checked bool, resolvedNode *common.WikiNode) func(error) error {
	actualType := ""
	actualToken := ""
	if resolvedNode != nil {
		actualType = strings.TrimSpace(resolvedNode.ObjType)
		actualToken = strings.TrimSpace(resolvedNode.ObjToken)
	}

	return func(cause error) error {
		input := strings.TrimSpace(runtime.Str("doc"))
		// Bare tokens are intentionally parsed as docx because their type is
		// ambiguous without I/O. After a failed Doc read, treat them as possible
		// Wiki node tokens and probe once; a normal docx token simply fails that
		// best-effort probe and keeps its original error.
		wikiCandidate := ref.Kind == "wiki" || (ref.Kind == "docx" && !strings.Contains(input, "://"))
		if !wikiCandidate || !shouldDiagnoseWikiFetchType(cause) {
			return nil
		}
		if !checked {
			checked = true
			node, err := common.ResolveWikiNode(runtime, ref.Token)
			if err != nil {
				// Type enrichment is best effort. Permission, transport, and malformed
				// get_node responses must not replace the original fetch failure.
				return nil //nolint:nilerr // Retain the original fetch failure when the optional Wiki probe fails.
			}
			actualType = strings.TrimSpace(node.ObjType)
			actualToken = strings.TrimSpace(node.ObjToken)
		}

		switch strings.ToLower(actualType) {
		case "", "doc", "docx":
			return nil
		}

		redirectErr := errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"Wiki input resolves to %q, but docs +fetch only supports doc/docx content", actualType).
			WithParam("--doc").
			WithHint("%s; do not retry `docs +fetch` for this Wiki resource",
				wikiFetchFallbackHint(input, ref, actualType, actualToken))
		if cause != nil {
			redirectErr.WithCause(cause)
		}
		return redirectErr
	}
}

// wikiFetchFallbackHint routes only types drive +fetch actually supports there.
// Mindnote has its own content API; unknown future types get an inspect command
// instead of a remediation that is guaranteed to fail.
func wikiFetchFallbackHint(input string, ref documentRef, actualType, actualToken string) string {
	switch strings.ToLower(actualType) {
	case "sheet", "sheets", "base", "bitable", "slides", "file", "minutes":
		if ref.Kind == "wiki" && strings.Contains(input, "://") {
			return fmt.Sprintf("run once: `lark-cli drive +fetch --url %s`", shellQuoteFetchURL(input))
		}
		return fmt.Sprintf("run once: `lark-cli drive +fetch --token %s --type wiki`", shellQuoteFetchURL(ref.Token))
	case "mindnote":
		return fmt.Sprintf("run: `lark-cli mindnotes nodes list --mindnote-id %s`", shellQuoteFetchURL(actualToken))
	default:
		if ref.Kind == "wiki" && strings.Contains(input, "://") {
			return fmt.Sprintf("inspect the resource with `lark-cli drive +inspect --url %s` and use its entity-specific reader", shellQuoteFetchURL(input))
		}
		return fmt.Sprintf("inspect the resource with `lark-cli drive +inspect --url %s --type wiki` and use its entity-specific reader", shellQuoteFetchURL(ref.Token))
	}
}

// shouldDiagnoseWikiFetchType limits the extra Wiki probe to failures that can
// plausibly mean "this is not a document": an untyped failure, a malformed
// content response, or an upstream API error. Authentication, permission,
// network, and safety failures keep their original recovery guidance.
func shouldDiagnoseWikiFetchType(cause error) bool {
	if cause == nil {
		return false
	}
	if !errs.IsTyped(cause) || errs.IsAPI(cause) {
		return true
	}
	problem, ok := errs.ProblemOf(cause)
	return ok && problem.Subtype == errs.SubtypeInvalidResponse
}

// shellQuoteFetchURL returns a POSIX-shell-safe single argument. The hint is
// intentionally executable and preserves Wiki query/fragment selectors.
func shellQuoteFetchURL(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
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
