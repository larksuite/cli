// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	driveImportPollAttempts = 30
	driveImportPollInterval = 2 * time.Second
)

const (
	// These limits follow the current product-side import constraints per format.
	driveImport20MBFileSizeLimit  int64 = 20 * 1024 * 1024
	driveImport100MBFileSizeLimit int64 = 100 * 1024 * 1024
	driveImport600MBFileSizeLimit int64 = 600 * 1024 * 1024
	driveImport800MBFileSizeLimit int64 = 800 * 1024 * 1024
)

// driveImportExtToDocTypes defines which source file extensions can be imported
// into which Drive-native document types.
var driveImportExtToDocTypes = map[string][]string{
	"docx":     {"docx"},
	"doc":      {"docx"},
	"txt":      {"docx"},
	"md":       {"docx"},
	"mark":     {"docx"},
	"markdown": {"docx"},
	"html":     {"docx"},
	"xlsx":     {"sheet", "bitable"},
	"xls":      {"sheet"},
	"csv":      {"sheet", "bitable"},
	"base":     {"bitable"},
}

// driveImportSpec contains the user-facing import inputs after normalization.
type driveImportSpec struct {
	FilePath    string
	DocType     string
	FolderToken string
	Name        string
}

func (s driveImportSpec) FileExtension() string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(s.FilePath)), ".")
}

func (s driveImportSpec) SourceFileName() string {
	return filepath.Base(s.FilePath)
}

func (s driveImportSpec) TargetFileName() string {
	return importTargetFileName(s.FilePath, s.Name)
}

// CreateTaskBody builds the request body expected by /drive/v1/import_tasks.
func (s driveImportSpec) CreateTaskBody(fileToken string) map[string]interface{} {
	return map[string]interface{}{
		"file_extension": s.FileExtension(),
		"file_token":     fileToken,
		"type":           s.DocType,
		"file_name":      s.TargetFileName(),
		"point": map[string]interface{}{
			"mount_type": 1,
			// The import API treats an empty mount_key as "use the caller's root
			// folder", so preserve the zero value when --folder-token is omitted.
			"mount_key": s.FolderToken,
		},
	}
}

// uploadMediaForImport uploads the source file to the temporary import media
// endpoint and returns the file token consumed by import_tasks.
func uploadMediaForImport(ctx context.Context, runtime *common.RuntimeContext, filePath, fileName, docType string) (string, error) {
	importInfo, err := runtime.FileIO().Stat(filePath)
	if err != nil {
		return "", common.WrapInputStatError(err)
	}

	fileSize := importInfo.Size()
	if err = validateDriveImportFileSize(filePath, docType, fileSize); err != nil {
		return "", err
	}

	extra, err := buildImportMediaExtra(filePath, docType)
	if err != nil {
		return "", err
	}

	if fileSize <= common.MaxDriveMediaUploadSinglePartSize {
		fmt.Fprintf(runtime.IO().ErrOut, "Uploading media for import: %s (%s)\n", fileName, common.FormatSize(fileSize))
		// upload_all for import works without parent_node; omitting it preserves
		// the existing root-level import staging behavior.
		return common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   filePath,
			FileName:   fileName,
			FileSize:   fileSize,
			ParentType: "ccm_import_open",
			Extra:      extra,
		})
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Uploading media for import via multipart upload: %s (%s)\n", fileName, common.FormatSize(fileSize))
	// upload_prepare is stricter than upload_all here and expects parent_node to
	// be sent explicitly, even when import uses the implicit root staging area.
	return common.UploadDriveMediaMultipart(runtime, common.DriveMediaMultipartUploadConfig{
		FilePath:   filePath,
		FileName:   fileName,
		FileSize:   fileSize,
		ParentType: "ccm_import_open",
		ParentNode: "",
		Extra:      extra,
	})
}

