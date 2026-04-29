// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

var MarkdownCreate = common.Shortcut{
	Service:     "markdown",
	Command:     "+create",
	Description: "Create a Markdown file in Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "folder-token", Desc: "target Drive folder token (default: root folder)"},
		{Name: "name", Desc: "file name with .md suffix; required with --content, optional with --file"},
		{Name: "content", Desc: "Markdown content", Input: []string{common.File, common.Stdin}},
		{Name: "file", Desc: "local .md file path"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateMarkdownSpec(runtime, markdownUploadSpec{
			FileName:    strings.TrimSpace(runtime.Str("name")),
			FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
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
			FilePath:    strings.TrimSpace(runtime.Str("file")),
			FileSet:     runtime.Changed("file"),
			Content:     runtime.Str("content"),
			ContentSet:  runtime.Changed("content"),
		}
		payload, err := markdownSourceBytes(runtime, spec)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return markdownUploadDryRun(spec, int64(len(payload)), int64(len(payload)) > markdownSinglePartSizeLimit)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := markdownUploadSpec{
			FileName:    strings.TrimSpace(runtime.Str("name")),
			FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
			FilePath:    strings.TrimSpace(runtime.Str("file")),
			FileSet:     runtime.Changed("file"),
			Content:     runtime.Str("content"),
			ContentSet:  runtime.Changed("content"),
		}
		payload, err := markdownSourceBytes(runtime, spec)
		if err != nil {
			return err
		}

		result, err := uploadMarkdownFile(runtime, spec, payload)
		if err != nil {
			return err
		}

		out := map[string]interface{}{
			"file_token": result.FileToken,
			"file_name":  finalMarkdownFileName(spec),
			"size_bytes": len(payload),
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
