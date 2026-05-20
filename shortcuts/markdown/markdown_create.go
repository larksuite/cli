// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var MarkdownCreate = common.Shortcut{
	Service:     "markdown",
	Command:     "+create",
	Description: "Create a Markdown file in Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "folder-token", Desc: "target Drive folder token (default: root folder; mutually exclusive with --wiki-token)"},
		{Name: "wiki-token", Desc: "target wiki node token (uploads under that wiki node; mutually exclusive with --folder-token)"},
		{Name: "name", Desc: "file name with .md suffix; required with --content, optional with --file"},
		{Name: "content", Desc: "Markdown content", Input: []string{common.File, common.Stdin}},
		{Name: "file", Desc: "local .md file path"},
	},
	Tips: []string{
		"Omit both --folder-token and --wiki-token to create the Markdown file in the caller's Drive root folder.",
		"Use --wiki-token <wiki_node_token> to create the Markdown file under a wiki node; the shortcut maps this to parent_type=wiki automatically.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMarkdownSpec(runtime, markdownUploadSpec{
			FileName:    strings.TrimSpace(runtime.Str("name")),
			FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
			WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
			FilePath:    strings.TrimSpace(runtime.Str("file")),
			FileSet:     runtime.Changed("file"),
			Content:     runtime.Str("content"),
			ContentSet:  runtime.Changed("content"),
		}, true)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := markdownUploadSpec{
			FileName:    strings.TrimSpace(runtime.Str("name")),
			FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
			WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
			FilePath:    strings.TrimSpace(runtime.Str("file")),
			FileSet:     runtime.Changed("file"),
			Content:     runtime.Str("content"),
			ContentSet:  runtime.Changed("content"),
		}
		fileSize, err := markdownSourceSize(runtime, spec)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		dry := markdownUploadDryRun(spec, fileSize, fileSize > markdownSinglePartSizeLimit)
		dry.POST("/open-apis/drive/v1/metas/batch_query").
			Desc("Fetch the created Markdown file's real access URL").
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": "<file_token from upload response>",
						"doc_type":  "file",
					},
				},
				"with_url": true,
			})
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := markdownUploadSpec{
			FileName:    strings.TrimSpace(runtime.Str("name")),
			FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
			WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
			FilePath:    strings.TrimSpace(runtime.Str("file")),
			FileSet:     runtime.Changed("file"),
			Content:     runtime.Str("content"),
			ContentSet:  runtime.Changed("content"),
		}
		fileSize, err := markdownSourceSize(runtime, spec)
		if err != nil {
			return err
		}

		var result markdownUploadResult
		if spec.FileSet {
			result, err = uploadMarkdownLocalFile(runtime, spec, fileSize)
		} else {
			result, err = uploadMarkdownContent(runtime, spec, []byte(spec.Content))
		}
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"file_token": result.FileToken,
			"file_name":  finalMarkdownFileName(spec),
			"size_bytes": fileSize,
		}
		if u, metaErr := common.FetchDriveMetaURL(runtime, result.FileToken, "file"); metaErr == nil && strings.TrimSpace(u) != "" {
			out["url"] = u
		} else if metaErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "warning: created Markdown file URL lookup failed: %v\n", metaErr)
		}
		if grant := common.AutoGrantCurrentUserDrivePermission(runtime, result.FileToken, "file"); grant != nil {
			out["permission_grant"] = grant
		}

		runtime.OutFormat(out, nil, func(w io.Writer) {
			prettyPrintMarkdownWrite(w, out)
		})
		return nil
	},
}
