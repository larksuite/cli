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

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// DriveImport uploads a local file, creates an import task, and polls until
// the imported cloud document is ready or the local polling window expires.
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
		{Name: "folder-token", Desc: "target folder token (omit for root folder; API accepts empty mount_key as root)"},
		{Name: "name", Desc: "imported file name (default: local file name without extension)"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveImportSpec(driveImportSpec{
			FilePath:    runtime.Str("file"),
			DocType:     strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
			Name:        runtime.Str("name"),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveImportSpec{
			FilePath:    runtime.Str("file"),
			DocType:     strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
			Name:        runtime.Str("name"),
		}

		dry := common.NewDryRunAPI()
		dry.Desc("3-step orchestration: upload file -> create import task -> poll status")

		dry.POST("/open-apis/drive/v1/medias/upload_all").
			Desc("[1] Upload file to get file_token").
			Body(map[string]interface{}{
				"file_name":   spec.SourceFileName(),
				"parent_type": "ccm_import_open",
				"size":        "<file_size>",
				"extra":       fmt.Sprintf(`{"obj_type":"%s","file_extension":"%s"}`, spec.DocType, spec.FileExtension()),
				"file":        "@" + spec.FilePath,
			})

		dry.POST("/open-apis/drive/v1/import_tasks").
			Desc("[2] Create import task").
			Body(spec.CreateTaskBody("<file_token>"))

		dry.GET("/open-apis/drive/v1/import_tasks/:ticket").
			Desc("[3] Poll import task result").
			Set("ticket", "<ticket>")

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveImportSpec{
			FilePath:    runtime.Str("file"),
			DocType:     strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
			Name:        runtime.Str("name"),
		}

		// Normalize and validate the local input path before opening the file.
		safeFilePath, err := validate.SafeInputPath(spec.FilePath)
		if err != nil {
			return output.ErrValidation("unsafe file path: %s", err)
		}
		spec.FilePath = safeFilePath

		// Step 1: Upload file as media
		fileToken, uploadErr := uploadMediaForImport(ctx, runtime, spec.FilePath, spec.SourceFileName(), spec.DocType)
		if uploadErr != nil {
			return uploadErr
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Creating import task for %s as %s...\n", spec.TargetFileName(), spec.DocType)

		// Step 2: Create import task
		ticket, err := createDriveImportTask(runtime, spec, fileToken)
		if err != nil {
			return err
		}

		// Step 3: Poll task
		fmt.Fprintf(runtime.IO().ErrOut, "Polling import task %s...\n", ticket)

		status, ready, err := pollDriveImportTask(runtime, ticket)
		if err != nil {
			return err
		}

		// Some intermediate responses omit the final type, so fall back to the
		// requested type to keep the output shape stable.
		resultType := status.DocType
		if resultType == "" {
			resultType = spec.DocType
		}
		out := map[string]interface{}{
			"ticket":           ticket,
			"type":             resultType,
			"ready":            ready,
			"job_status":       status.JobStatus,
			"job_status_label": status.StatusLabel(),
		}
		if status.Token != "" {
			out["token"] = status.Token
		}
		if status.URL != "" {
			out["url"] = status.URL
		}
		if status.JobErrorMsg != "" {
			out["job_error_msg"] = status.JobErrorMsg
		}
		if status.Extra != nil {
			out["extra"] = status.Extra
		}
		if !ready {
			nextCommand := driveImportTaskResultCommand(ticket)
			fmt.Fprintf(runtime.IO().ErrOut, "Import task is still in progress. Continue with: %s\n", nextCommand)
			out["timed_out"] = true
			out["next_command"] = nextCommand
		}

		runtime.Out(out, nil)
		return nil
	},
}

// importTargetFileName returns the explicit import name when present, otherwise
// derives one from the local file name.
func importTargetFileName(filePath, explicitName string) string {
	if explicitName != "" {
		return explicitName
	}
	return importDefaultFileName(filePath)
}

// importDefaultFileName strips only the last extension so names like
// "report.final.csv" become "report.final".
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

// uploadMediaForImport uploads the source file to the temporary import media
// endpoint and returns the file token consumed by import_tasks.
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

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
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
			// Preserve already-classified CLI errors from lower layers instead of
			// wrapping them as a generic network failure.
			return "", err
		}
		return "", output.ErrNetwork("upload media failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(apiResp.RawBody, &result); err != nil {
		return "", output.Errorf(output.ExitAPI, "api_error", "upload media failed: invalid response JSON: %v", err)
	}

	if larkCode := int(common.GetFloat(result, "code")); larkCode != 0 {
		// Surface the backend error body so callers can see import-specific
		// validation failures such as unsupported formats or permission issues.
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
