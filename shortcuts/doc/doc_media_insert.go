// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var alignMap = map[string]int{
	"left":   1,
	"center": 2,
	"right":  3,
}

// readClipboardImage is the clipboard read function, swappable in tests to
// inject synthetic image bytes without depending on the host pasteboard.
var readClipboardImage = readClipboardImageBytes

// fileViewMap maps the user-facing --file-view value to the docx File block
// `view_type` enum. The underlying values come from the open platform spec:
//
//	1 = card view (default)
//	2 = preview view (renders audio/video files as an inline player)
//	3 = inline view
var fileViewMap = map[string]int{
	"card":    1,
	"preview": 2,
	"inline":  3,
}

var DocMediaInsert = common.Shortcut{
	Service:     "docs",
	Command:     "+media-insert",
	Description: "Insert a local image or file into a Lark document (4-step orchestration + auto-rollback); appends to end by default, or inserts relative to a text selection with --selection-with-ellipsis",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload", "docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)"},
		{Name: "from-clipboard", Type: "bool", Desc: "read image from system clipboard instead of a local file (macOS/Windows built-in; Linux requires xclip, xsel or wl-paste)"},
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: "image", Desc: "type: image | file"},
		{Name: "align", Desc: "alignment: left | center | right"},
		{Name: "caption", Desc: "image caption text"},
		{Name: "selection-with-ellipsis", Desc: "plain text (or 'start...end' to disambiguate) matching the target block's content. Media is inserted at the top-level ancestor of the matched block — i.e., when the selection is inside a callout, table cell, or nested list, media lands outside that container, not inside it. Pass 'start...end' (a unique prefix and suffix separated by '...') when the plain text appears in more than one block"},
		{Name: "before", Type: "bool", Desc: "insert before the matched block instead of after (requires --selection-with-ellipsis)"},
		{Name: "file-view", Desc: "file block rendering: card (default) | preview | inline; only applies when --type=file. preview renders audio/video as an inline player"},
		{Name: "width", Type: "int", Desc: "image display width in pixels (only for --type=image); if --height is omitted it is auto-computed from the source image aspect ratio"},
		{Name: "height", Type: "int", Desc: "image display height in pixels (only for --type=image); if --width is omitted it is auto-computed from the source image aspect ratio"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		fromClipboard := runtime.Bool("from-clipboard")
		if filePath == "" && !fromClipboard {
			return common.FlagErrorf("one of --file or --from-clipboard is required")
		}
		if filePath != "" && fromClipboard {
			return common.FlagErrorf("--file and --from-clipboard are mutually exclusive")
		}

		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return err
		}
		if docRef.Kind == "doc" {
			return output.ErrValidation("docs +media-insert only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx")
		}
		rawSelection := runtime.Str("selection-with-ellipsis")
		trimmedSelection := strings.TrimSpace(rawSelection)
		// Explicitly reject a flag that was supplied but blank: runtime.Str cannot
		// distinguish "omitted" from "provided as empty/whitespace", and a silent
		// trim-to-empty would make +media-insert fall back to append-mode and
		// write at the wrong location.
		if rawSelection != "" && trimmedSelection == "" {
			return output.ErrValidation("--selection-with-ellipsis must not be blank or whitespace-only")
		}
		if runtime.Bool("before") && trimmedSelection == "" {
			return output.ErrValidation("--before requires --selection-with-ellipsis")
		}
		if view := runtime.Str("file-view"); view != "" {
			if _, ok := fileViewMap[view]; !ok {
				return output.ErrValidation("invalid --file-view value %q, expected one of: card | preview | inline", view)
			}
			if runtime.Str("type") != "file" {
				return output.ErrValidation("--file-view only applies when --type=file")
			}
		}
		widthChanged := runtime.Changed("width")
		heightChanged := runtime.Changed("height")
		if (widthChanged || heightChanged) && runtime.Str("type") != "image" {
			return output.ErrValidation("--width/--height only apply when --type=image")
		}
		if widthChanged && runtime.Int("width") <= 0 {
			return output.ErrValidation("--width must be a positive integer")
		}
		if heightChanged && runtime.Int("height") <= 0 {
			return output.ErrValidation("--height must be a positive integer")
		}
		const maxDimension = 10000
		if widthChanged && runtime.Int("width") > maxDimension {
			return output.ErrValidation("--width must not exceed %d pixels", maxDimension)
		}
		if heightChanged && runtime.Int("height") > maxDimension {
			return output.ErrValidation("--height must not exceed %d pixels", maxDimension)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		documentID := docRef.Token
		stepBase := 1
		filePath := runtime.Str("file")
		if runtime.Bool("from-clipboard") {
			filePath = "<clipboard image>"
		}
		mediaType := runtime.Str("type")
		caption := runtime.Str("caption")
		selection := strings.TrimSpace(runtime.Str("selection-with-ellipsis"))
		hasSelection := selection != ""
		fileViewType := fileViewMap[runtime.Str("file-view")]

		parentType := parentTypeForMediaType(mediaType)
		createBlockData := buildCreateBlockData(mediaType, 0, fileViewType)
		if hasSelection {
			createBlockData["index"] = "<locate_index>"
		} else {
			createBlockData["index"] = "<children_len>"
		}
		// Best-effort dimension computation for dry-run.
		dryWidth := runtime.Int("width")
		dryHeight := runtime.Int("height")
		widthChanged := runtime.Changed("width")
		heightChanged := runtime.Changed("height")

		if (widthChanged || heightChanged) && !(widthChanged && heightChanged) {
			if filePath == "<clipboard image>" {
				fmt.Fprintf(runtime.IO().ErrOut, "Note: cannot detect clipboard image dimensions in dry-run; provide both --width and --height for accurate preview\n")
			} else if nativeW, nativeH, err := detectImageDimensionsFromPath(runtime.FileIO(), filePath); err == nil {
				dims := computeMissingDimension(dryWidth, dryHeight, nativeW, nativeH)
				dryWidth = dims.width
				dryHeight = dims.height
			} else {
				fmt.Fprintf(runtime.IO().ErrOut, "Note: unable to detect image dimensions from %s; provide both --width and --height to avoid failure at execution time\n", filePath)
			}
		}

		batchUpdateData := buildBatchUpdateData("<new_block_id>", mediaType, "<file_token>", runtime.Str("align"), caption, dryWidth, dryHeight)

		d := common.NewDryRunAPI()
		totalSteps := 4
		if docRef.Kind == "wiki" {
			totalSteps++
		}
		if hasSelection {
			totalSteps++
		}

		positionLabel := map[bool]string{true: "before", false: "after"}[runtime.Bool("before")]

		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			stepBase = 2
			d.Desc(fmt.Sprintf("%d-step orchestration: resolve wiki → query root →%s create block → upload file → bind to block (auto-rollback on failure)",
				totalSteps, map[bool]string{true: " locate-doc →", false: ""}[hasSelection])).
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		} else {
			d.Desc(fmt.Sprintf("%d-step orchestration: query root →%s create block → upload file → bind to block (auto-rollback on failure)",
				totalSteps, map[bool]string{true: " locate-doc →", false: ""}[hasSelection]))
		}

		d.
			GET("/open-apis/docx/v1/documents/:document_id/blocks/:document_id").
			Desc(fmt.Sprintf("[%d] Get document root block", stepBase))

		if hasSelection {
			mcpEndpoint := common.MCPEndpoint(runtime.Config.Brand)
			mcpArgs := map[string]interface{}{
				"doc_id":                  documentID,
				"selection_with_ellipsis": selection,
				"limit":                   1,
			}
			d.POST(mcpEndpoint).
				Desc(fmt.Sprintf("[%d] MCP locate-doc: find block matching selection (%s)", stepBase+1, positionLabel)).
				Body(map[string]interface{}{
					"method": "tools/call",
					"params": map[string]interface{}{
						"name":      "locate-doc",
						"arguments": mcpArgs,
					},
				}).
				Set("mcp_tool", "locate-doc").
				Set("args", mcpArgs)
			stepBase++
		}

		d.
			POST("/open-apis/docx/v1/documents/:document_id/blocks/:document_id/children").
			Desc(fmt.Sprintf("[%d] Create empty block at target position", stepBase+1)).
			Body(createBlockData)
		appendDocMediaInsertUploadDryRun(d, runtime.FileIO(), filePath, parentType, stepBase+2)
		d.PATCH("/open-apis/docx/v1/documents/:document_id/blocks/batch_update").
			Desc(fmt.Sprintf("[%d] Bind uploaded file token to the new block", stepBase+3)).
			Body(batchUpdateData)

		d.Set("document_id", documentID)
		// Annotate dry-run when reading from the clipboard: DryRun never touches
		// the pasteboard, so it cannot tell in advance whether the payload is
		// above or below the 20MB single-part threshold. Execute will make the
		// real decision once it reads the bytes.
		if runtime.Bool("from-clipboard") {
			d.Set("upload_size_note", "clipboard size unknown; single-part vs multipart decision deferred to runtime")
		}
		if runtime.Bool("from-clipboard") && (widthChanged || heightChanged) && !(widthChanged && heightChanged) {
			d.Set("dimension_note", "clipboard dimensions unknown; aspect-ratio calculation deferred to runtime")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		docInput := runtime.Str("doc")
		mediaType := runtime.Str("type")
		alignStr := runtime.Str("align")
		caption := runtime.Str("caption")
		fileViewType := fileViewMap[runtime.Str("file-view")]

		// Clipboard path: read image bytes into memory, bypassing FileIO path validation.
		var clipboardContent []byte
		if runtime.Bool("from-clipboard") {
			fmt.Fprintf(runtime.IO().ErrOut, "Reading image from clipboard...\n")
			var err error
			clipboardContent, err = readClipboardImage()
			if err != nil {
				return err
			}
		}

		documentID, err := resolveDocxDocumentID(runtime, docInput)
		if err != nil {
			return err
		}

		// Determine file size and name.
		var fileSize int64
		var fileName string
		if clipboardContent != nil {
			fileSize = int64(len(clipboardContent))
			fileName = "clipboard.png"
		} else {
			stat, err := runtime.FileIO().Stat(filePath)
			if err != nil {
				return common.WrapInputStatError(err, "file not found")
			}
			if !stat.Mode().IsRegular() {
				return output.ErrValidation("file must be a regular file: %s", filePath)
			}
			fileSize = stat.Size()
			fileName = filepath.Base(filePath)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Inserting: %s -> document %s\n", fileName, common.MaskToken(documentID))
		if fileSize > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}

		// Step 1: Get document root block to find where to insert
		rootData, err := runtime.CallAPI("GET",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", validate.EncodePathSegment(documentID), validate.EncodePathSegment(documentID)),
			nil, nil)
		if err != nil {
			return err
		}

		parentBlockID, insertIndex, rootChildren, err := extractAppendTarget(rootData, documentID)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Root block ready: %s (%d children)\n", parentBlockID, insertIndex)

		selection := strings.TrimSpace(runtime.Str("selection-with-ellipsis"))
		if selection != "" {
			before := runtime.Bool("before")
			// Redact the selection when logging — it is copied verbatim from
			// document content and may contain confidential text.
			fmt.Fprintf(runtime.IO().ErrOut, "Locating block matching selection (%s)\n", redactSelection(selection))
			idx, err := locateInsertIndex(runtime, documentID, selection, rootChildren, before)
			if err != nil {
				return err
			}
			insertIndex = idx
			posLabel := "after"
			if before {
				posLabel = "before"
			}
			fmt.Fprintf(runtime.IO().ErrOut, "locate-doc matched: inserting %s at index %d\n", posLabel, insertIndex)
		}

		// Step 2: Create an empty block at the target position
		fmt.Fprintf(runtime.IO().ErrOut, "Creating block at index %d\n", insertIndex)

		createData, err := runtime.CallAPI("POST",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children", validate.EncodePathSegment(documentID), validate.EncodePathSegment(parentBlockID)),
			nil, buildCreateBlockData(mediaType, insertIndex, fileViewType))
		if err != nil {
			return err
		}

		blockId, uploadParentNode, replaceBlockID := extractCreatedBlockTargets(createData, mediaType)

		if blockId == "" {
			return output.Errorf(output.ExitAPI, "api_error", "failed to create block: no block_id returned")
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Block created: %s\n", blockId)
		if uploadParentNode != blockId || replaceBlockID != blockId {
			fmt.Fprintf(runtime.IO().ErrOut, "Resolved file block targets: upload=%s replace=%s\n", uploadParentNode, replaceBlockID)
		}

		// The placeholder block is created before any upload starts, so failures in
		// later steps should try to remove it instead of leaving an empty artifact.
		rollback := func() error {
			fmt.Fprintf(runtime.IO().ErrOut, "Rolling back: deleting block %s\n", blockId)
			_, err := runtime.CallAPI("DELETE",
				fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s/children/batch_delete", validate.EncodePathSegment(documentID), validate.EncodePathSegment(parentBlockID)),
				nil, buildDeleteBlockData(insertIndex))
			return err
		}
		withRollbackWarning := func(opErr error) error {
			rollbackErr := rollback()
			if rollbackErr == nil {
				return opErr
			}
			warning := fmt.Sprintf("rollback failed for block %s: %v", blockId, rollbackErr)
			fmt.Fprintf(runtime.IO().ErrOut, "warning: %s\n", warning)
			return opErr
		}

		// Step 3: Upload media file.
		// Only materialize Content when clipboard bytes exist, so the `io.Reader`
		// interface stays a true nil for the --file path. Passing a typed-nil
		// *bytes.Reader here would make the downstream `if cfg.Content != nil`
		// check incorrectly take the clipboard branch and crash on Read.
		// Resolve display dimensions before upload to fail fast on unreadable images.
		var finalWidth, finalHeight int
		if mediaType == "image" {
			userWidth := runtime.Int("width")
			userHeight := runtime.Int("height")
			widthChanged := runtime.Changed("width")
			heightChanged := runtime.Changed("height")

			if widthChanged && heightChanged {
				finalWidth = userWidth
				finalHeight = userHeight
			} else if widthChanged || heightChanged {
				var nativeW, nativeH int
				var dimErr error
				if clipboardContent != nil {
					nativeW, nativeH, dimErr = detectImageDimensions(bytes.NewReader(clipboardContent))
				} else {
					f, openErr := runtime.FileIO().Open(filePath)
					if openErr != nil {
						return withRollbackWarning(output.ErrValidation(
							"unable to detect image dimensions from %s for aspect-ratio calculation; provide both --width and --height", fileName))
					}
					nativeW, nativeH, dimErr = detectImageDimensions(f)
					f.Close()
				}
				if dimErr != nil {
					return withRollbackWarning(output.ErrValidation(
						"unable to detect image dimensions from %s for aspect-ratio calculation; provide both --width and --height", fileName))
				}
				dims := computeMissingDimension(userWidth, userHeight, nativeW, nativeH)
				finalWidth = dims.width
				finalHeight = dims.height
				fmt.Fprintf(runtime.IO().ErrOut, "Image dimensions: %dx%d (native: %dx%d)\n", finalWidth, finalHeight, nativeW, nativeH)
			}
		}

		uploadCfg := UploadDocMediaFileConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: parentTypeForMediaType(mediaType),
			ParentNode: uploadParentNode,
			DocID:      documentID,
		}
		if clipboardContent != nil {
			uploadCfg.Reader = bytes.NewReader(clipboardContent)
		}
		fileToken, err := uploadDocMediaFile(runtime, uploadCfg)
		if err != nil {
			return withRollbackWarning(err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "File uploaded: %s\n", fileToken)

		// Step 4: Bind file token to block via batch_update
		fmt.Fprintf(runtime.IO().ErrOut, "Binding uploaded media to block %s\n", replaceBlockID)

		if _, err := runtime.CallAPI("PATCH",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", validate.EncodePathSegment(documentID)),
			nil, buildBatchUpdateData(replaceBlockID, mediaType, fileToken, alignStr, caption, finalWidth, finalHeight)); err != nil {
			return withRollbackWarning(err)
		}

		outData := map[string]interface{}{
			"document_id": documentID,
			"block_id":    blockId,
			"file_token":  fileToken,
			"type":        mediaType,
		}
		if finalWidth > 0 {
			outData["width"] = finalWidth
		}
		if finalHeight > 0 {
			outData["height"] = finalHeight
		}
		runtime.Out(outData, nil)
		return nil
	},
}

