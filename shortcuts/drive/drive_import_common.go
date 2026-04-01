// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
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

var driveImportExtToDocTypes = map[string][]string{
	"docx":     {"docx"},
	"doc":      {"docx"},
	"txt":      {"docx"},
	"md":       {"docx"},
	"mark":     {"docx"},
	"markdown": {"docx"},
	"html":     {"docx"},
	"xlsx":     {"sheet", "bitable"},
	"xls":      {"sheet", "bitable"},
	"csv":      {"sheet", "bitable"},
}

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

func (s driveImportSpec) CreateTaskBody(fileToken string) map[string]interface{} {
	return map[string]interface{}{
		"file_extension": s.FileExtension(),
		"file_token":     fileToken,
		"type":           s.DocType,
		"file_name":      s.TargetFileName(),
		"point": map[string]interface{}{
			"mount_type": 1,
			"mount_key":  s.FolderToken,
		},
	}
}

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
		return output.ErrValidation("unsupported file extension: %s. Supported extensions are: docx, doc, txt, md, mark, markdown, html, xlsx, xls, csv", ext)
	}

	typeAllowed := false
	for _, allowedType := range supportedTypes {
		if allowedType == spec.DocType {
			typeAllowed = true
			break
		}
	}
	if !typeAllowed {
		var hint string
		switch ext {
		case "xlsx", "xls", "csv":
			hint = fmt.Sprintf(".%s files can only be imported as 'sheet' or 'bitable', not '%s'", ext, spec.DocType)
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

func driveImportTaskResultCommand(ticket string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario import --ticket %s", ticket)
}

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

func parseDriveImportStatus(ticket string, data map[string]interface{}) driveImportStatus {
	result := common.GetMap(data, "result")
	if result == nil {
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

func pollDriveImportTask(runtime *common.RuntimeContext, ticket string) (driveImportStatus, bool, error) {
	lastStatus := driveImportStatus{Ticket: ticket}
	for attempt := 1; attempt <= driveImportPollAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(driveImportPollInterval)
		}

		status, err := getDriveImportStatus(runtime, ticket)
		if err != nil {
			return driveImportStatus{}, false, err
		}
		lastStatus = status
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

	return lastStatus, false, nil
}
