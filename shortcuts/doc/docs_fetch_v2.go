// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

// v2FetchFlags returns the flag definitions for the v2 (OpenAPI) fetch path.
func v2FetchFlags() []common.Flag {
	return []common.Flag{
		{Name: "doc-format", Desc: "content format", Hidden: true, Default: "xml", Enum: []string{"xml", "markdown", "text"}},
		{Name: "detail", Desc: "export detail level: simple (read-only) | with-ids (block IDs for cross-referencing) | full (all attrs for editing)", Hidden: true, Default: "simple", Enum: []string{"simple", "with-ids", "full"}},
		{Name: "revision-id", Desc: "document revision (-1 = latest)", Hidden: true, Type: "int", Default: "-1"},
		{Name: "scope", Desc: "partial read scope: outline | range | keyword | section (omit to read whole doc)", Default: "full", Enum: []string{"full", "outline", "range", "keyword", "section"}},
		{Name: "start-block-id", Desc: "range/section mode: start (anchor) block id"},
		{Name: "end-block-id", Desc: "range mode: end block id; \"-1\" = to end of document"},
		{Name: "keyword", Desc: "keyword mode: substring + regex match (case-insensitive); use '|' for OR branches, e.g. 'foo|bar' or 'bug|缺陷'"},
		{Name: "context-before", Desc: "range/keyword/section mode: sibling blocks before match", Type: "int", Default: "0"},
		{Name: "context-after", Desc: "range/keyword/section mode: sibling blocks after match", Type: "int", Default: "0"},
		{Name: "max-depth", Desc: "outline: heading level cap; range/keyword/section: block subtree depth (-1 = unlimited)", Type: "int", Default: "-1"},
	}
}

// validateFetchV2 is the Validate hook for the v2 fetch path. It runs before
// --dry-run so that invalid input fails with a structured exit code (2) and
// JSON envelope instead of slipping through dry-run as a "success".
func validateFetchV2(_ context.Context, runtime *common.RuntimeContext) error {
	if _, err := parseDocumentRef(runtime.Str("doc")); err != nil {
		return common.FlagErrorf("invalid --doc: %v", err)
	}
	if err := validateFetchDetail(runtime); err != nil {
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
		"format": runtime.Str("doc-format"),
	}
	if v := runtime.Int("revision-id"); v > 0 {
		body["revision_id"] = v
	}

	detail := runtime.Str("detail")
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

	return body
}

// buildReadOption 拼装 read_option JSON；full/空模式返回 nil，让服务端走默认全文路径。
func buildReadOption(runtime *common.RuntimeContext) map[string]interface{} {
	mode := strings.TrimSpace(runtime.Str("scope"))
	if mode == "" || mode == "full" {
		return nil
	}
	ro := map[string]interface{}{"read_mode": mode}
	if v := strings.TrimSpace(runtime.Str("start-block-id")); v != "" {
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

// validateFetchDetail 非 xml 格式（markdown/text）不承载 block_id 与样式属性，拒绝 with-ids/full。
func validateFetchDetail(runtime *common.RuntimeContext) error {
	format := strings.TrimSpace(runtime.Str("doc-format"))
	detail := strings.TrimSpace(runtime.Str("detail"))
	if format == "" || format == "xml" {
		return nil
	}
	if detail == "with-ids" || detail == "full" {
		return common.FlagErrorf("--detail %s is only supported with --doc-format xml; %s output has no block ids, use --detail simple or switch to --doc-format xml", detail, format)
	}
	return nil
}

// validateReadModeFlags 客户端前置校验，服务端也会再校验一次。
func validateReadModeFlags(runtime *common.RuntimeContext) error {
	mode := strings.TrimSpace(runtime.Str("scope"))
	if mode == "" || mode == "full" {
		return nil
	}

	if v := runtime.Int("context-before"); v < 0 {
		return common.FlagErrorf("--context-before must be >= 0, got %d", v)
	}
	if v := runtime.Int("context-after"); v < 0 {
		return common.FlagErrorf("--context-after must be >= 0, got %d", v)
	}
	if v := runtime.Int("max-depth"); v < -1 {
		return common.FlagErrorf("--max-depth must be >= -1, got %d", v)
	}

	switch mode {
	case "outline":
		return nil
	case "range":
		if strings.TrimSpace(runtime.Str("start-block-id")) == "" &&
			strings.TrimSpace(runtime.Str("end-block-id")) == "" {
			return common.FlagErrorf("range mode requires --start-block-id or --end-block-id")
		}
		return nil
	case "keyword":
		if strings.TrimSpace(runtime.Str("keyword")) == "" {
			return common.FlagErrorf("keyword mode requires --keyword")
		}
		return nil
	case "section":
		if strings.TrimSpace(runtime.Str("start-block-id")) == "" {
			return common.FlagErrorf("section mode requires --start-block-id")
		}
		return nil
	default:
		return common.FlagErrorf("invalid --scope %q", mode)
	}
}