func blockTypeForMediaType(mediaType string) int {
	if mediaType == "file" {
		return 23
	}
	return 27
}

// redactSelection summarizes --selection-with-ellipsis values for logging and
// error messages without echoing raw document text. Returns the rune count and,
// for longer strings, a short prefix so operators can still identify which
// selection failed without leaking confidential content into terminals or CI
// logs.
func redactSelection(s string) string {
	const prefixRunes = 8
	runes := []rune(s)
	if len(runes) <= prefixRunes {
		return fmt.Sprintf("%d chars", len(runes))
	}
	return fmt.Sprintf("%q… %d chars total", string(runes[:prefixRunes]), len(runes))
}

func parentTypeForMediaType(mediaType string) string {
	if mediaType == "file" {
		return "docx_file"
	}
	return "docx_image"
}

func buildCreateBlockData(mediaType string, index int, fileViewType int) map[string]interface{} {
	child := map[string]interface{}{
		"block_type": blockTypeForMediaType(mediaType),
	}
	if mediaType == "file" {
		fileData := map[string]interface{}{}
		// view_type can only be set at block creation time; the PATCH
		// replace_file endpoint does not accept it, so if the caller wants
		// preview/inline rendering we must wire it in here. Whitelist the
		// concrete enum values so a stray positive int cannot produce a
		// malformed payload if Validate is ever bypassed.
		switch fileViewType {
		case 1, 2, 3:
			fileData["view_type"] = fileViewType
		}
		child["file"] = fileData
	} else {
		child["image"] = map[string]interface{}{}
	}
	return map[string]interface{}{
		"children": []interface{}{
			child,
		},
		"index": index,
	}
}

