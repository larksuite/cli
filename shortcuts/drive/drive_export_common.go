// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	driveExportPollAttempts = 10
	driveExportPollInterval = 5 * time.Second
)

type driveExportSpec struct {
	Token         string
	DocType       string
	FileExtension string
	SubID         string
}

func driveExportTaskResultCommand(ticket, fileToken string) string {
	return fmt.Sprintf("lark-cli drive +task_result --scenario export --ticket %s --file-token %s", ticket, fileToken)
}

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
	return s.JobStatus == 1 || s.JobStatus == 2 || s.JobStatus == 0 && s.FileToken == ""
}

func (s driveExportStatus) Failed() bool {
	return !s.Ready() && !s.Pending() && s.JobStatus != 0
}

func (s driveExportStatus) StatusLabel() string {
	switch s.JobStatus {
	case 0:
		return "success"
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
		if s.JobStatus == 0 {
			return "success"
		}
		if s.JobStatus == 0 && s.FileToken == "" {
			return "unknown"
		}
		return fmt.Sprintf("status_%d", s.JobStatus)
	}
}

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
	case "docx", "pdf", "xlsx", "csv", "markdown":
	default:
		return output.ErrValidation("invalid --file-extension %q: allowed values are docx, pdf, xlsx, csv, markdown", spec.FileExtension)
	}

	if spec.FileExtension == "markdown" && spec.DocType != "docx" {
		return output.ErrValidation("--file-extension markdown only supports --doc-type docx")
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

func parseDriveExportStatus(ticket string, data map[string]interface{}) driveExportStatus {
	result := common.GetMap(data, "result")
	status := driveExportStatus{
		Ticket: ticket,
	}
	if result == nil {
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

func saveContentToOutputDir(outputDir, fileName string, payload []byte, overwrite bool) (string, error) {
	if outputDir == "" {
		outputDir = "."
	}

	safeName := sanitizeExportFileName(fileName, "export.bin")
	target := filepath.Join(outputDir, safeName)
	safePath, err := validate.SafeOutputPath(target)
	if err != nil {
		return "", output.ErrValidation("unsafe output path: %s", err)
	}
	if err := common.EnsureWritableFile(safePath, overwrite); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		return "", output.Errorf(output.ExitInternal, "io", "cannot create output directory: %s", err)
	}
	if err := validate.AtomicWrite(safePath, payload, 0644); err != nil {
		return "", output.Errorf(output.ExitInternal, "io", "cannot write file: %s", err)
	}
	return safePath, nil
}

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
		fileName = client.ResolveFilename(apiResp)
	}
	savedPath, err := saveContentToOutputDir(outputDir, fileName, apiResp.RawBody, overwrite)
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
	default:
		return ""
	}
}
