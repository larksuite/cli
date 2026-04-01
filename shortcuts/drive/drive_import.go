// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var DriveImport = common.Shortcut{
	Service:     "drive",
	Command:     "+import",
	Description: "Import a local file to Drive as a cloud document (docx, sheet, bitable)",
	Risk:        "write",
	Scopes: []string{
		"docs:document.media:upload",
		"docs:document:import",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "file", Desc: "local file path (e.g. .docx, .xlsx, .md)", Required: true},
		{Name: "type", Desc: "target document type (docx, sheet, bitable)", Required: true},
		{Name: "folder-token", Desc: "target folder token (default: root)"},
		{Name: "name", Desc: "imported file name (default: local file name without extension)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
		if ext == "" {
			return output.ErrValidation("file must have an extension (e.g. .md, .docx, .xlsx)")
		}
		ext = strings.ToLower(ext)

		docType := strings.ToLower(runtime.Str("type"))
		validTypes := map[string]bool{
			"docx": true, "sheet": true, "bitable": true,
		}
		if !validTypes[docType] {
			return output.ErrValidation("unsupported target document type: %s. Supported types are: docx, sheet, bitable", docType)
		}

		// Map file extensions to the supported cloud document types.
		extToDocTypes := map[string][]string{
			// Document-like files can only be imported as docx.
			"docx":     {"docx"},
			"doc":      {"docx"},
			"txt":      {"docx"},
			"md":       {"docx"},
			"mark":     {"docx"},
			"markdown": {"docx"},
			"html":     {"docx"},
			// Spreadsheet-like files can be imported as sheet or bitable.
			"xlsx": {"sheet", "bitable"},
			"xls":  {"sheet", "bitable"},
			"csv":  {"sheet", "bitable"},
		}

		supportedTypes, ok := extToDocTypes[ext]
		if !ok {
			return output.ErrValidation("unsupported file extension: %s. Supported extensions are: docx, doc, txt, md, mark, markdown, html, xlsx, xls, csv", ext)
		}

		// Validate that the requested target type is compatible with the file extension.
		typeAllowed := false
		for _, allowedType := range supportedTypes {
			if allowedType == docType {
				typeAllowed = true
				break
			}
		}
		if !typeAllowed {
			var hint string
			switch ext {
			case "xlsx", "xls", "csv":
				hint = fmt.Sprintf(".%s files can only be imported as 'sheet' or 'bitable', not '%s'", ext, docType)
			default:
				hint = fmt.Sprintf(".%s files can only be imported as 'docx', not '%s'", ext, docType)
			}
			return output.ErrValidation("file type mismatch: %s", hint)
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		filePath := runtime.Str("file")
		docType := runtime.Str("type")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")

		sourceFileName := filepath.Base(filePath)
		targetFileName := importTargetFileName(filePath, name)

		ext := strings.TrimPrefix(filepath.Ext(filePath), ".")

		dry := common.NewDryRunAPI()
		dry.Desc("3-step orchestration: upload file -> create import task -> poll status")

		dry.POST("/open-apis/drive/v1/medias/upload_all").
			Desc("[1] Upload file to get file_token").
			Body(map[string]interface{}{
				"file_name":   sourceFileName,
				"parent_type": "ccm_import_open",
				"size":        "<file_size>",
				"extra":       fmt.Sprintf(`{"obj_type":"%s","file_extension":"%s"}`, docType, ext),
				"file":        "@" + filePath,
			})

		dry.POST("/open-apis/drive/v1/import_tasks").
			Desc("[2] Create import task").
			Body(map[string]interface{}{
				"file_extension": ext,
				"file_token":     "<file_token>",
				"type":           docType,
				"file_name":      targetFileName,
				"point": map[string]interface{}{
					"mount_type": 1,
					"mount_key":  folderToken,
				},
			})

		dry.GET("/open-apis/drive/v1/import_tasks/:ticket").
			Desc("[3] Poll import task result").
			Set("ticket", "<ticket>")

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		filePath := runtime.Str("file")
		docType := runtime.Str("type")
		folderToken := runtime.Str("folder-token")
		name := runtime.Str("name")

		safeFilePath, err := validate.SafeInputPath(filePath)
		if err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		filePath = safeFilePath

		sourceFileName := filepath.Base(filePath)
		targetFileName := importTargetFileName(filePath, name)

		ext := strings.TrimPrefix(filepath.Ext(filePath), ".")

		// Step 1: Upload file as media
		fileToken, uploadErr := uploadMediaForImport(ctx, runtime, filePath, sourceFileName, docType)
		if uploadErr != nil {
			return uploadErr
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Creating import task for %s as %s...\n", targetFileName, docType)

		// Step 2: Create import task
		createBody := map[string]interface{}{
			"file_extension": ext,
			"file_token":     fileToken,
			"type":           docType,
			"file_name":      targetFileName,
			"point": map[string]interface{}{
				"mount_type": 1,
				"mount_key":  folderToken,
			},
		}

		createResp, err := runtime.CallAPI("POST", "/open-apis/drive/v1/import_tasks", nil, createBody)
		if err != nil {
			return err
		}

		ticket := common.GetString(createResp, "ticket")
		if ticket == "" {
			return output.Errorf(output.ExitAPI, "api_error", "no ticket returned from import_tasks")
		}

		// Step 3: Poll task
		fmt.Fprintf(runtime.IO().ErrOut, "Polling import task %s...\n", ticket)

		maxRetries := 30
		delay := 2 * time.Second

		for i := 0; i < maxRetries; i++ {
			pollResp, err := runtime.CallAPI("GET", "/open-apis/drive/v1/import_tasks/"+validate.EncodePathSegment(ticket), nil, nil)
			if err != nil {
				return err
			}

			result := common.GetMap(pollResp, "result")
			if result == nil {
				// Fallback if result is flattened
				result = pollResp
			}

			jobStatus := int(common.GetFloat(result, "job_status"))

			if jobStatus == 0 {
				token := common.GetString(result, "token")
				url := common.GetString(result, "url")

				fmt.Fprintf(runtime.IO().ErrOut, "Import completed successfully.\n")

				runtime.Out(map[string]interface{}{
					"ticket":     ticket,
					"token":      token,
					"url":        url,
					"job_status": jobStatus,
				}, nil)
				return nil
			}

			// job_status 1, 2 typically mean pending/processing. If there's an error msg, it might have failed.
			if jobStatus != 1 && jobStatus != 2 && jobStatus != 0 {
				errMsg := common.GetString(result, "job_error_msg")
				if errMsg == "" {
					errMsg = "unknown error"
				}
				return output.Errorf(output.ExitAPI, "api_error", "import failed with status %d: %s", jobStatus, errMsg)
			}

			time.Sleep(delay)
		}

		return output.Errorf(output.ExitAPI, "timeout", "import task did not complete in time")
	},
}

func importTargetFileName(filePath, explicitName string) string {
	if explicitName != "" {
		return explicitName
	}
	return importDefaultFileName(filePath)
}

func importDefaultFileName(filePath string) string {
	base := filepath.Base(filePath)
	ext := filepath.Ext(base)
	if ext == "" {
		return base
	}
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return base
	}
	return name
}

