// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var alignMap = map[string]int{
	"left":   1,
	"center": 2,
	"right":  3,
}

// readClipboardImage is swappable in tests so callers do not depend on the
// host pasteboard.
var readClipboardImage = readClipboardImageBytes

func blockTypeForMediaType(mediaType string) int {
	if mediaType == "file" {
		return 23
	}
	return 27
}

func parentTypeForMediaType(mediaType string) string {
	if mediaType == "file" {
		return "docx_file"
	}
	return "docx_image"
}

func resolveDocxDocumentID(runtime *common.RuntimeContext, input string) (string, error) {
	docRef, err := parseDocumentRef(input)
	if err != nil {
		return "", err
	}

	switch docRef.Kind {
	case "docx":
		return docRef.Token, nil
	case "doc":
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "this document operation only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx").WithParam("--doc")
	case "wiki":
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(docRef.Token))
		data, err := runtime.CallAPITyped(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": docRef.Token},
			nil,
		)
		if err != nil {
			return "", err
		}

		node := common.GetMap(data, "node")
		objType := common.GetString(node, "obj_type")
		objToken := common.GetString(node, "obj_token")
		if objType == "" || objToken == "" {
			return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data")
		}
		if objType != "docx" {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "wiki resolved to %q, but this document operation only supports docx documents", objType).WithParam("--doc")
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to docx: %s\n", common.MaskToken(objToken))
		return objToken, nil
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "this document operation only supports docx documents").WithParam("--doc")
	}
}

func detectImageDimensions(r io.Reader) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func detectImageDimensionsFromPath(fio fileio.FileIO, filePath string) (int, int, error) {
	width, height, _, err := detectImageConfigFromPath(fio, filePath)
	return width, height, err
}

func detectImageConfigFromPath(fio fileio.FileIO, filePath string) (int, int, string, error) {
	if _, err := validate.SafeInputPath(filePath); err != nil {
		return 0, 0, "", err
	}
	f, err := fio.Open(filePath)
	if err != nil {
		return 0, 0, "", err
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, "", err
	}
	return cfg.Width, cfg.Height, format, nil
}
