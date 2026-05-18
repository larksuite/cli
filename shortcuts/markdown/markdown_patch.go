// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	markdownPatchModeLiteral = "literal"
	markdownPatchModeRegex   = "regex"
)

type markdownPatchSpec struct {
	FileToken  string
	Pattern    string
	Content    string
	ContentSet bool
	Regex      bool
}

var MarkdownPatch = common.Shortcut{
	Service:     "markdown",
	Command:     "+patch",
	Description: "Patch a Markdown file in Drive via fetch-local-replace-overwrite",
	Risk:        "write",
	Scopes:      []string{"drive:file:download", "drive:file:upload", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "target Markdown file token", Required: true},
		{Name: "pattern", Desc: "literal text or RE2 regex to match", Input: []string{common.File, common.Stdin}},
		{Name: "content", Desc: "replacement Markdown content", Input: []string{common.File, common.Stdin}},
		{Name: "regex", Type: "bool", Desc: "interpret --pattern as RE2 regular expression"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := newMarkdownPatchSpec(runtime)
		if err := validateMarkdownPatchSpec(runtime, spec); err != nil {
			return err
		}
		if spec.Regex {
			if _, err := regexp.Compile(spec.Pattern); err != nil {
				return output.ErrValidation("invalid --pattern regex: %s", err)
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := newMarkdownPatchSpec(runtime)
		mode := markdownPatchModeLiteral
		if spec.Regex {
			mode = markdownPatchModeRegex
		}
		sizeThreshold := common.FormatSize(markdownSinglePartSizeLimit)
		return common.NewDryRunAPI().
			Desc("Download the current Markdown file, apply the replacement locally, and overwrite the file only when matches are found").
			GET("/open-apis/drive/v1/files/:file_token/download").
			Desc("[1] Download the current Markdown content").
			Set("file_token", spec.FileToken).
			POST("/open-apis/drive/v1/metas/batch_query").
			Desc("[2] Read current file metadata to preserve the existing file name before overwrite").
			Body(map[string]interface{}{
				"request_docs": []map[string]interface{}{
					{
						"doc_token": spec.FileToken,
						"doc_type":  "file",
					},
				},
			}).
			POST("/open-apis/drive/v1/files/upload_all").
			Desc("[3a] If the patched Markdown is at most "+sizeThreshold+", overwrite the file with multipart/form-data upload_all").
			Body(map[string]interface{}{
				"file_name":   "<existing_remote_name_or_" + spec.FileToken + ".md>",
				"parent_type": "explorer",
				"parent_node": "",
				"size":        "<updated_size_bytes>",
				"file":        "<patched_markdown_content>",
				"file_token":  spec.FileToken,
			}).
			POST("/open-apis/drive/v1/files/upload_prepare").
			Desc("[3b] If the patched Markdown exceeds "+sizeThreshold+", initialize multipart overwrite upload").
			Body(map[string]interface{}{
				"file_name":   "<existing_remote_name_or_" + spec.FileToken + ".md>",
				"parent_type": "explorer",
				"parent_node": "",
				"size":        "<updated_size_bytes>",
				"file_token":  spec.FileToken,
			}).
			POST("/open-apis/drive/v1/files/upload_part").
			Desc("[3c] Upload file parts (repeated) when multipart overwrite is required").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"seq":       "<chunk_index>",
				"size":      "<chunk_size>",
				"file":      "<chunk_binary>",
			}).
			POST("/open-apis/drive/v1/files/upload_finish").
			Desc("[3d] Finalize multipart overwrite upload and return the new version").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"block_num": "<block_num>",
			}).
			Set("mode", mode)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := newMarkdownPatchSpec(runtime)

		resp, err := openMarkdownDownload(ctx, runtime, spec.FileToken)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			return output.ErrNetwork("download failed: %s", err)
		}
		original := string(payload)
		patched, matchCount, err := applyMarkdownPatch(original, spec)
		if err != nil {
			return err
		}

		mode := markdownPatchModeLiteral
		if spec.Regex {
			mode = markdownPatchModeRegex
		}

		out := map[string]interface{}{
			"updated":           false,
			"mode":              mode,
			"match_count":       matchCount,
			"version":           "",
			"size_bytes_before": len(payload),
			"size_bytes_after":  len(payload),
		}
		if matchCount == 0 {
			runtime.OutFormat(out, nil, func(w io.Writer) {
				prettyPrintMarkdownPatch(w, out)
			})
			return nil
		}

		patchedPayload := []byte(patched)
		if err := validateNonEmptyMarkdownSize(int64(len(patchedPayload))); err != nil {
			return err
		}

		specUpload := markdownUploadSpec{
			FileToken: spec.FileToken,
		}
		fileName, err := resolveMarkdownOverwriteFileName(runtime, specUpload)
		if err != nil {
			return err
		}
		specUpload.FileName = fileName

		result, err := uploadMarkdownContent(runtime, specUpload, patchedPayload)
		if err != nil {
			return err
		}

		out["updated"] = true
		out["version"] = result.Version
		out["size_bytes_after"] = len(patchedPayload)

		runtime.OutFormat(out, nil, func(w io.Writer) {
			prettyPrintMarkdownPatch(w, out)
		})
		return nil
	},
}

func newMarkdownPatchSpec(runtime *common.RuntimeContext) markdownPatchSpec {
	return markdownPatchSpec{
		FileToken:  strings.TrimSpace(runtime.Str("file-token")),
		Pattern:    runtime.Str("pattern"),
		Content:    runtime.Str("content"),
		ContentSet: runtime.Changed("content"),
		Regex:      runtime.Bool("regex"),
	}
}

func validateMarkdownPatchSpec(runtime *common.RuntimeContext, spec markdownPatchSpec) error {
	if err := validate.ResourceName(spec.FileToken, "--file-token"); err != nil {
		return output.ErrValidation("%s", err)
	}
	if !runtime.Changed("pattern") {
		return common.FlagErrorf("--pattern is required")
	}
	if spec.Pattern == "" {
		return output.ErrValidation("--pattern cannot be empty")
	}
	if !spec.ContentSet {
		return common.FlagErrorf("--content is required")
	}
	return nil
}

func applyMarkdownPatch(original string, spec markdownPatchSpec) (string, int, error) {
	if !spec.Regex {
		return strings.ReplaceAll(original, spec.Pattern, spec.Content), strings.Count(original, spec.Pattern), nil
	}
	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return "", 0, output.ErrValidation("invalid --pattern regex: %s", err)
	}
	matches := re.FindAllStringIndex(original, -1)
	return re.ReplaceAllString(original, spec.Content), len(matches), nil
}

func prettyPrintMarkdownPatch(w io.Writer, data map[string]interface{}) {
	updated := common.GetBool(data, "updated")
	if updated {
		io.WriteString(w, "updated: true\n")
	} else {
		io.WriteString(w, "updated: false\n")
	}
	io.WriteString(w, "mode: "+common.GetString(data, "mode")+"\n")
	fmt.Fprintf(w, "match_count: %d\n", common.GetInt(data, "match_count"))
	if version := common.GetString(data, "version"); version != "" {
		io.WriteString(w, "version: "+version+"\n")
	}
	fmt.Fprintf(w, "size_bytes_before: %d\n", common.GetInt(data, "size_bytes_before"))
	fmt.Fprintf(w, "size_bytes_after: %d\n", common.GetInt(data, "size_bytes_after"))
}
