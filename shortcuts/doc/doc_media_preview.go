// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

var previewMimeToExt = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
	"video/mp4":       ".mp4",
	"text/plain":      ".txt",
}

const PreviewType_SOURCE_FILE = "16"

var DocMediaPreview = common.Shortcut{
	Service:     "docs",
	Command:     "+media-preview",
	Description: "Preview document media file (auto-detects extension)",
	Risk:        "read",
	Scopes:      []string{"docs:document.media:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "media file token", Required: true},
		{Name: "output", Desc: "local save path", Required: true},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		return common.NewDryRunAPI().
			GET("/open-apis/drive/v1/medias/:token/preview_download").
			Desc("Preview document media file").
			Params(map[string]interface{}{"preview_type": PreviewType_SOURCE_FILE}).
			Set("token", token).Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		overwrite := runtime.Bool("overwrite")

		if err := validate.ResourceName(token, "--token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		// Early path validation before API call (final validation after auto-extension below)
		if _, err := validate.SafeOutputPath(outputPath); err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Previewing: media %s\n", common.MaskToken(token))

		encodedToken := validate.EncodePathSegment(token)
		apiPath := fmt.Sprintf("/open-apis/drive/v1/medias/%s/preview_download", encodedToken)

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    apiPath,
			QueryParams: larkcore.QueryParams{
				"preview_type": []string{PreviewType_SOURCE_FILE},
			},
		})
		if err != nil {
			return output.ErrNetwork("preview failed: %v", err)
		}
		defer resp.Body.Close()

		finalPath := outputPath
		currentExt := filepath.Ext(outputPath)
		if currentExt == "" {
			contentType := resp.Header.Get("Content-Type")
			mimeType := strings.Split(contentType, ";")[0]
			mimeType = strings.TrimSpace(mimeType)
			if ext, ok := previewMimeToExt[mimeType]; ok {
				finalPath = outputPath + ext
			}
		}

		safePath, err := validate.SafeOutputPath(finalPath)
		if err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}
		if err := common.EnsureWritableFile(safePath, overwrite); err != nil {
			return err
		}

		if err := vfs.MkdirAll(filepath.Dir(safePath), 0700); err != nil {
			return output.Errorf(output.ExitInternal, "io", "cannot create parent directory: %v", err)
		}

		sizeBytes, err := validate.AtomicWriteFromReader(safePath, resp.Body, 0600)
		if err != nil {
			return output.Errorf(output.ExitInternal, "io", "cannot create file: %v", err)
		}

		runtime.Out(map[string]interface{}{
			"saved_path":   safePath,
			"size_bytes":   sizeBytes,
			"content_type": resp.Header.Get("Content-Type"),
		}, nil)
		return nil
	},
}
