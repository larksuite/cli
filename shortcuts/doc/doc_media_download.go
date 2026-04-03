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

var mimeToExt = map[string]string{
	"image/png":       ".png",
	"image/jpeg":      ".jpg",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"image/svg+xml":   ".svg",
	"application/pdf": ".pdf",
	"video/mp4":       ".mp4",
	"text/plain":      ".txt",
}

var DocMediaDownload = common.Shortcut{
	Service:     "docs",
	Command:     "+media-download",
	Description: "Download document media or whiteboard thumbnail (auto-detects extension)",
	Risk:        "read",
	Scopes:      []string{"docs:document.media:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "resource token (file_token or whiteboard_id)", Required: true},
		{Name: "output", Desc: "local save path", Required: true},
		{Name: "type", Default: "media", Desc: "resource type: media (default) | whiteboard"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		mediaType := runtime.Str("type")
		if mediaType == "whiteboard" {
			return common.NewDryRunAPI().
				GET("/open-apis/board/v1/whiteboards/:token/download_as_image").
				Desc("(when --type=whiteboard) Download whiteboard as image").
				Set("token", token).Set("output", outputPath)
		}
		return common.NewDryRunAPI().
			GET("/open-apis/drive/v1/medias/:token/download").
			Desc("(when --type=media) Download document media file").
			Set("token", token).Set("output", outputPath)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token := runtime.Str("token")
		outputPath := runtime.Str("output")
		mediaType := runtime.Str("type")
		overwrite := runtime.Bool("overwrite")

		if err := validate.ResourceName(token, "--token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		// Early path validation before API call (final validation after auto-extension below)
		if _, err := validate.SafeOutputPath(outputPath); err != nil {
			return output.ErrValidation("unsafe output path: %s", err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Downloading: %s %s\n", mediaType, common.MaskToken(token))

		// Build API URL
		encodedToken := validate.EncodePathSegment(token)
		var apiPath string
		if mediaType == "whiteboard" {
			apiPath = fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", encodedToken)
		} else {
			apiPath = fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", encodedToken)
		}

		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    apiPath,
		})
		if err != nil {
			return output.ErrNetwork("download failed: %v", err)
		}
		defer resp.Body.Close()

		// Auto-detect extension from Content-Type
		finalPath := outputPath
		currentExt := filepath.Ext(outputPath)
		if currentExt == "" {
			contentType := resp.Header.Get("Content-Type")
			mimeType := strings.Split(contentType, ";")[0]
			mimeType = strings.TrimSpace(mimeType)
			if ext, ok := mimeToExt[mimeType]; ok {
				finalPath = outputPath + ext
			} else if mediaType == "whiteboard" {
				finalPath = outputPath + ".png"
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
