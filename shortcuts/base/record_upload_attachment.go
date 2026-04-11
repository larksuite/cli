// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	baseAttachmentUploadMaxFileSize = 20 * 1024 * 1024
	baseAttachmentParentType        = "bitable_file"
)

var BaseRecordUploadAttachment = common.Shortcut{
	Service:     "base",
	Command:     "+record-upload-attachment",
	Description: "Upload a local file to a Base attachment field and write it into the target record",
	Risk:        "write",
	Scopes:      []string{"base:record:update", "base:field:read", "docs:document.media:upload"},
	AuthTypes:   authTypes(),
	Flags: []common.Flag{
		baseTokenFlag(true),
		tableRefFlag(true),
		recordRefFlag(true),
		fieldRefFlag(true),
		{Name: "file", Desc: "local file path (max 20MB)", Required: true},
		{Name: "name", Desc: "attachment file name (default: local file name)"},
	},
	DryRun: dryRunRecordUploadAttachment,
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return executeRecordUploadAttachment(runtime)
	},
}

func dryRunRecordUploadAttachment(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	filePath := runtime.Str("file")
	fileName := strings.TrimSpace(runtime.Str("name"))
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}
	return common.NewDryRunAPI().
		Desc("4-step orchestration: validate attachment field → read existing record attachments → upload file to Base → patch merged attachment array").
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/fields/:field_id").
		Desc("[1] Read target field and ensure it is an attachment field").
		Set("base_token", runtime.Str("base-token")).
		Set("table_id", baseTableID(runtime)).
		Set("field_id", runtime.Str("field-id")).
		GET("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
		Desc("[2] Read current record to preserve existing attachments in the target cell").
		Set("record_id", runtime.Str("record-id")).
		POST("/open-apis/drive/v1/medias/upload_all").
		Desc("[3] Upload local file to the current Base as attachment media (multipart/form-data)").
		Body(map[string]interface{}{
			"file_name":   fileName,
			"parent_type": baseAttachmentParentType,
			"parent_node": runtime.Str("base-token"),
			"file":        "@" + filePath,
		}).
		PATCH("/open-apis/base/v3/bases/:base_token/tables/:table_id/records/:record_id").
		Desc("[4] Update the target attachment cell with existing attachments plus the uploaded file token").
		Body(map[string]interface{}{
			"<attachment_field_name>": []interface{}{
				map[string]interface{}{
					"file_token":                "<existing_file_token>",
					"name":                      "<existing_file_name>",
					"deprecated_set_attachment": true,
				},
				map[string]interface{}{
					"file_token":                "<uploaded_file_token>",
					"name":                      fileName,
					"deprecated_set_attachment": true,
				},
			},
		})
}

func executeRecordUploadAttachment(runtime *common.RuntimeContext) error {
	filePath := runtime.Str("file")
	fio := runtime.FileIO()
	if fio == nil {
		return output.ErrValidation("file operations require a FileIO provider")
	}
	fileInfo, err := fio.Stat(filePath)
	if err != nil {
		if errors.Is(err, fileio.ErrPathValidation) {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		return output.ErrValidation("file not accessible: %s: %v", filePath, err)
	}
	if fileInfo.Size() > baseAttachmentUploadMaxFileSize {
		return output.ErrValidation("file %.1fMB exceeds 20MB limit", float64(fileInfo.Size())/1024/1024)
	}

	fileName := strings.TrimSpace(runtime.Str("name"))
	if fileName == "" {
		fileName = filepath.Base(filePath)
	}

	field, err := fetchBaseField(runtime, runtime.Str("base-token"), baseTableID(runtime), runtime.Str("field-id"))
	if err != nil {
		return err
	}
	if normalized := normalizeFieldTypeName(fieldTypeName(field)); normalized != "attachment" {
		return output.ErrValidation("field %q is type %q, expected attachment", fieldName(field), normalized)
	}

	record, err := fetchBaseRecord(runtime, runtime.Str("base-token"), baseTableID(runtime), runtime.Str("record-id"))
	if err != nil {
		return err
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Uploading attachment: %s -> record %s field %s\n", fileName, runtime.Str("record-id"), fieldName(field))

	attachment, err := uploadAttachmentToBase(runtime, filePath, fileName, runtime.Str("base-token"), fileInfo.Size())
	if err != nil {
		return err
	}

	attachments, err := mergeRecordAttachments(record, fieldName(field), attachment)
	if err != nil {
		return err
	}

	body := map[string]interface{}{
		fieldName(field): attachments,
	}
	data, err := baseV3Call(runtime, "PATCH", baseV3Path("bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", runtime.Str("record-id")), nil, body)
	if err != nil {
		return err
	}
	runtime.Out(map[string]interface{}{
		"record":      data,
		"attachment":  attachment,
		"attachments": attachments,
		"updated":     true,
	}, nil)
	return nil
}

func fetchBaseField(runtime *common.RuntimeContext, baseToken, tableIDValue, fieldRef string) (map[string]interface{}, error) {
	return baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "fields", fieldRef), nil, nil)
}

