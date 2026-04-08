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
	Description: "Insert a local image or file into a Lark document (4-step orchestration + auto-rollback); appends to end by default, or inserts relative to a keyword with --after-keyword/--before-keyword",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload", "docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (files > 20MB use multipart upload automatically)", Required: true},
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: "image", Desc: "type: image | file"},
		{Name: "align", Desc: "alignment: left | center | right"},
		{Name: "caption", Desc: "image caption text"},
		{Name: "after-keyword", Desc: "insert after the first block whose text contains this keyword (case-insensitive); mutually exclusive with --before-keyword"},
		{Name: "before-keyword", Desc: "insert before the first block whose text contains this keyword (case-insensitive); mutually exclusive with --after-keyword"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return err
		}
		if docRef.Kind == "doc" {
			return output.ErrValidation("docs +media-insert only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx")
		}
		if runtime.Str("after-keyword") != "" && runtime.Str("before-keyword") != "" {
			return output.ErrValidation("--after-keyword and --before-keyword are mutually exclusive")
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
		afterKeyword := runtime.Str("after-keyword")
		beforeKeyword := runtime.Str("before-keyword")
		hasKeyword := afterKeyword != "" || beforeKeyword != ""

		parentType := parentTypeForMediaType(mediaType)
		createBlockData := buildCreateBlockData(mediaType, 0)
		createBlockData["index"] = "<children_len>"
		batchUpdateData := buildBatchUpdateData("<new_block_id>", mediaType, "<file_token>", runtime.Str("align"), caption)

		d := common.NewDryRunAPI()
		totalSteps := 4
		if docRef.Kind == "wiki" {
			totalSteps++
		}
		if hasKeyword {
			totalSteps++
		}

		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			stepBase = 2
			d.Desc(fmt.Sprintf("%d-step orchestration: resolve wiki → query root →%s create block → upload file → bind to block (auto-rollback on failure)",
				totalSteps, map[bool]string{true: " search blocks →", false: ""}[hasKeyword])).
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		} else {
			d.Desc(fmt.Sprintf("%d-step orchestration: query root →%s create block → upload file → bind to block (auto-rollback on failure)",
				totalSteps, map[bool]string{true: " search blocks →", false: ""}[hasKeyword]))
		}

		d.
			GET("/open-apis/docx/v1/documents/:document_id/blocks/:document_id").
			Desc(fmt.Sprintf("[%d] Get document root block", stepBase))

		if hasKeyword {
			kw := afterKeyword
			if kw == "" {
				kw = beforeKeyword
			}
			d.GET("/open-apis/docx/v1/documents/:document_id/blocks").
				Desc(fmt.Sprintf("[%d] List all blocks to find insert position for keyword %q", stepBase+1, kw)).
				Params(map[string]interface{}{"page_size": 200})
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

		afterKeyword := runtime.Str("after-keyword")
		beforeKeyword := runtime.Str("before-keyword")
		keyword := afterKeyword
		before := false
		if beforeKeyword != "" {
			keyword = beforeKeyword
			before = true
		}
		if keyword != "" {
			fmt.Fprintf(runtime.IO().ErrOut, "Searching blocks for keyword: %q\n", keyword)
			allBlocks, err := fetchAllBlocks(runtime, documentID)
			if err != nil {
				return err
			}
			idx, err := findInsertIndexByKeyword(allBlocks, rootChildren, keyword, before)
			if err != nil {
				return err
			}
			insertIndex = idx
			fmt.Fprintf(runtime.IO().ErrOut, "Keyword found: inserting at index %d\n", insertIndex)
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

// fetchAllBlocks retrieves all blocks in a document via paginated API calls.
func fetchAllBlocks(runtime *common.RuntimeContext, documentID string) ([]map[string]interface{}, error) {
	const maxPages = 50
	var all []map[string]interface{}
	pageToken := ""

	for page := 0; page < maxPages; page++ {
		params := map[string]interface{}{"page_size": 200}
		if pageToken != "" {
			params["page_token"] = pageToken
		}
		data, err := runtime.CallAPI("GET",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks", validate.EncodePathSegment(documentID)),
			params, nil)
		if err != nil {
			return nil, err
		}

		items, _ := data["items"].([]interface{})
		for _, item := range items {
			if block, ok := item.(map[string]interface{}); ok {
				all = append(all, block)
			}
		}

		hasMore, _ := data["has_more"].(bool)
		if !hasMore {
			break
		}
		pageToken, _ = data["page_token"].(string)
		if pageToken == "" {
			break
		}
	}
	return all, nil
}

// extractBlockPlainText returns the concatenated plain text of a block
// by inspecting all known text-bearing sub-maps (text, heading1-9, bullet,
// ordered, todo, code, quote). All these block types share the same
// {elements: [{text_run: {content: "..."}}]} structure.
func extractBlockPlainText(block map[string]interface{}) string {
	keys := []string{"text", "heading1", "heading2", "heading3", "heading4", "heading5",
		"heading6", "heading7", "heading8", "heading9", "bullet", "ordered", "todo", "code", "quote"}
	for _, key := range keys {
		sub, ok := block[key].(map[string]interface{})
		if !ok {
			continue
		}
		elements, _ := sub["elements"].([]interface{})
		var sb strings.Builder
		for _, el := range elements {
			elem, _ := el.(map[string]interface{})
			textRun, _ := elem["text_run"].(map[string]interface{})
			content, _ := textRun["content"].(string)
			sb.WriteString(content)
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return ""
}

// findInsertIndexByKeyword finds the insert position relative to the first block
// whose plain text contains keyword (case-insensitive). When before is false it
// inserts after the matched root-level block; when before is true it inserts before.
// It walks parent_id chains to handle nested blocks.
func findInsertIndexByKeyword(blocks []map[string]interface{}, rootChildren []interface{}, keyword string, before bool) (int, error) {
	lowerKw := strings.ToLower(keyword)

	// Build a blockID → block map and a blockID → parent map for quick lookup.
	blockByID := make(map[string]map[string]interface{}, len(blocks))
	parentByID := make(map[string]string, len(blocks))
	for _, b := range blocks {
		id, _ := b["block_id"].(string)
		if id != "" {
			blockByID[id] = b
			parentID, _ := b["parent_id"].(string)
			parentByID[id] = parentID
		}
	}

	// Build root children set for O(1) membership test.
	rootSet := make(map[string]int, len(rootChildren))
	for i, c := range rootChildren {
		if id, ok := c.(string); ok {
			rootSet[id] = i
		}
	}

	// Search blocks in document order.
	for _, b := range blocks {
		text := extractBlockPlainText(b)
		if text == "" || !strings.Contains(strings.ToLower(text), lowerKw) {
			continue
		}
		// Found a match — walk up parent chain to find its top-level ancestor in rootChildren.
		id, _ := b["block_id"].(string)
		cur := id
		for {
			if idx, ok := rootSet[cur]; ok {
				if before {
					return idx, nil // insert before this root-level block
				}
				return idx + 1, nil // insert after this root-level block
			}
			parent := parentByID[cur]
			if parent == "" || parent == cur {
				break
			}
			cur = parent
		}
		return 0, output.ErrValidation("block containing keyword %q is not reachable from document root; try a top-level heading", keyword)
	}
	return 0, output.ErrValidation("no block found containing keyword %q", keyword)
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
