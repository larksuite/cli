// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var MarkdownOverwrite = common.Shortcut{
	Service:     "markdown",
	Command:     "+overwrite",
	Description: "Overwrite an existing Markdown file in Drive",
	Risk:        "write",
	Scopes:      []string{"drive:file:upload", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target Markdown file token", Required: true},
		{Name: "name", Desc: "optional file name with .md suffix; overrides the existing/local file name"},
		{Name: "content", Desc: "new Markdown content", Input: []string{common.File, common.Stdin}},
		{Name: "file", Desc: "local .md file path"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		return validateMarkdownSpec(runtime, markdownUploadSpec{
			FileToken:  fileToken,
			FileName:   strings.TrimSpace(runtime.Str("name")),
			FilePath:   strings.TrimSpace(runtime.Str("file")),
			FileSet:    runtime.Changed("file"),
			Content:    runtime.Str("content"),
			ContentSet: runtime.Changed("content"),
		}, false)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := markdownUploadSpec{
			FileToken:  strings.TrimSpace(runtime.Str("file-token")),
			FileName:   strings.TrimSpace(runtime.Str("name")),
			FilePath:   strings.TrimSpace(runtime.Str("file")),
			FileSet:    runtime.Changed("file"),
			Content:    runtime.Str("content"),
			ContentSet: runtime.Changed("content"),
		}
		fileSize, err := markdownSourceSize(runtime, spec)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return markdownOverwriteDryRun(spec, fileSize, fileSize > markdownSinglePartSizeLimit)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		spec := markdownUploadSpec{
			FileToken:  fileToken,
			FileName:   strings.TrimSpace(runtime.Str("name")),
			FilePath:   strings.TrimSpace(runtime.Str("file")),
			FileSet:    runtime.Changed("file"),
			Content:    runtime.Str("content"),
			ContentSet: runtime.Changed("content"),
		}

		fileSize, err := markdownSourceSize(runtime, spec)
		if err != nil {
			return err
		}

		fileName := strings.TrimSpace(spec.FileName)
		if fileName == "" && spec.FileSet {
			fileName = filepath.Base(spec.FilePath)
		}
		if fileName == "" {
			remoteName, err := fetchMarkdownFileName(runtime, fileToken)
			if err != nil {
				return err
			}
			fileName = strings.TrimSpace(remoteName)
		}
		if fileName == "" {
			fileName = fileToken + ".md"
		}
		spec.FileName = fileName

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
			"file_name":  fileName,
			"version":    result.Version,
			"size_bytes": fileSize,
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			prettyPrintMarkdownWrite(w, out)
		})
		return nil
	},
}