func fetchBaseRecord(runtime *common.RuntimeContext, baseToken, tableIDValue, recordID string) (map[string]interface{}, error) {
	return baseV3Call(runtime, "GET", baseV3Path("bases", baseToken, "tables", tableIDValue, "records", recordID), nil, nil)
}

func mergeRecordAttachments(record map[string]interface{}, fieldName string, uploaded map[string]interface{}) ([]interface{}, error) {
	fields, _ := record["fields"].(map[string]interface{})
	if fields == nil {
		return []interface{}{uploaded}, nil
	}
	current, exists := fields[fieldName]
	if !exists || util.IsNil(current) {
		return []interface{}{uploaded}, nil
	}
	items, ok := current.([]interface{})
	if !ok {
		return nil, output.ErrValidation("record field %q has unexpected attachment payload type %T", fieldName, current)
	}
	merged := make([]interface{}, 0, len(items)+1)
	for _, item := range items {
		attachment, ok := item.(map[string]interface{})
		if !ok {
			return nil, output.ErrValidation("record field %q contains unexpected attachment item type %T", fieldName, item)
		}
		merged = append(merged, normalizeAttachmentForPatch(attachment))
	}
	merged = append(merged, uploaded)
	return merged, nil
}

func normalizeAttachmentForPatch(attachment map[string]interface{}) map[string]interface{} {
	normalized := map[string]interface{}{}
	if fileToken, _ := attachment["file_token"].(string); fileToken != "" {
		normalized["file_token"] = fileToken
	}
	if name, _ := attachment["name"].(string); name != "" {
		normalized["name"] = name
	}
	if mimeType, _ := attachment["mime_type"].(string); mimeType != "" {
		normalized["mime_type"] = mimeType
	}
	if size, ok := attachment["size"]; ok && !util.IsNil(size) {
		normalized["size"] = size
	}
	if imageWidth, ok := attachment["image_width"]; ok && !util.IsNil(imageWidth) {
		normalized["image_width"] = imageWidth
	}
	if imageHeight, ok := attachment["image_height"]; ok && !util.IsNil(imageHeight) {
		normalized["image_height"] = imageHeight
	}
	normalized["deprecated_set_attachment"] = true
	return normalized
}

func uploadAttachmentToBase(runtime *common.RuntimeContext, filePath, fileName, baseToken string, fileSize int64) (map[string]interface{}, error) {
	f, err := runtime.FileIO().Open(filePath)
	if err != nil {
		return nil, output.ErrValidation("cannot open file: %v", err)
	}
	defer f.Close()

	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", baseAttachmentParentType)
	fd.AddField("parent_node", baseToken)
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	fd.AddFile("file", f)

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    "/open-apis/drive/v1/medias/upload_all",
		Body:       fd,
	}, larkcore.WithFileUpload())
	if err != nil {
		var exitErr *output.ExitError
		if errors.As(err, &exitErr) {
			return nil, err
		}
		return nil, output.ErrNetwork("upload failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return nil, output.Errorf(output.ExitAPI, "api_error", "upload failed: invalid response JSON: %v", err)
	}

	code, _ := util.ToFloat64(result["code"])
	if code != 0 {
		msg, _ := result["msg"].(string)
		return nil, output.ErrAPI(int(code), fmt.Sprintf("upload failed: [%d] %s", int(code), msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken, _ := data["file_token"].(string)
	if fileToken == "" {
		return nil, output.Errorf(output.ExitAPI, "api_error", "upload failed: no file_token returned")
	}

	attachment := map[string]interface{}{
		"file_token":                fileToken,
		"name":                      fileName,
		"deprecated_set_attachment": true,
	}
	return attachment, nil
}