func buildDeleteBlockData(index int) map[string]interface{} {
	return map[string]interface{}{
		"start_index": index,
		"end_index":   index + 1,
	}
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
		return "", output.ErrValidation("docs +media-insert only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx")
	case "wiki":
		fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node: %s\n", common.MaskToken(docRef.Token))
		data, err := runtime.CallAPI(
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
			return "", output.Errorf(output.ExitAPI, "api_error", "wiki get_node returned incomplete node data")
		}
		if objType != "docx" {
			return "", output.ErrValidation("wiki resolved to %q, but docs +media-insert only supports docx documents", objType)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to docx: %s\n", common.MaskToken(objToken))
		return objToken, nil
	default:
		return "", output.ErrValidation("docs +media-insert only supports docx documents")
	}
}

type imageDimensions struct {
	width  int
	height int
}

func computeMissingDimension(userWidth, userHeight, nativeWidth, nativeHeight int) imageDimensions {
	if nativeWidth <= 0 || nativeHeight <= 0 {
		return imageDimensions{width: userWidth, height: userHeight}
	}
	if userWidth > 0 && userHeight == 0 {
		return imageDimensions{
			width:  userWidth,
			height: (userWidth*nativeHeight + nativeWidth/2) / nativeWidth,
		}
	}
	if userHeight > 0 && userWidth == 0 {
		return imageDimensions{
			width:  (userHeight*nativeWidth + nativeHeight/2) / nativeHeight,
			height: userHeight,
		}
	}
	return imageDimensions{width: userWidth, height: userHeight}
}