func buildImportMediaExtra(filePath, docType string) (string, error) {
	// The import media endpoint uses extra to decide both the target native type
	// and how to interpret the uploaded source file.
	extraBytes, err := json.Marshal(map[string]string{
		"obj_type":       docType,
		"file_extension": strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), "."),
	})
	if err != nil {
		return "", output.Errorf(output.ExitInternal, "json_error", "build upload extra failed: %v", err)
	}
	return string(extraBytes), nil
}

func driveImportFileSizeLimit(filePath, docType string) (int64, bool) {
	// Keep the limit mapping local to import flows so we do not widen behavior
	// changes beyond drive +import.
	switch strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".") {
	case "docx", "doc":
		return driveImport600MBFileSizeLimit, true
	case "txt", "md", "mark", "markdown", "html", "xls", "base":
		return driveImport20MBFileSizeLimit, true
	case "xlsx":
		return driveImport800MBFileSizeLimit, true
	case "csv":
		if docType == "bitable" {
			return driveImport100MBFileSizeLimit, true
		}
		return driveImport20MBFileSizeLimit, true
	default:
		return 0, false
	}
}

func validateDriveImportFileSize(filePath, docType string, fileSize int64) error {
	limit, ok := driveImportFileSizeLimit(filePath, docType)
	if !ok || fileSize <= limit {
		return nil
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	if ext == "csv" {
		// CSV is the only source format whose limit depends on the target type.
		return output.ErrValidation(
			"file %s exceeds %s import limit for .csv when importing as %s",
			common.FormatSize(fileSize),
			common.FormatSize(limit),
			docType,
		)
	}

	return output.ErrValidation(
		"file %s exceeds %s import limit for .%s",
		common.FormatSize(fileSize),
		common.FormatSize(limit),
		ext,
	)
}

// validateDriveImportSpec enforces the CLI-level compatibility rules before any
// upload or import request is sent to the backend.
func validateDriveImportSpec(spec driveImportSpec) error {
	ext := spec.FileExtension()
	if ext == "" {
		return output.ErrValidation("file must have an extension (e.g. .md, .docx, .xlsx)")
	}

	switch spec.DocType {
	case "docx", "sheet", "bitable":
	default:
		return output.ErrValidation("unsupported target document type: %s. Supported types are: docx, sheet, bitable", spec.DocType)
	}

	supportedTypes, ok := driveImportExtToDocTypes[ext]
	if !ok {
		return output.ErrValidation("unsupported file extension: %s. Supported extensions are: docx, doc, txt, md, mark, markdown, html, xlsx, xls, csv, base", ext)
	}

	typeAllowed := false
	// Validate the extension/type pair locally so users get a precise error
	// before the file upload step.
	for _, allowedType := range supportedTypes {
		if allowedType == spec.DocType {
			typeAllowed = true
			break
		}
	}
	if !typeAllowed {
		var hint string
		switch ext {
		case "xlsx", "csv":
			hint = fmt.Sprintf(".%s files can only be imported as 'sheet' or 'bitable', not '%s'", ext, spec.DocType)
		case "xls":
			hint = fmt.Sprintf(".xls files can only be imported as 'sheet', not '%s'", spec.DocType)
		case "base":
			hint = fmt.Sprintf(".base files can only be imported as 'bitable', not '%s'", spec.DocType)
		default:
			hint = fmt.Sprintf(".%s files can only be imported as 'docx', not '%s'", ext, spec.DocType)
		}
		return output.ErrValidation("file type mismatch: %s", hint)
	}

	if strings.TrimSpace(spec.FolderToken) != "" {
		if err := validate.ResourceName(spec.FolderToken, "--folder-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}

	return nil
}

// driveImportStatus captures the backend fields needed to decide whether the
// import can be surfaced immediately or requires a follow-up poll.
type driveImportStatus struct {
	Ticket      string
	DocType     string
	Token       string
	URL         string
	JobErrorMsg string
	Extra       interface{}
	JobStatus   int
}

func (s driveImportStatus) Ready() bool {
	return s.Token != "" && s.JobStatus == 0
}

func (s driveImportStatus) Pending() bool {
	return s.JobStatus == 1 || s.JobStatus == 2 || (s.JobStatus == 0 && s.Token == "")
}

func (s driveImportStatus) Failed() bool {
	return !s.Ready() && !s.Pending() && s.JobStatus != 0
}

func (s driveImportStatus) StatusLabel() string {
	switch s.JobStatus {
	case 0:
		// Some responses report status=0 before the imported token is materialized.
		// Treat that intermediate state as pending rather than completed.
		if s.Token == "" {
			return "pending"
		}
		return "success"
	case 1:
		return "new"
	case 2:
		return "processing"
	default:
		return fmt.Sprintf("status_%d", s.JobStatus)
	}
}

// driveImportTaskResultCommand prints the resume command returned after bounded
// polling times out locally.
func driveImportTaskResultCommand(ticket string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario import --ticket %s", ticket)
}

// createDriveImportTask creates the server-side import task after the media
// upload has produced a reusable file token.
func createDriveImportTask(runtime *common.RuntimeContext, spec driveImportSpec, fileToken string) (string, error) {
	data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/import_tasks", nil, spec.CreateTaskBody(fileToken))
	if err != nil {
		return "", err
	}

	ticket := common.GetString(data, "ticket")
	if ticket == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "no ticket returned from import_tasks")
	}
	return ticket, nil
}

// getDriveImportStatus fetches the current state of an import task by ticket.
func getDriveImportStatus(runtime *common.RuntimeContext, ticket string) (driveImportStatus, error) {
	if err := validate.ResourceName(ticket, "--ticket"); err != nil {
		return driveImportStatus{}, output.ErrValidation("%s", err)
	}

	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/import_tasks/%s", validate.EncodePathSegment(ticket)),
		nil,
		nil,
	)
	if err != nil {
		return driveImportStatus{}, err
	}

	return parseDriveImportStatus(ticket, data), nil
}

