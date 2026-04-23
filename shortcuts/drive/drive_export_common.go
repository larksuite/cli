// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	driveExportPollAttempts = 10
	driveExportPollInterval = 5 * time.Second
)

// driveExportSpec contains the normalized export request understood by the
// shortcut and the underlying export task APIs.
type driveExportSpec struct {
	Token         string
	DocType       string
	FileExtension string
	SubID         string
}

// driveExportTaskResultCommand prints the resume command shown when bounded
// export polling times out locally.
func driveExportTaskResultCommand(ticket, docToken string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario export --ticket %s --file-token %s", ticket, docToken)
}

// driveExportDownloadCommand prints a copy-pasteable follow-up command for
// downloading an already-generated export artifact by file token.
func driveExportDownloadCommand(fileToken, fileName, outputDir string, overwrite bool) string {
	parts := []string{
		"lark-cli", "drive", "+export-download",
		"--file-token", strconv.Quote(fileToken),
	}
	if strings.TrimSpace(fileName) != "" {
		parts = append(parts, "--file-name", strconv.Quote(fileName))
	}
	if strings.TrimSpace(outputDir) != "" && outputDir != "." {
		parts = append(parts, "--output-dir", strconv.Quote(outputDir))
	}
	if overwrite {
		parts = append(parts, "--overwrite")
	}
	return strings.Join(parts, " ")
}

// driveExportStatus captures the fields needed to decide whether the export is
// ready for download, still pending, or terminally failed.
type driveExportStatus struct {
	Ticket        string
	FileExtension string
	DocType       string
	FileName      string
	FileToken     string
	JobErrorMsg   string
	FileSize      int64
	JobStatus     int
}

func (s driveExportStatus) Ready() bool {
	return s.FileToken != "" && s.JobStatus == 0
}

func (s driveExportStatus) Pending() bool {
	// A zero status without a file token is still in progress because there is
	// nothing downloadable yet.
	return s.JobStatus == 1 || s.JobStatus == 2 || s.JobStatus == 0 && s.FileToken == ""
}

func (s driveExportStatus) Failed() bool {
	return !s.Ready() && !s.Pending() && s.JobStatus != 0
}

func (s driveExportStatus) StatusLabel() string {
	switch s.JobStatus {
	case 0:
		// Success is a special case where the file token is set.
		if s.FileToken != "" {
			return "success"
		}
		return "pending"
	case 1:
		return "new"
	case 2:
		return "processing"
	case 3:
		return "internal_error"
	case 107:
		return "export_size_limit"
	case 108:
		return "timeout"
	case 109:
		return "export_block_not_permitted"
	case 110:
		return "no_permission"
	case 111:
		return "docs_deleted"
	case 122:
		return "export_denied_on_copying"
	case 123:
		return "docs_not_exist"
	case 6000:
		return "export_images_exceed_limit"
	default:
		return fmt.Sprintf("status_%d", s.JobStatus)
	}
}

// validateDriveExportSpec enforces shortcut-level export constraints before any
// backend request is sent.
func validateDriveExportSpec(spec driveExportSpec) error {
	if err := validate.ResourceName(spec.Token, "--token"); err != nil {
		return output.ErrValidation("%s", err)
	}

	switch spec.DocType {
	case "doc", "docx", "sheet", "bitable":
	default:
		return output.ErrValidation("invalid --doc-type %q: allowed values are doc, docx, sheet, bitable", spec.DocType)
	}

	switch spec.FileExtension {
	case "docx", "pdf", "xlsx", "csv", "markdown", "base":
	default:
		return output.ErrValidation("invalid --file-extension %q: allowed values are docx, pdf, xlsx, csv, markdown, base", spec.FileExtension)
	}

	if spec.FileExtension == "markdown" && spec.DocType != "docx" {
		return output.ErrValidation("--file-extension markdown only supports --doc-type docx")
	}

	if spec.FileExtension == "base" && spec.DocType != "bitable" {
		return output.ErrValidation("--file-extension base only supports --doc-type bitable")
	}

	if strings.TrimSpace(spec.SubID) != "" {
		if spec.FileExtension != "csv" || (spec.DocType != "sheet" && spec.DocType != "bitable") {
			return output.ErrValidation("--sub-id is only used when exporting sheet/bitable as csv")
		}
		if err := validate.ResourceName(spec.SubID, "--sub-id"); err != nil {
			return output.ErrValidation("%s", err)
		}
	}

	if spec.FileExtension == "csv" && (spec.DocType == "sheet" || spec.DocType == "bitable") && strings.TrimSpace(spec.SubID) == "" {
		return output.ErrValidation("--sub-id is required when exporting sheet/bitable as csv")
	}

	return nil
}

// createDriveExportTask starts the asynchronous export job and returns its
// ticket for subsequent polling.
func createDriveExportTask(runtime *common.RuntimeContext, spec driveExportSpec) (string, error) {
	body := map[string]interface{}{
		"token":          spec.Token,
		"type":           spec.DocType,
		"file_extension": spec.FileExtension,
	}
	if strings.TrimSpace(spec.SubID) != "" {
		body["sub_id"] = spec.SubID
	}

	data, err := runtime.CallAPI("POST", "/open-apis/drive/v1/export_tasks", nil, body)
	if err != nil {
		return "", err
	}

	ticket := common.GetString(data, "ticket")
	if ticket == "" {
		return "", output.Errorf(output.ExitAPI, "api_error", "export task created but ticket is missing")
	}
	return ticket, nil
}