func detectImageDimensions(r io.Reader) (width, height int, err error) {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}

func detectImageDimensionsFromPath(fio fileio.FileIO, filePath string) (int, int, error) {
	if _, err := validate.SafeInputPath(filePath); err != nil {
		return 0, 0, err
	}
	f, err := fio.Open(filePath)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	return detectImageDimensions(f)
}

func buildBatchUpdateData(blockID, mediaType, fileToken, alignStr, caption string, width, height int) map[string]interface{} {
	request := map[string]interface{}{
		"block_id": blockID,
	}
	if mediaType == "file" {
		request["replace_file"] = map[string]interface{}{
			"token": fileToken,
		}
	} else {
		replaceImage := map[string]interface{}{
			"token": fileToken,
		}
		if width > 0 {
			replaceImage["width"] = width
		}
		if height > 0 {
			replaceImage["height"] = height
		}
		if alignVal, ok := alignMap[alignStr]; ok {
			replaceImage["align"] = alignVal
		}
		if caption != "" {
			replaceImage["caption"] = map[string]interface{}{
				"content": caption,
			}
		}
		request["replace_image"] = replaceImage
	}
	return map[string]interface{}{
		"requests": []interface{}{request},
	}
}

func extractAppendTarget(rootData map[string]interface{}, fallbackBlockID string) (parentBlockID string, insertIndex int, children []interface{}, err error) {
	block, _ := rootData["block"].(map[string]interface{})
	if len(block) == 0 {
		return "", 0, nil, output.Errorf(output.ExitAPI, "api_error", "failed to query document root block")
	}

	parentBlockID = fallbackBlockID
	if blockID, _ := block["block_id"].(string); blockID != "" {
		parentBlockID = blockID
	}

	children, _ = block["children"].([]interface{})
	return parentBlockID, len(children), children, nil
}