func uploadMediaForImport(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName, docType string) (string, error) {
	importInfo, err := os.Stat(filePath)
	if err != nil {
		return "", output.ErrValidation("cannot read file: %s", err)
	}
	fileSize := importInfo.Size()
	if fileSize > maxDriveUploadFileSize {
		return "", output.ErrValidation("file %.1fMB exceeds 20MB limit", float64(fileSize)/1024/1024)
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Uploading media for import: %s (%s)\n", fileName, common.FormatSize(fileSize))

	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	ext := strings.TrimPrefix(filepath.Ext(filePath), ".")
	extraMap := map[string]string{
		"obj_type":       docType,
		"file_extension": ext,
	}
	extraBytes, _ := json.Marshal(extraMap)

	// Build SDK Formdata
	fd := larkcore.NewFormdata()
	fd.AddField("file_name", fileName)
	fd.AddField("parent_type", "ccm_import_open")
	fd.AddField("size", fmt.Sprintf("%d", fileSize))
	fd.AddField("extra", string(extraBytes))
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
		return "", output.ErrNetwork("upload media failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload media failed: invalid response JSON: %v", err)
	}

	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		msg, _ := result["msg"].(string)
		return "", output.ErrAPI(larkCode, fmt.Sprintf("upload media failed: [%d] %s", larkCode, msg), result["error"])
	}

	data, _ := result["data"].(map[string]interface{})
	fileToken, _ := data["file_token"].(string)
	if fileToken == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload media failed: no file_token returned")
	}
	return fileToken, nil
}
