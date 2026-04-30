// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

var validModesV1 = map[string]bool{
	"append":        true,
	"overwrite":     true,
	"replace_range": true,
	"replace_all":   true,
	"insert_before": true,
	"insert_after":  true,
	"delete_range":  true,
}

var needsSelectionV1 = map[string]bool{
	"replace_range": true,
	"replace_all":   true,
	"insert_before": true,
	"insert_after":  true,
	"delete_range":  true,
}

// v1UpdateFlags returns the flag definitions for the v1 (MCP) update path.
func v1UpdateFlags() []common.Flag {
	return []common.Flag{
		{Name: "mode", Desc: "update mode: append | overwrite | replace_range | replace_all | insert_before | insert_after | delete_range", Hidden: true},
		{Name: "markdown", Desc: "new content (Lark-flavored Markdown; create blank whiteboards with <whiteboard type=\"blank\"></whiteboard>, repeat to create multiple boards)", Hidden: true, Input: []string{common.File, common.Stdin}},
		{Name: "selection-with-ellipsis", Desc: "content locator (e.g. 'start...end')", Hidden: true},
		{Name: "selection-by-title", Desc: "title locator (e.g. '## Section')", Hidden: true},
		{Name: "new-title", Desc: "also update document title", Hidden: true},
	}
}

var docsUpdateFlagVersions = buildFlagVersionMap(v1UpdateFlags(), v2UpdateFlags())

// useV2Update returns true when the v2 (OpenAPI) update path should be used.
// Explicit --api-version v2 takes priority; otherwise auto-detect by v2-only flags.
func useV2Update(runtime *common.RuntimeContext) bool {
	if runtime.Str("api-version") == "v2" {
		return true
	}
	return runtime.Str("command") != "" ||
		runtime.Str("content") != "" ||
		runtime.Str("pattern") != "" ||
		runtime.Str("block-id") != "" ||
		runtime.Str("src-block-ids") != ""
}

var DocsUpdate = common.Shortcut{
	Service:     "docs",
	Command:     "+update",
	Description: "Update a Lark document",
	Risk:        "write",
	Scopes:      []string{"docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Tips:        docsVersionSelectionTips,
	Flags: concatFlags(
		[]common.Flag{
			{Name: "api-version", Desc: "API version", Default: "v1", Enum: []string{"v1", "v2"}},
			{Name: "doc", Desc: "document URL or token", Required: true},
		},
		v1UpdateFlags(),
		v2UpdateFlags(),
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if useV2Update(runtime) {
			return validateUpdateV2(ctx, runtime)
		}
		return validateUpdateV1(ctx, runtime)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if useV2Update(runtime) {
			return dryRunUpdateV2(ctx, runtime)
		}
		return dryRunUpdateV1(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if useV2Update(runtime) {
			return executeUpdateV2(ctx, runtime)
		}
		return executeUpdateV1(ctx, runtime)
	},
	PostMount: func(cmd *cobra.Command) {
		installVersionedHelp(cmd, "v1", docsUpdateFlagVersions)
	},
}

// ── V1 (MCP) implementation ──

func validateUpdateV1(_ context.Context, runtime *common.RuntimeContext) error {
	mode := runtime.Str("mode")
	if mode == "" {
		return common.FlagErrorf("--mode is required")
	}
	if !validModesV1[mode] {
		return common.FlagErrorf("invalid --mode %q, valid: append | overwrite | replace_range | replace_all | insert_before | insert_after | delete_range", mode)
	}

	if mode != "delete_range" && runtime.Str("markdown") == "" {
		return common.FlagErrorf("--%s mode requires --markdown", mode)
	}

	selEllipsis := runtime.Str("selection-with-ellipsis")
	selTitle := runtime.Str("selection-by-title")
	if selEllipsis != "" && selTitle != "" {
		return common.FlagErrorf("--selection-with-ellipsis and --selection-by-title are mutually exclusive")
	}

	if needsSelectionV1[mode] && selEllipsis == "" && selTitle == "" {
		return common.FlagErrorf("--%s mode requires --selection-with-ellipsis or --selection-by-title", mode)
	}
	if err := validateSelectionByTitleV1(selTitle); err != nil {
		return err
	}

	return nil
}

func validateSelectionByTitleV1(title string) error {
	if title == "" {
		return nil
	}
	trimmed := strings.TrimSpace(title)
	if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
		return common.FlagErrorf("--selection-by-title must be a single heading line (for example: '## Section')")
	}
	if strings.HasPrefix(trimmed, "#") {
		return nil
	}
	return common.FlagErrorf("--selection-by-title must include markdown heading prefix '#'. Example: --selection-by-title '## Section'")
}

func dryRunUpdateV1(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	args := buildUpdateArgsV1(runtime)
	return common.NewDryRunAPI().
		POST(common.MCPEndpoint(runtime.Config.Brand)).
		Desc("MCP tool: update-doc").
		Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "update-doc", "arguments": args}}).
		Set("mcp_tool", "update-doc").Set("args", args)
}

func executeUpdateV1(_ context.Context, runtime *common.RuntimeContext) error {
	warnDeprecatedV1(runtime, "+update")

	// Static semantic checks run before the MCP call so users see
	// warnings even if the subsequent request fails. They never block
	// execution — the update still proceeds.
	for _, w := range docsUpdateWarnings(runtime.Str("mode"), runtime.Str("markdown")) {
		fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", w)
	}

	// Surface callout type= hint so users know to switch to background-color/
	// border-color when they want a colored callout. Non-blocking, advisory.
	if md := runtime.Str("markdown"); md != "" {
		WarnCalloutType(md, runtime.IO().ErrOut)
	}

	args := buildUpdateArgsV1(runtime)

	result, err := common.CallMCPTool(runtime, "update-doc", args)
	if err != nil {
		return err
	}

	normalizeWhiteboardResult(result, runtime.Str("markdown"))
	runtime.Out(result, nil)
	return nil
}

func buildUpdateArgsV1(runtime *common.RuntimeContext) map[string]interface{} {
	args := map[string]interface{}{
		"doc_id": runtime.Str("doc"),
		"mode":   runtime.Str("mode"),
	}
	if v := runtime.Str("markdown"); v != "" {
		args["markdown"] = v
	}
	if v := runtime.Str("selection-with-ellipsis"); v != "" {
		args["selection_with_ellipsis"] = v
	}
	if v := runtime.Str("selection-by-title"); v != "" {
		args["selection_by_title"] = v
	}
	if v := runtime.Str("new-title"); v != "" {
		args["new_title"] = v
	}
	return args
}