// getDriveExportStatus fetches the current backend state for a previously
// created export task.
func getDriveExportStatus(runtime *common.RuntimeContext, token, ticket string) (driveExportStatus, error) {
	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/export_tasks/%s", validate.EncodePathSegment(ticket)),
		map[string]interface{}{"token": token},
		nil,
	)
	if err != nil {
		return driveExportStatus{}, err
	}
	return parseDriveExportStatus(ticket, data), nil
}

// parseDriveExportStatus accepts the wrapped export result and normalizes the
// subset of fields used by the shortcut.
func parseDriveExportStatus(ticket string, data map[string]interface{}) driveExportStatus {
	result := common.GetMap(data, "result")
	status := driveExportStatus{
		Ticket: ticket,
	}
	if result == nil {
		// Keep the ticket even when the result body is missing so callers can
		// still show a resumable task reference.
		return status
	}

	status.FileExtension = common.GetString(result, "file_extension")
	status.DocType = common.GetString(result, "type")
	status.FileName = common.GetString(result, "file_name")
	status.FileToken = common.GetString(result, "file_token")
	status.JobErrorMsg = common.GetString(result, "job_error_msg")
	status.FileSize = int64(common.GetFloat(result, "file_size"))
	status.JobStatus = int(common.GetFloat(result, "job_status"))
	return status
}

// fetchDriveMetaTitle looks up the document title so exported files can use a
// human-readable default name when possible.
func fetchDriveMetaTitle(runtime *common.RuntimeContext, token, docType string) (string, error) {
	data, err := runtime.CallAPI(
		"POST",
		"/open-apis/drive/v1/metas/batch_query",
		nil,
		map[string]interface{}{
			"request_docs": []map[string]interface{}{
				{
					"doc_token": token,
					"doc_type":  docType,
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	metas := common.GetSlice(data, "metas")
	if len(metas) == 0 {
		return "", nil
	}
	meta, _ := metas[0].(map[string]interface{})
	return common.GetString(meta, "title"), nil
}

// saveContentToOutputDir validates the target path, enforces overwrite policy,
// and writes the payload atomically via FileIO.Save.
func saveContentToOutputDir(fio fileio.FileIO, outputDir, fileName string, payload []byte, overwrite bool) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}

	// Sanitize both the filename and the combined output path so caller-provided
	// names cannot escape the requested output directory.
	safeName := sanitizeExportFileName(fileName, "export.bin")
	target := filepath.Join(outputDir, safeName)

	// Overwrite check via FileIO.Stat
	if !overwrite {
		if _, statErr := fio.Stat(target); statErr == nil {
			return "", output.ErrValidation("output file already exists: %s (use --overwrite to replace)", target)
		}
	}

	if _, err := fio.Save(target, fileio.SaveOptions{}, bytes.NewReader(payload)); err != nil {
		return "", common.WrapSaveErrorByCategory(err, "io")
	}
	resolvedPath, _ := fio.ResolvePath(target)
	if resolvedPath == "" {
		resolvedPath = target
	}
	return resolvedPath, nil
}

// downloadDriveExportFile downloads the exported artifact, derives a safe local
// file name, and returns metadata about the saved file.
func downloadDriveExportFile(ctx context.Context, runtime *common.RuntimeContext, fileToken, outputDir, preferredName string, overwrite bool) (map[string]interface{}, error) {
	if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
		return nil, output.ErrValidation("%s", err)
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
	}, larkcore.WithFileDownload())
	if err != nil {
		return nil, output.ErrNetwork("download failed: %s", err)
	}
	if apiResp.StatusCode >= 400 {
		return nil, output.ErrNetwork("download failed: HTTP %d: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}

	fileName := strings.TrimSpace(preferredName)
	if fileName == "" {
		// Fall back to the server-provided download name when the caller did not
		// request an explicit local file name.
		fileName = client.ResolveFilename(apiResp)
	}
	savedPath, err := saveContentToOutputDir(runtime.FileIO(), outputDir, fileName, apiResp.RawBody, overwrite)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"file_token":   fileToken,
		"file_name":    filepath.Base(savedPath),
		"saved_path":   savedPath,
		"size_bytes":   len(apiResp.RawBody),
		"content_type": apiResp.Header.Get("Content-Type"),
	}, nil
}

// sanitizeExportFileName strips path traversal and unsupported characters while
// preserving a readable file name when possible.
func sanitizeExportFileName(name, fallback string) string {
	name = strings.TrimSpace(filepath.Base(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = fallback
	}

	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
		"\n", "_", "\r", "_", "\t", "_", "\x00", "_",
	)
	name = replacer.Replace(name)
	name = strings.Trim(name, ". ")
	if name == "" {
		return fallback
	}
	return name
}

// ensureExportFileExtension appends the expected local suffix when the chosen
// file name does not already end with the export format's extension.
func ensureExportFileExtension(name, fileExtension string) string {
	expected := exportFileSuffix(fileExtension)
	if expected == "" {
		return name
	}
	if strings.EqualFold(filepath.Ext(name), expected) {
		return name
	}
	return name + expected
}

// exportFileSuffix maps shortcut-level export formats to the local filename
// suffix written to disk.
func exportFileSuffix(fileExtension string) string {
	switch fileExtension {
	case "markdown":
		return ".md"
	case "docx":
		return ".docx"
	case "pdf":
		return ".pdf"
	case "xlsx":
		return ".xlsx"
	case "csv":
		return ".csv"
	case "base":
		return ".base"
	default:
		return ""
	}
}
