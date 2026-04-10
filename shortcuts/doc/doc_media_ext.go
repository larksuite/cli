// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type docMediaExtensionResolution struct {
	Ext    string
	Source string
	Detail string
}

var docMediaMimeToExt = map[string]string{
	"application/msword":            ".doc",
	"application/pdf":               ".pdf",
	"application/vnd.ms-excel":      ".xls",
	"application/vnd.ms-powerpoint": ".ppt",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": ".pptx",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         ".xlsx",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   ".docx",
	"application/xml": ".xml",
	"application/zip": ".zip",
	"image/bmp":       ".bmp",
	"image/gif":       ".gif",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/svg+xml":   ".svg",
	"image/webp":      ".webp",
	"text/csv":        ".csv",
	"text/html":       ".html",
	"text/plain":      ".txt",
	"text/xml":        ".xml",
	"video/mp4":       ".mp4",
}

func autoAppendDocMediaExtension(outputPath string, header http.Header, fallbackExt string) (string, *docMediaExtensionResolution) {
	if docMediaHasExplicitExtension(outputPath) {
		return outputPath, nil
	}
	normalizedPath := outputPath
	if filepath.Ext(outputPath) == "." {
		normalizedPath = strings.TrimSuffix(outputPath, ".")
	}
	if resolution := docMediaExtensionByContentType(header.Get("Content-Type")); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if resolution := docMediaExtensionByContentDisposition(header); resolution != nil {
		return normalizedPath + resolution.Ext, resolution
	}
	if fallbackExt != "" {
		return normalizedPath + fallbackExt, &docMediaExtensionResolution{
			Ext:    fallbackExt,
			Source: "fallback",
			Detail: "default fallback",
		}
	}
	return outputPath, nil
}

func docMediaHasExplicitExtension(path string) bool {
	ext := filepath.Ext(path)
	return ext != "" && ext != "."
}

func docMediaExtensionByContentType(contentType string) *docMediaExtensionResolution {
	if contentType == "" {
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	if ext, ok := docMediaMimeToExt[strings.ToLower(mediaType)]; ok {
		return &docMediaExtensionResolution{
			Ext:    ext,
			Source: "Content-Type",
			Detail: contentType,
		}
	}
	return nil
}

func docMediaExtensionByContentDisposition(header http.Header) *docMediaExtensionResolution {
	filename := strings.TrimSpace(larkcore.FileNameByHeader(header))
	if filename == "" {
		return nil
	}
	ext := filepath.Ext(filename)
	if ext == "" || ext == "." {
		return nil
	}
	return &docMediaExtensionResolution{
		Ext:    ext,
		Source: "Content-Disposition",
		Detail: filename,
	}
}
