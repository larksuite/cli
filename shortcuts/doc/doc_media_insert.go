// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

var alignMap = map[string]int{
	"left":   1,
	"center": 2,
	"right":  3,
}

var DocMediaInsert = common.Shortcut{
	Service:     "docs",
	Command:     "+media-insert",
	Description: "Insert a local image or file into a Lark document (4-step orchestration + auto-rollback); appends to end by default, or inserts relative to a text selection with --selection-with-ellipsis",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload", "docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)", Required: true},
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: "image", Desc: "type: image | file"},
		{Name: "align", Desc: "alignment: left | center | right"},
		{Name: "caption", Desc: "image caption text"},
		{Name: "selection-with-ellipsis", Desc: "plain text (or 'start...end') that identifies the target block; the media is inserted after that block by default"},
		{Name: "before", Type: "bool", Desc: "insert before the matched block instead of after (requires --selection-with-ellipsis)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return err
		}
		if docRef.Kind == "doc" {
			return output.ErrValidation("docs +media-insert only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx")
		}
		if runtime.Bool("before") && strings.TrimSpace(runtime.Str("selection-with-ellipsis")) == "" {
			return output.ErrValidation("--before requires --selection-with-ellipsis")
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
		mediaType := runtime.Str("type")
		caption := runtime.Str("caption")
		selection := strings.TrimSpace(runtime.Str("selection-with-ellipsis"))
		hasSelection := selection != ""

		parentType := parentTypeForMediaType(mediaType)
		createBlockData := buildCreateBlockData(mediaType, 0)
		createBlockData["index"] = "<children_len>"
		batchUpdateData := buildBatchUpdateData("<new_block_id>", mediaType, "<file_token>", runtime.Str("align"), caption)

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
		appendDocMediaInsertUploadDryRun(d, filePath, parentType, stepBase+2)
		d.PATCH("/open-apis/docx/v1/documents/:document_id/blocks/batch_update").
			Desc(fmt.Sprintf("[%d] Bind uploaded file token to the new block", stepBase+3)).
			Body(batchUpdateData)

		return d.Set("document_id", documentID)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		docInput := runtime.Str("doc")
		mediaType := runtime.Str("type")
		alignStr := runtime.Str("align")
		caption := runtime.Str("caption")

		safeFilePath, pathErr := validate.SafeInputPath(filePath)
		if pathErr != nil {
			return output.ErrValidation("unsafe file path: %s", pathErr)
		}

		documentID, err := resolveDocxDocumentID(runtime, docInput)
		if err != nil {
			return err
		}

		// Validate file
		stat, err := vfs.Stat(safeFilePath)
		if err != nil {
			return output.ErrValidation("file not found: %s", filePath)
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("file must be a regular file: %s", filePath)
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Inserting: %s -> document %s\n", fileName, common.MaskToken(documentID))
		if stat.Size() > common.MaxDriveMediaUploadSinglePartSize {
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
			fmt.Fprintf(runtime.IO().ErrOut, "Locating block matching selection: %q\n", selection)
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
			nil, buildCreateBlockData(mediaType, insertIndex))
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

		// Step 3: Upload media file
		fileToken, err := uploadDocMediaFile(runtime, filePath, fileName, stat.Size(), parentTypeForMediaType(mediaType), uploadParentNode, documentID)
		if err != nil {
			return withRollbackWarning(err)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "File uploaded: %s\n", fileToken)

		// Step 4: Bind file token to block via batch_update
		fmt.Fprintf(runtime.IO().ErrOut, "Binding uploaded media to block %s\n", replaceBlockID)

		if _, err := runtime.CallAPI("PATCH",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/batch_update", validate.EncodePathSegment(documentID)),
			nil, buildBatchUpdateData(replaceBlockID, mediaType, fileToken, alignStr, caption)); err != nil {
			return withRollbackWarning(err)
		}

		runtime.Out(map[string]interface{}{
			"document_id": documentID,
			"block_id":    blockId,
			"file_token":  fileToken,
			"type":        mediaType,
		}, nil)
		return nil
	},
}

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

func buildCreateBlockData(mediaType string, index int) map[string]interface{} {
	child := map[string]interface{}{
		"block_type": blockTypeForMediaType(mediaType),
	}
	if mediaType == "file" {
		child["file"] = map[string]interface{}{}
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

func buildBatchUpdateData(blockID, mediaType, fileToken, alignStr, caption string) map[string]interface{} {
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
	args := map[string]interface{}{
		"doc_id":                  documentID,
		"selection_with_ellipsis": selection,
		"limit":                   1,
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
			fmt.Sprintf("locate-doc did not find any block matching selection %q", selection),
			"check spelling or use 'start...end' syntax to narrow the selection",
		)
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

	// Walk up the parent chain. locate-doc already gives us one level of parent,
	// so most cases need zero extra API calls.
	cur := anchorBlockID
	nextParent := parentBlockID
	visited := map[string]bool{}
	const maxDepth = 8
	for depth := 0; depth < maxDepth; depth++ {
		if visited[cur] {
			break
		}
		visited[cur] = true

		if idx, ok := rootSet[cur]; ok {
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
	}

	return 0, output.ErrWithHint(
		output.ExitValidation,
		"block_not_reachable",
		fmt.Sprintf("block matching selection %q is not reachable from document root", selection),
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

func appendDocMediaInsertUploadDryRun(d *common.DryRunAPI, filePath, parentType string, step int) {
	// The upload step runs only after the empty placeholder block is created, so
	// dry-run can refer to that future block ID only symbolically. For large
	// files, keep multipart internals as substeps of the single user-facing
	// "upload file" step.
	if docMediaShouldUseMultipart(filePath) {
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
