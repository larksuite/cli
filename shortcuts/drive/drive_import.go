// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
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
		{Name: "file", Desc: "local file path (e.g. .docx, .xlsx, .md, .base; large files auto use multipart upload; .base is capped at 20MB)", Required: true},
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
		fileSize, err := preflightDriveImportFile(runtime.FileIO(), &spec)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}

		dry := common.NewDryRunAPI()
		dry.Desc("Upload file (single-part or multipart) -> create import task -> poll status")

		appendDriveImportUploadDryRun(dry, spec, fileSize)

		dry.POST("/open-apis/drive/v1/import_tasks").
			Desc("[2] Create import task").
			Body(spec.CreateTaskBody("<file_token>"))

		dry.GET("/open-apis/drive/v1/import_tasks/:ticket").
			Desc("[3] Poll import task result").
			Set("ticket", "<ticket>")
		if runtime.IsBot() {
			dry.Desc("After the import result returns the final cloud document target in bot mode, the CLI will also try to grant the current CLI user full_access (可管理权限) on it.")
		}

		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveImportSpec{
			FilePath:    runtime.Str("file"),
			DocType:     strings.ToLower(runtime.Str("type")),
			FolderToken: runtime.Str("folder-token"),
			Name:        runtime.Str("name"),
		}
		if _, err := preflightDriveImportFile(runtime.FileIO(), &spec); err != nil {
			return err
		}

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
		if ready {
			if grant := common.AutoGrantCurrentUserDrivePermission(runtime, common.GetString(out, "token"), resultType); grant != nil {
				out["permission_grant"] = grant
			}
		}

		runtime.Out(out, nil)
		return nil
	},
}

func preflightDriveImportFile(fio fileio.FileIO, spec *driveImportSpec) (int64, error) {
	// Keep dry-run and execution aligned on path normalization, file existence,
	// and format-specific size limits before planning the upload path.
	info, err := fio.Stat(spec.FilePath)
	if err != nil {
		return 0, common.WrapInputStatError(err)
	}
	if !info.Mode().IsRegular() {
		return 0, output.ErrValidation("file must be a regular file: %s", spec.FilePath)
	}
	if err = validateDriveImportFileSize(spec.FilePath, spec.DocType, info.Size()); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func appendDriveImportUploadDryRun(dry *common.DryRunAPI, spec driveImportSpec, fileSize int64) {
	extra, err := buildImportMediaExtra(spec.FilePath, spec.DocType)
	if err != nil {
		extra = fmt.Sprintf(`{"obj_type":"%s","file_extension":"%s"}`, spec.DocType, spec.FileExtension())
	}

	if fileSize > common.MaxDriveMediaUploadSinglePartSize {
		dry.POST("/open-apis/drive/v1/medias/upload_prepare").
			Desc("[1a] Initialize multipart upload").
			Body(map[string]interface{}{
				"file_name":   spec.SourceFileName(),
				"parent_type": "ccm_import_open",
				"parent_node": "",
				"size":        "<file_size>",
				"extra":       extra,
			})
		dry.POST("/open-apis/drive/v1/medias/upload_part").
			Desc("[1b] Upload file parts (repeated)").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"seq":       "<chunk_index>",
				"size":      "<chunk_size>",
				"file":      "<chunk_binary>",
			})
		dry.POST("/open-apis/drive/v1/medias/upload_finish").
			Desc("[1c] Finalize multipart upload and get file_token").
			Body(map[string]interface{}{
				"upload_id": "<upload_id>",
				"block_num": "<block_num>",
			})
		return
	}

	dry.POST("/open-apis/drive/v1/medias/upload_all").
		Desc("[1] Upload file to get file_token").
		Body(map[string]interface{}{
			"file_name":   spec.SourceFileName(),
			"parent_type": "ccm_import_open",
			"size":        "<file_size>",
			"extra":       extra,
			"file":        "@" + spec.FilePath,
		})
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
