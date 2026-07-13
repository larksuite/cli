// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/validate"
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
		"--folder-token and --wiki-token also accept full Lark URLs and normalize them to the required token.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readMarkdownCreateSpec(runtime)
		if err != nil {
			return err
		}
		return validateMarkdownSpec(runtime, spec, true)
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readMarkdownCreateSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
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
		spec, err := readMarkdownCreateSpec(runtime)
		if err != nil {
			return err
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

func readMarkdownCreateSpec(runtime *common.RuntimeContext) (markdownUploadSpec, error) {
	spec := markdownUploadSpec{
		FileName:    strings.TrimSpace(runtime.Str("name")),
		FolderToken: strings.TrimSpace(runtime.Str("folder-token")),
		WikiToken:   strings.TrimSpace(runtime.Str("wiki-token")),
		FilePath:    strings.TrimSpace(runtime.Str("file")),
		FileSet:     runtime.Changed("file"),
		Content:     runtime.Str("content"),
		ContentSet:  runtime.Changed("content"),
	}
	return normalizeMarkdownCreateTargetSpec(spec)
}

func normalizeMarkdownCreateTargetSpec(spec markdownUploadSpec) (markdownUploadSpec, error) {
	if spec.FolderToken != "" {
		token, err := normalizeMarkdownFolderToken(spec.FolderToken)
		if err != nil {
			return markdownUploadSpec{}, err
		}
		spec.FolderToken = token
	}
	if spec.WikiToken != "" {
		token, err := normalizeMarkdownWikiToken(spec.WikiToken)
		if err != nil {
			return markdownUploadSpec{}, err
		}
		spec.WikiToken = token
	}
	return spec, nil
}

func normalizeMarkdownFolderToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if strings.Contains(token, "://") {
		ref, ok := common.ParseResourceURL(token)
		if !ok {
			return "", markdownValidationParamError("--folder-token", "--folder-token URL is unsupported").
				WithHint("Pass a Drive folder URL or raw folder token.")
		}
		if ref.Type != "folder" {
			return "", markdownValidationParamError("--folder-token",
				"--folder-token must identify a Drive folder; got a %s URL",
				ref.Type,
			).WithHint("Use --wiki-token for wiki nodes or pass a Drive folder URL/token.")
		}
		if err := validateMarkdownTargetTokenName(ref.Token, "--folder-token"); err != nil {
			return "", err
		}
		return ref.Token, nil
	}
	if err := rejectMarkdownPartialToken(token, "--folder-token"); err != nil {
		return "", err
	}
	switch markdownKnownResourceTokenKind(token) {
	case "wiki":
		return "", markdownValidationParamError("--folder-token", "--folder-token looks like a wiki node token").
			WithHint("Pass it with --wiki-token instead.")
	case "doc", "docx", "sheet", "bitable", "mindnote", "slides", "file":
		return "", markdownValidationParamError("--folder-token", "--folder-token must be a Drive folder token, not a %s token", markdownKnownResourceTokenKind(token))
	}
	if err := validateMarkdownTargetTokenName(token, "--folder-token"); err != nil {
		return "", err
	}
	return token, nil
}

func normalizeMarkdownWikiToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if strings.Contains(token, "://") {
		ref, ok := common.ParseResourceURL(token)
		if !ok {
			return "", markdownValidationParamError("--wiki-token", "--wiki-token URL is unsupported").
				WithHint("Pass a wiki node URL or raw wiki node token.")
		}
		if ref.Type != "wiki" {
			return "", markdownValidationParamError("--wiki-token",
				"--wiki-token must identify a wiki node; got a %s URL",
				ref.Type,
			).WithHint("Resolve document URLs with `lark-cli wiki +node-get --node-token <url>` and use the returned node_token.")
		}
		if err := validateMarkdownTargetTokenName(ref.Token, "--wiki-token"); err != nil {
			return "", err
		}
		return ref.Token, nil
	}
	if err := rejectMarkdownPartialToken(token, "--wiki-token"); err != nil {
		return "", err
	}
	if kind := markdownKnownResourceTokenKind(token); kind != "" && kind != "wiki" {
		return "", markdownValidationParamError("--wiki-token", "--wiki-token must be a wiki node token, not a %s token", kind)
	}
	if err := validateMarkdownTargetTokenName(token, "--wiki-token"); err != nil {
		return "", err
	}
	return token, nil
}

func rejectMarkdownPartialToken(token, flagName string) error {
	if strings.ContainsAny(token, "/?#") {
		return markdownValidationParamError(flagName, "%s must be a raw token, not a path, query, or fragment", flagName).
			WithHint("Pass a full Lark URL, or copy only the token value without path/query/fragment characters.")
	}
	return nil
}

func validateMarkdownTargetTokenName(token, flagName string) error {
	if err := validate.ResourceName(token, flagName); err != nil {
		return markdownValidationParamError(flagName, "%s", err).WithCause(err)
	}
	return nil
}

func markdownKnownResourceTokenKind(token string) string {
	lower := strings.ToLower(strings.TrimSpace(token))
	switch {
	case strings.HasPrefix(lower, "wik"):
		return "wiki"
	case strings.HasPrefix(lower, "docx"):
		return "docx"
	case strings.HasPrefix(lower, "doc"):
		return "doc"
	case strings.HasPrefix(lower, "sht"):
		return "sheet"
	case strings.HasPrefix(lower, "bas"):
		return "bitable"
	case strings.HasPrefix(lower, "mn"):
		return "mindnote"
	case strings.HasPrefix(lower, "sld"):
		return "slides"
	case strings.HasPrefix(lower, "box"), strings.HasPrefix(lower, "file"):
		return "file"
	default:
		return ""
	}
}