// locateInsertIndex uses the MCP locate-doc tool to find the root-level index
// at which to insert relative to the block matching selection. It walks the
// parent_id chain (using single-block GET calls when needed) to resolve nested
// blocks to their top-level ancestor in rootChildren.
func locateInsertIndex(runtime *common.RuntimeContext, documentID string, selection string, rootChildren []interface{}, before bool) (int, error) {
	// Ask for 2 matches so we can warn when the selection is ambiguous. locate-doc
	// orders matches by document position, so matches[0] is still deterministic.
	args := map[string]interface{}{
		"doc_id":                  documentID,
		"selection_with_ellipsis": selection,
		"limit":                   2,
	}
	result, err := common.CallMCPTool(runtime, "locate-doc", args)
	if err != nil {
		return 0, err
	}

	matches := common.GetSlice(result, "matches")
	if len(matches) == 0 {
		return 0, output.ErrWithHint(
			output.ExitValidation,
			"no_match",
			fmt.Sprintf("locate-doc did not find any block matching selection (%s)", redactSelection(selection)),
			"check spelling or use 'start...end' syntax to narrow the selection",
		)
	}
	if len(matches) > 1 {
		// Silently picking the first match surprises users whose selection appears
		// in more than one block (e.g. the same phrase in a title and a paragraph).
		// Surface that another match exists and point at the 'start...end' disambiguator.
		fmt.Fprintf(runtime.IO().ErrOut,
			"warning: selection (%s) matched more than one block; inserting relative to the first. "+
				"Pass --selection-with-ellipsis 'start...end' to narrow.\n",
			redactSelection(selection))
	}

	matchMap, _ := matches[0].(map[string]interface{})
	anchorBlockID := common.GetString(matchMap, "anchor_block_id")
	if anchorBlockID == "" {
		// Fall back to first block entry if anchor_block_id is absent.
		blocks := common.GetSlice(matchMap, "blocks")
		if len(blocks) > 0 {
			if b, ok := blocks[0].(map[string]interface{}); ok {
				anchorBlockID = common.GetString(b, "block_id")
			}
		}
	}
	if anchorBlockID == "" {
		return 0, output.Errorf(output.ExitAPI, "api_error", "locate-doc response missing anchor_block_id")
	}
	parentBlockID := common.GetString(matchMap, "parent_block_id")

	// Build root children set for O(1) lookup.
	rootSet := make(map[string]int, len(rootChildren))
	for i, c := range rootChildren {
		if id, ok := c.(string); ok {
			rootSet[id] = i
		}
	}

	// Walk up the parent chain to the top-level ancestor in rootChildren. This
	// is serial by nature: each level's parent_id is only known after the
	// previous level's GET /blocks/{id} response arrives, so the calls cannot
	// be batched or parallelised.
	//
	// visited is the real cycle guard — it stops an A→B→A parent-id loop (seen
	// on malformed API responses) after one lap. maxDepth is belt-and-suspenders
	// in case both visited tracking and parent_id sanity simultaneously break;
	// 32 comfortably exceeds the deepest real docx nesting (~6–8 levels for
	// quote/callout/list combinations) without letting a bug run unbounded.
	cur := anchorBlockID
	nextParent := parentBlockID
	visited := map[string]bool{}
	const maxDepth = 32
	walkDepth := 0
	for depth := 0; depth < maxDepth; depth++ {
		if visited[cur] {
			break
		}
		visited[cur] = true

		if idx, ok := rootSet[cur]; ok {
			if walkDepth > 0 {
				// The anchor was nested inside a callout / table cell / list and
				// got resolved to its top-level ancestor. Surface this so users
				// don't misread "insert before 'X'" as "insert right next to X"
				// when X is buried several levels deep.
				posLabel := "after"
				if before {
					posLabel = "before"
				}
				fmt.Fprintf(runtime.IO().ErrOut,
					"note: selection (%s) was nested %d level(s) deep; inserting %s its top-level ancestor at index %d\n",
					redactSelection(selection), walkDepth, posLabel, idx)
			}
			if before {
				return idx, nil
			}
			return idx + 1, nil
		}

		// Advance: use the parent hint we already have, or fetch from API.
		parent := nextParent
		nextParent = "" // clear hint after first use
		if parent == "" || parent == cur {
			// Need to fetch this block to find its parent.
			data, err := runtime.CallAPI("GET",
				fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s",
					validate.EncodePathSegment(documentID), validate.EncodePathSegment(cur)),
				nil, nil)
			if err != nil {
				return 0, err
			}
			block := common.GetMap(data, "block")
			parent = common.GetString(block, "parent_id")
		}
		if parent == "" || parent == cur {
			break
		}
		cur = parent
		walkDepth++
	}

	return 0, output.ErrWithHint(
		output.ExitValidation,
		"block_not_reachable",
		fmt.Sprintf("block matching selection (%s) is not reachable from document root", redactSelection(selection)),
		"try a top-level heading or paragraph as the selection",
	)
}

