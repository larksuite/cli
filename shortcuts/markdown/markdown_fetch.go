// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package markdown

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var MarkdownFetch = common.Shortcut{
	Service:     "markdown",
	Command:     "+fetch",
	Description: "Fetch a Markdown file from Drive",
	Risk:        "read",
	Scopes:      []string{"drive:file:download"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "Markdown file token", Required: true},
		{Name: "output", Desc: "local save path or directory; omit to return content directly"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing local output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		outputPath := strings.TrimSpace(runtime.Str("output"))
		if outputPath == "" {
			return nil
		}
		if _, err := validate.SafeOutputPath(outputPath); err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		dry := common.NewDryRunAPI().
			Desc("download markdown file bytes; when --output is omitted the CLI returns content as UTF-8 text").
			GET("/open-apis/drive/v1/files/:file_token/download").
			Set("file_token", runtime.Str("file-token"))
		if outputPath := strings.TrimSpace(runtime.Str("output")); outputPath != "" {
			dry.Set("output", outputPath)
		} else {
			dry.Set("output", "<stdout>")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := strings.TrimSpace(runtime.Str("file-token"))
		outputPath := strings.TrimSpace(runtime.Str("output"))

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/files/%s/download", validate.EncodePathSegment(fileToken)),
		})
		if err != nil {
			return output.ErrNetwork("download failed: %s", err)
		}
		defer resp.Body.Close()

		fileName := fileNameFromDownloadHeader(resp.Header, fileToken+".md")
		if outputPath == "" {
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				return output.ErrNetwork("download failed: %s", err)
			}
			out := map[string]interface{}{
				"file_token": fileToken,
				"file_name":  fileName,
				"content":    string(payload),
				"size_bytes": len(payload),
			}
			runtime.OutFormatRaw(out, nil, func(w io.Writer) {
				prettyPrintMarkdownContent(w, out)
			})
			return nil
		}

		if markdownFetchOutputIsDirectory(runtime, outputPath) {
			outputPath = filepath.Join(outputPath, fileName)
		}
		if _, statErr := runtime.FileIO().Stat(outputPath); statErr == nil && !runtime.Bool("overwrite") {
			return output.ErrValidation("output file already exists: %s (use --overwrite to replace)", outputPath)
		}

		result, err := runtime.FileIO().Save(outputPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorByCategory(err, "io")
		}

		savedPath, _ := runtime.ResolveSavePath(outputPath)
		if savedPath == "" {
			savedPath = outputPath
		}

		out := map[string]interface{}{
			"file_token": fileToken,
			"file_name":  fileName,
			"saved_path": savedPath,
			"size_bytes": result.Size(),
		}
		runtime.OutFormat(out, nil, func(w io.Writer) {
			prettyPrintMarkdownSavedFile(w, out)
		})
		return nil
	},
}

func markdownFetchOutputIsDirectory(runtime *common.RuntimeContext, outputPath string) bool {
	if strings.HasSuffix(outputPath, "/") || strings.HasSuffix(outputPath, "\\") {
		return true
	}
	info, err := runtime.FileIO().Stat(outputPath)
	return err == nil && info.IsDir()
}