// parseDriveImportStatus accepts either the wrapped API response or an already
// extracted result object to keep the helper easy to test.
func parseDriveImportStatus(ticket string, data map[string]interface{}) driveImportStatus {
	result := common.GetMap(data, "result")
	if result == nil {
		// Some tests and helper call sites already pass the unwrapped result body.
		result = data
	}

	return driveImportStatus{
		Ticket:      ticket,
		DocType:     common.GetString(result, "type"),
		Token:       common.GetString(result, "token"),
		URL:         common.GetString(result, "url"),
		JobErrorMsg: common.GetString(result, "job_error_msg"),
		Extra:       result["extra"],
		JobStatus:   int(common.GetFloat(result, "job_status")),
	}
}

// pollDriveImportTask waits for the import to finish within a bounded window
// and returns the last observed status for resume-on-timeout flows.
func pollDriveImportTask(runtime *common.RuntimeContext, ticket string) (driveImportStatus, bool, error) {
	lastStatus := driveImportStatus{Ticket: ticket}
	var lastErr error
	hadSuccessfulPoll := false
	for attempt := 1; attempt <= driveImportPollAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(driveImportPollInterval)
		}

		status, err := getDriveImportStatus(runtime, ticket)
		if err != nil {
			lastErr = err
			// Log the error but continue polling.
			fmt.Fprintf(runtime.IO().ErrOut, "Import status attempt %d/%d failed: %v\n", attempt, driveImportPollAttempts, err)
			continue
		}
		lastStatus = status
		hadSuccessfulPoll = true

		// Stop immediately on terminal states and otherwise return the last known
		// status so the caller can expose a follow-up command on timeout.
		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Import completed successfully.\n")
			return status, true, nil
		}
		if status.Failed() {
			msg := strings.TrimSpace(status.JobErrorMsg)
			if msg == "" {
				msg = status.StatusLabel()
			}
			return status, false, output.Errorf(output.ExitAPI, "api_error", "import failed with status %d: %s", status.JobStatus, msg)
		}
	}
	if !hadSuccessfulPoll && lastErr != nil {
		return lastStatus, false, lastErr
	}

	return lastStatus, false, nil
}
