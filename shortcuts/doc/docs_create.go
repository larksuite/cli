// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

// v1CreateFlags returns the flag definitions for the v1 (MCP) create path.
func v1CreateFlags() []common.Flag {
	return []common.Flag{
		{Name: "title", Desc: "document title", Hidden: true},
		{Name: "markdown", Desc: "Markdown content (Lark-flavored)", Hidden: true, Input: []string{common.File, common.Stdin}},
		{Name: "folder-token", Desc: "parent folder token", Hidden: true},
		{Name: "wiki-node", Desc: "wiki node token", Hidden: true},
		{Name: "wiki-space", Desc: "wiki space ID (use my_library for personal library)", Hidden: true},
	}
}

var docsCreateFlagVersions = buildFlagVersionMap(v1CreateFlags(), v2CreateFlags())

// useV2Create returns true when the v2 (OpenAPI) create path should be used.
// Explicit --api-version v2 takes priority; otherwise auto-detect by v2-only flags.
func useV2Create(runtime *common.RuntimeContext) bool {
	if runtime.Str("api-version") == "v2" {
		return true
	}
	return runtime.Str("content") != "" ||
		runtime.Str("parent-token") != "" ||
		runtime.Str("parent-position") != ""
}

var DocsCreate = common.Shortcut{
	Service:     "docs",
	Command:     "+create",
	Description: "Create a Lark document",
	Risk:        "write",
	AuthTypes:   []string{"user", "bot"},
	Scopes:      []string{"docx:document:create"},
	Tips:        docsVersionSelectionTips,
	Flags: concatFlags(
		[]common.Flag{
			{Name: "api-version", Desc: "API version", Default: "v1", Enum: []string{"v1", "v2"}},
		},
		v1CreateFlags(),
		v2CreateFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if useV2Create(runtime) {
			return validateCreateV2(ctx, runtime)
		}
		return validateCreateV1(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if useV2Create(runtime) {
			return dryRunCreateV2(ctx, runtime)
		}
		return dryRunCreateV1(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if useV2Create(runtime) {
			return executeCreateV2(ctx, runtime)
		}
		return executeCreateV1(ctx, runtime)
	},
	PostMount: func(cmd *cobra.Command) {
		installVersionedHelp(cmd, "v1", docsCreateFlagVersions)
	},
}

// ── V1 (MCP) implementation ──

func validateCreateV1(_ context.Context, runtime *common.RuntimeContext) error {
	if runtime.Str("markdown") == "" {
		return common.FlagErrorf("--markdown is required")
	}
	count := 0
	if runtime.Str("folder-token") != "" {
		count++
	}
	if runtime.Str("wiki-node") != "" {
		count++
	}
	if runtime.Str("wiki-space") != "" {
		count++
	}
	if count > 1 {
		return common.FlagErrorf("--folder-token, --wiki-node, and --wiki-space are mutually exclusive")
	}
	return nil
}

func dryRunCreateV1(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	args := buildCreateArgsV1(runtime)
	d := common.NewDryRunAPI().
		POST(common.MCPEndpoint(runtime.Config.Brand)).
		Desc("MCP tool: create-doc").
		Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "create-doc", "arguments": args}}).
		Set("mcp_tool", "create-doc").Set("args", args)
	if runtime.IsBot() {
		d.Desc("After create-doc succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new document.")
	}
	return d
}

func executeCreateV1(_ context.Context, runtime *common.RuntimeContext) error {
	warnDeprecatedV1(runtime, "+create")
	// Surface callout type= hint so users know to switch to background-color/
	// border-color when they want a colored callout. Non-blocking, advisory.
	if md := runtime.Str("markdown"); md != "" {
		WarnCalloutType(md, runtime.IO().ErrOut)
	}
	args := buildCreateArgsV1(runtime)
	result, err := common.CallMCPTool(runtime, "create-doc", args)
	if err != nil {
		return err
	}
	augmentCreateResultV1(runtime, result)
	normalizeWhiteboardResult(result, runtime.Str("markdown"))
	runtime.Out(result, nil)
	return nil
}

