// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// taskAttachmentUploadMaxSize is the upper bound on a single attachment upload
// to the Task service (50MB, as documented by the open API).
const taskAttachmentUploadMaxSize int64 = 50 * 1024 * 1024

// taskAttachmentUploadPath is the Task open-api endpoint that accepts a single
// multipart/form-data upload per call.
const taskAttachmentUploadPath = "/open-apis/task/v2/attachments/upload"

// defaultTaskAttachmentResourceType is used when the caller does not pass an
// explicit --resource-type flag. Task is the only resource type documented for
// this endpoint today, but the flag is kept open so that future resource types
// can be targeted without a client upgrade.
const defaultTaskAttachmentResourceType = "task"

// UploadAttachmentTask uploads a single local file as an attachment to a task
// (or any other resource type accepted by the Task attachment endpoint).
var UploadAttachmentTask = common.Shortcut{
	Service:     "task",
	Command:     "+upload-attachment",
	Description: "upload a local file as an attachment to a task; use --resource-type=task_delivery when uploading to task agents",
	Risk:        "write",
	Scopes:      []string{"task:attachment:write"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,

	Flags: []common.Flag{
		{Name: "resource-id", Desc: "task guid (or task applink URL)", Required: true},
		{Name: "file", Desc: "local file path (single file, <= 50MB)", Required: true},
		{Name: "resource-type", Desc: "owning resource type (default: task); use task_delivery when uploading to task agents", Default: defaultTaskAttachmentResourceType},
		{Name: "user-id-type", Desc: "user id type (default: open_id)", Default: "open_id"},
	},

	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		resourceType := runtime.Str("resource-type")
		if resourceType == "" {
			resourceType = defaultTaskAttachmentResourceType
		}
		resourceID := extractTaskGuid(runtime.Str("resource-id"))
		filePath := runtime.Str("file")
		userIDType := runtime.Str("user-id-type")
		if userIDType == "" {
			userIDType = "open_id"
		}

		return common.NewDryRunAPI().
			POST(taskAttachmentUploadPath).
			Params(map[string]interface{}{"user_id_type": userIDType}).
			Body(map[string]interface{}{
				"resource_type": resourceType,
				"resource_id":   resourceID,
				"file": map[string]string{
					"field": "file",
					"path":  filePath,
					"name":  filepath.Base(filePath),
				},
			})
	},

	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		resourceType := runtime.Str("resource-type")
		if resourceType == "" {
			resourceType = defaultTaskAttachmentResourceType
		}
		resourceID := extractTaskGuid(runtime.Str("resource-id"))
		filePath := runtime.Str("file")
		userIDType := runtime.Str("user-id-type")
		if userIDType == "" {
			userIDType = "open_id"
		}

		fio := runtime.FileIO()
		if fio == nil {
			return output.ErrValidation("file operations require a FileIO provider")
		}
		stat, err := fio.Stat(filePath)
		if err != nil {
			return common.WrapInputStatError(err, "file not found")
		}
		if !stat.Mode().IsRegular() {
			return output.ErrValidation("file must be a regular file: %s", filePath)
		}
		if stat.Size() > taskAttachmentUploadMaxSize {
			return output.ErrValidation(
				"attachment %s exceeds the 50MB per-file limit",
				common.FormatSize(stat.Size()),
			)
		}

		fileName := filepath.Base(filePath)

		// Observability: input parsed.
		fmt.Fprintf(
			runtime.IO().ErrOut,
			"[+upload-attachment] input parsed: resource_type=%s resource_id=%s file=%s size=%s\n",
			resourceType, resourceID, filePath, common.FormatSize(stat.Size()),
		)

		f, err := fio.Open(filePath)
		if err != nil {
			return common.WrapInputStatError(err, "cannot open file")
		}
		defer f.Close()

		// Build the multipart body manually so the real filename is preserved
		// in the `file` part's Content-Disposition. The SDK's Formdata.AddFile
		// hardcodes the filename to "unknown-file" (see oapi-sdk-go
		// core/reqtranslator.go), which is what was showing up in the Task UI.
		var bodyBuf bytes.Buffer
		mw := common.NewMultipartWriter(&bodyBuf)
		if err := mw.WriteField("resource_type", resourceType); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "build multipart body: %s", err)
		}
		if err := mw.WriteField("resource_id", resourceID); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "build multipart body: %s", err)
		}
		filePart, err := mw.CreateFormFile("file", fileName)
		if err != nil {
			return output.Errorf(output.ExitInternal, "internal", "build multipart body: %s", err)
		}
		if _, err := io.Copy(filePart, f); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "write file to multipart body: %s", err)
		}
		if err := mw.Close(); err != nil {
			return output.Errorf(output.ExitInternal, "internal", "finalize multipart body: %s", err)
		}

		queryParams := make(larkcore.QueryParams)
		queryParams.Set("user_id_type", userIDType)

		// Observability: HTTP call about to start.
		fmt.Fprintf(
			runtime.IO().ErrOut,
			"[+upload-attachment] http call: POST %s user_id_type=%s\n",
			taskAttachmentUploadPath, userIDType,
		)

		headers := http.Header{}
		headers.Set("Content-Type", mw.FormDataContentType())

		httpResp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod:  "POST",
			ApiPath:     taskAttachmentUploadPath,
			QueryParams: queryParams,
			Body:        &bodyBuf,
		}, client.WithHeaders(headers))
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut,
				"[+upload-attachment] http response: error=%v\n", err)
			return err
		}
		defer httpResp.Body.Close()

		rawBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut,
				"[+upload-attachment] http response: read_error=%v\n", readErr)
			return WrapTaskError(ErrCodeTaskInternalError,
				fmt.Sprintf("failed to read response: %v", readErr),
				"upload task attachment")
		}

		var result map[string]interface{}
		if parseErr := json.Unmarshal(rawBody, &result); parseErr != nil {
			fmt.Fprintf(runtime.IO().ErrOut,
				"[+upload-attachment] http response: parse_error=%v\n", parseErr)
			return WrapTaskError(ErrCodeTaskInternalError,
				fmt.Sprintf("failed to parse response: %v", parseErr),
				"upload task attachment")
		}

		data, err := HandleTaskApiResult(result, nil, "upload task attachment")
		if err != nil {
			code, _ := result["code"]
			msg, _ := result["msg"].(string)
			fmt.Fprintf(runtime.IO().ErrOut,
				"[+upload-attachment] http response: code=%v msg=%q error=%v\n",
				code, msg, err)
			return err
		}

		// The Task attachment upload endpoint returns `data.items` containing
		// the freshly created attachment records. Since this shortcut uploads
		// exactly one file per call, we surface the single record directly as
		// the output envelope — all fields returned by the API (guid, name,
		// size, url, resource_type, uploader, ...) are preserved verbatim.
		items, _ := data["items"].([]interface{})
		var first map[string]interface{}
		if len(items) > 0 {
			first, _ = items[0].(map[string]interface{})
		}
		if first == nil {
			first = map[string]interface{}{}
		}
		guid, _ := first["guid"].(string)

		code, _ := result["code"]
		msg, _ := result["msg"].(string)
		fmt.Fprintf(runtime.IO().ErrOut,
			"[+upload-attachment] http response: code=%v msg=%q attachment_guid=%s\n",
			code, msg, guid)

		runtime.OutFormat(first, nil, func(w io.Writer) {
			fmt.Fprintf(w, "✅ Attachment uploaded successfully!\n")
			fmt.Fprintf(w, "Resource: %s/%s\n", resourceType, resourceID)
			name, _ := first["name"].(string)
			if name == "" {
				name = fileName
			}
			fmt.Fprintf(w, "File: %s (%s)\n", name, common.FormatSize(stat.Size()))
			if guid != "" {
				fmt.Fprintf(w, "Attachment GUID: %s\n", guid)
			}
		})
		return nil
	},
}
