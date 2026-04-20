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
		{Name: "title", Desc: "document title"},
		{Name: "markdown", Desc: "Markdown content (Lark-flavored)", Required: true, Input: []string{common.File, common.Stdin}},
		{Name: "folder-token", Desc: "parent folder token"},
		{Name: "wiki-node", Desc: "wiki node token"},
		{Name: "wiki-space", Desc: "wiki space ID (use my_library for personal library)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
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
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		args := buildDocsCreateArgs(runtime)
		d := common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: create-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "create-doc", "arguments": args}}).
			Set("mcp_tool", "create-doc").Set("args", args)
		if runtime.IsBot() {
			d.Desc("After create-doc succeeds in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on the new document.")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args := buildDocsCreateArgs(runtime)
		result, err := common.CallMCPTool(runtime, "create-doc", args)
		if err != nil {
			return err
		}
		augmentDocsCreateResult(runtime, result)

		normalizeDocsUpdateResult(result, runtime.Str("markdown"))
		runtime.Out(result, nil)
		return nil
	},
}

func buildDocsCreateArgs(runtime *common.RuntimeContext) map[string]interface{} {
	args := map[string]interface{}{
		"markdown": runtime.Str("markdown"),
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

func augmentDocsCreateResult(runtime *common.RuntimeContext, result map[string]interface{}) {
	target := selectDocsPermissionTarget(result)
	if grant := common.AutoGrantCurrentUserDrivePermission(runtime, target.Token, target.Type); grant != nil {
		result["permission_grant"] = grant
	}
}

func selectDocsPermissionTarget(result map[string]interface{}) docsPermissionTarget {
	if ref, ok := parseDocsPermissionTargetFromURL(common.GetString(result, "doc_url")); ok {
		return ref
	}

	docID := strings.TrimSpace(common.GetString(result, "doc_id"))
	if docID != "" {
		return docsPermissionTarget{Token: docID, Type: "docx"}
	}
	return docsPermissionTarget{}
}

func parseDocsPermissionTargetFromURL(docURL string) (docsPermissionTarget, bool) {
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