func buildCreateArgsV1(runtime *common.RuntimeContext) map[string]interface{} {
	md := runtime.Str("markdown")
	args := map[string]interface{}{
		"markdown": md,
	}
	if v := runtime.Str("title"); v != "" {
		args["title"] = v
	}
	if v := runtime.Str("folder-token"); v != "" {
		args["folder_token"] = v
	}
	if v := runtime.Str("wiki-node"); v != "" {
		args["wiki_node"] = v
	}
	if v := runtime.Str("wiki-space"); v != "" {
		args["wiki_space"] = v
	}
	return args
}

type docsPermissionTarget struct {
	Token string
	Type  string
}

func augmentCreateResultV1(runtime *common.RuntimeContext, result map[string]interface{}) {
	target := selectPermissionTarget(result)
	if grant := common.AutoGrantCurrentUserDrivePermission(runtime, target.Token, target.Type); grant != nil {
		result["permission_grant"] = grant
	}
	fallbackDocURLV1(runtime, result)
}

// fallbackDocURLV1 fills result.doc_url with a brand-standard URL when the MCP
// response did not include one but did include a doc_id. This protects against
// degraded MCP responses (multi-content, non-JSON text) where ExtractMCPResult
// drops structured fields.
func fallbackDocURLV1(runtime *common.RuntimeContext, result map[string]interface{}) {
	if strings.TrimSpace(common.GetString(result, "doc_url")) != "" {
		return
	}
	docID := strings.TrimSpace(common.GetString(result, "doc_id"))
	if docID == "" {
		return
	}
	if u := common.BuildResourceURL(runtime.Config.Brand, "docx", docID); u != "" {
		result["doc_url"] = u
	}
}

func selectPermissionTarget(result map[string]interface{}) docsPermissionTarget {
	if ref, ok := parsePermissionTargetFromURL(common.GetString(result, "doc_url")); ok {
		return ref
	}
	docID := strings.TrimSpace(common.GetString(result, "doc_id"))
	if docID != "" {
		return docsPermissionTarget{Token: docID, Type: "docx"}
	}
	return docsPermissionTarget{}
}

func parsePermissionTargetFromURL(docURL string) (docsPermissionTarget, bool) {
	if strings.TrimSpace(docURL) == "" {
		return docsPermissionTarget{}, false
	}
	ref, err := parseDocumentRef(docURL)
	if err != nil {
		return docsPermissionTarget{}, false
	}
	switch ref.Kind {
	case "wiki":
		return docsPermissionTarget{Token: ref.Token, Type: "wiki"}, true
	case "doc", "docx":
		return docsPermissionTarget{Token: ref.Token, Type: ref.Kind}, true
	default:
		return docsPermissionTarget{}, false
	}
}

// normalizeWhiteboardResult normalizes board_tokens in the MCP response when
// whiteboard creation markdown is detected.
func normalizeWhiteboardResult(result map[string]interface{}, markdown string) {
	if !isWhiteboardCreateMarkdown(markdown) {
		return
	}
	result["board_tokens"] = normalizeBoardTokens(result["board_tokens"])
}

func isWhiteboardCreateMarkdown(markdown string) bool {
	lower := strings.ToLower(markdown)
	if strings.Contains(lower, "```mermaid") || strings.Contains(lower, "```plantuml") {
		return true
	}
	return strings.Contains(lower, "<whiteboard") &&
		(strings.Contains(lower, `type="blank"`) || strings.Contains(lower, `type='blank'`))
}

func normalizeBoardTokens(raw interface{}) []string {
	switch v := raw.(type) {
	case nil:
		return []string{}
	case []string:
		return v
	case []interface{}:
		tokens := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				tokens = append(tokens, s)
			}
		}
		return tokens
	case string:
		if v == "" {
			return []string{}
		}
		return []string{v}
	default:
		return []string{}
	}
}

// ── Shared helpers ──

// concatFlags combines multiple flag slices into one.
func concatFlags(slices ...[]common.Flag) []common.Flag {
	var out []common.Flag
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}

// buildFlagVersionMap creates a flag name → version mapping from v1 and v2 flag lists.
func buildFlagVersionMap(v1, v2 []common.Flag) map[string]string {
	m := make(map[string]string, len(v1)+len(v2))
	for _, f := range v1 {
		m[f.Name] = "v1"
	}
	for _, f := range v2 {
		m[f.Name] = "v2"
	}
	return m
}