func extractCreatedBlockTargets(createData map[string]interface{}, mediaType string) (blockID, uploadParentNode, replaceBlockID string) {
	children, _ := createData["children"].([]interface{})
	if len(children) == 0 {
		return "", "", ""
	}

	child, _ := children[0].(map[string]interface{})
	blockID, _ = child["block_id"].(string)
	uploadParentNode = blockID
	replaceBlockID = blockID

	if mediaType != "file" {
		return blockID, uploadParentNode, replaceBlockID
	}

	// File blocks are wrapped: the created top-level block owns a nested child
	// that is both the upload target and the replace_file target.
	nestedChildren, _ := child["children"].([]interface{})
	if len(nestedChildren) == 0 {
		return blockID, uploadParentNode, replaceBlockID
	}
	if nestedBlockID, ok := nestedChildren[0].(string); ok && nestedBlockID != "" {
		uploadParentNode = nestedBlockID
		replaceBlockID = nestedBlockID
	}
	return blockID, uploadParentNode, replaceBlockID
}

func appendDocMediaInsertUploadDryRun(d *common.DryRunAPI, fio fileio.FileIO, filePath, parentType string, step int) {
	// The upload step runs only after the empty placeholder block is created, so
	// dry-run can refer to that future block ID only symbolically. For large
	// files, keep multipart internals as substeps of the single user-facing
	// "upload file" step.
	if docMediaShouldUseMultipart(fio, filePath) {
		d.POST("/open-apis/drive/v1/medias/upload_prepare").
			Desc(fmt.Sprintf("[%da] Initialize multipart upload", step)).
			Body(map[string]interface{}{
				"file_name":   filepath.Base(filePath),
				"parent_type": parentType,
				"parent_node": "<new_block_id>",
				"size":        "<file_size>",
			}).
			POST("/open-apis/drive/v1/medias/upload_part").
			Desc(fmt.Sprintf("[%db] Upload file parts (repeated)", step)).
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"seq":       "<chunk_index>",
				"size":      "<chunk_size>",
				"file":      "<chunk_binary>",
			}).
			POST("/open-apis/drive/v1/medias/upload_finish").
			Desc(fmt.Sprintf("[%dc] Finalize multipart upload and get file_token", step)).
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"block_num": "<block_num>",
			})
		return
	}

	d.POST("/open-apis/drive/v1/medias/upload_all").
		Desc(fmt.Sprintf("[%d] Upload local file (multipart/form-data)", step)).
		Body(map[string]interface{}{
			"file_name":   filepath.Base(filePath),
			"parent_type": parentType,
			"parent_node": "<new_block_id>",
			"size":        "<file_size>",
			"file":        "@" + filePath,
		})
}
