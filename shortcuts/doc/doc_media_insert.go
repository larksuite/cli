// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

const maxFileSize = 20 * 1024 * 1024 // 20MB

var alignMap = map[string]int{
	"left":   1,
	"center": 2,
	"right":  3,
}

var DocMediaInsert = common.Shortcut{
	Service:     "docs",
	Command:     "+media-insert",
	Description: "Insert a local image or file at the end of a Lark document (4-step orchestration + auto-rollback)",
	Risk:        "write",
	Scopes:      []string{"docs:document.media:upload", "docx:document:write_only", "docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (max 20MB)", Required: true},
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: "image", Desc: "type: image | file"},
		{Name: "align", Desc: "alignment: left | center | right"},
		{Name: "caption", Desc: "image caption text"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return err
		}
		if docRef.Kind == "doc" {
			return output.ErrValidation("docs +media-insert only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx")
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

		parentType := parentTypeForMediaType(mediaType)
		createBlockData := buildCreateBlockData(mediaType, 0)
		createBlockData["index"] = "<children_len>"
		batchUpdateData := buildBatchUpdateData("<new_block_id>", mediaType, "<file_token>", runtime.Str("align"), caption)

		d := common.NewDryRunAPI()
		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			stepBase = 2
			d.Desc("5-step orchestration: resolve wiki → query root → create block → upload file → bind to block (auto-rollback on failure)").
				GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		} else {
			d.Desc("4-step orchestration: query root → create block → upload file → bind to block (auto-rollback on failure)")
		}

		d.
			GET("/open-apis/docx/v1/documents/:document_id/blocks/:document_id").
			Desc(fmt.Sprintf("[%d] Get document root block", stepBase)).
			POST("/open-apis/docx/v1/documents/:document_id/blocks/:document_id/children").
			Desc(fmt.Sprintf("[%d] Create empty block at document end", stepBase+1)).
			Body(createBlockData).
			POST("/open-apis/drive/v1/medias/upload_all").
			Desc(fmt.Sprintf("[%d] Upload local file (multipart/form-data)", stepBase+2)).
			Body(map[string]interface{}{
				"file_name":   filepath.Base(filePath),
				"parent_type": parentType,
				"parent_node": "<new_block_id>",
				"file":        "@" + filePath,
			}).
			PATCH("/open-apis/docx/v1/documents/:document_id/blocks/batch_update").
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
		filePath = safeFilePath

		documentID, err := resolveDocxDocumentID(runtime, docInput)
		if err != nil {
			return err
		}

		// Validate file
		stat, err := vfs.Stat(filePath)
		if err != nil {
			return output.ErrValidation("file not found: %s", filePath)
		}
		if stat.Size() > maxFileSize {
			return output.ErrValidation("file %.1fMB exceeds 20MB limit", float64(stat.Size())/1024/1024)
		}

		fileName := filepath.Base(filePath)
		fmt.Fprintf(runtime.IO().ErrOut, "Inserting: %s -> document %s\n", fileName, common.MaskToken(documentID))

		// Step 1: Get document root block to find where to insert
		rootData, err := runtime.CallAPI("GET",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s/blocks/%s", validate.EncodePathSegment(documentID), validate.EncodePathSegment(documentID)),
			nil, nil)
		if err != nil {
			return err
		}

		parentBlockID, insertIndex, err := extractAppendTarget(rootData, documentID)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "Root block ready: %s (%d children)\n", parentBlockID, insertIndex)

		// Step 2: Create an empty block at the end of the document
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

		// Rollback helper
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
		fileToken, err := uploadMediaFile(ctx, runtime, filePath, fileName, mediaType, uploadParentNode, documentID)
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

func extractAppendTarget(rootData map[string]interface{}, fallbackBlockID string) (string, int, error) {
	block, _ := rootData["block"].(map[string]interface{})
	if len(block) == 0 {
		return "", 0, output.Errorf(output.ExitAPI, "api_error", "failed to query document root block")
	}

	parentBlockID := fallbackBlockID
	if blockID, _ := block["block_id"].(string); blockID != "" {
		parentBlockID = blockID
	}

	children, _ := block["children"].([]interface{})
	return parentBlockID, len(children), nil
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

// uploadMediaFile uploads a file to Feishu drive as media.
func uploadMediaFile(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName, mediaType, parentNode, docId string) (string, error) {
	f, err := vfs.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", output.Errorf(output.ExitInternal, "internal_error", "failed to stat file: %v", err)
	}
	fileSize := stat.Size()

	parentType := parentTypeForMediaType(mediaType)

	// Build SDK Formdata
	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", parentType)
	fd.AddField("parent_node", parentNode)
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	if docId != "" {
		extra, err := buildDriveRouteExtra(docId)
		if err != nil {
			return "", err
		}
		fd.AddField("extra", extra)
	}
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/medias/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return "", err
		}
		return "", output.ErrNetwork("file upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", output.Errorf(output.ExitAPI, "api_error", "file upload failed: invalid response JSON: %v", err)
	}

	code, _ := util.ToFloat64(result["code"])
	if code != 0 {
		msg, _ := result["msg"].(string)
		return "", output.ErrAPI(int(code), fmt.Sprintf("file upload failed: [%d] %s", int(code), msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken, _ := data["file_token"].(string)
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "file upload failed: no file_token returned")
	}

	return fileToken, nil
}
