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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

var (
	driveExportPollAttempts = 10
	driveExportPollInterval = 5 * time.Second
)

const (
	driveExportResolvedDocTypeValues = "doc, docx, sheet, bitable, slides"
	driveExportInputDocTypeValues    = driveExportResolvedDocTypeValues + ", wiki"
	driveExportFileExtensionValues   = "docx, pdf, xlsx, csv, markdown, base, pptx"
)

// driveExportSpec contains the normalized export request understood by the
// shortcut and the underlying export task APIs.
type driveExportSpec struct {
	URL           string
	Token         string
	DocType       string
	FileExtension string
	SubID         string
	OnlySchema    bool
}

type driveExportInputSource struct {
	Type  string
	Token string
	Param string
}

type driveExportWikiResolution struct {
	Resolved  bool
	WikiToken string
	ObjToken  string
	ObjType   string
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
	normalized, source, err := normalizeDriveExportSpecInput(spec)
	if err != nil {
		return err
	}
	return validateDriveExportNormalizedSpecForSource(normalized, source)
}

func validateDriveExportNormalizedSpec(spec driveExportSpec) error {
	switch spec.DocType {
	case "doc", "docx", "sheet", "bitable", "slides":
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --doc-type %q: allowed values are %s", spec.DocType, driveExportInputDocTypeValues).
			WithParam("--doc-type").
			WithHint("use --url when you have a document URL; use --doc-type wiki only with a bare Wiki node token so the CLI can resolve the underlying document type")
	}

	if err := validate.ResourceName(spec.Token, "--token"); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
	}

	switch spec.FileExtension {
	case "docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx":
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --file-extension %q: allowed values are %s", spec.FileExtension, driveExportFileExtensionValues).
			WithParam("--file-extension").
			WithHint("choose an export format supported by the source type; common choices are docx/pdf for docs, xlsx/csv for sheets, xlsx/csv/base for bitable, and pptx/pdf for slides")
	}

	if err := validateDriveExportFormatCompatibility(spec); err != nil {
		return err
	}

	if spec.OnlySchema && (spec.DocType != "bitable" || spec.FileExtension != "base") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--only-schema is only used when exporting bitable as base").
			WithParam("--only-schema").
			WithHint("retry with --doc-type bitable --file-extension base, or remove --only-schema")
	}

	if strings.TrimSpace(spec.SubID) != "" {
		if spec.FileExtension != "csv" || (spec.DocType != "sheet" && spec.DocType != "bitable") {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--sub-id is only used when exporting sheet/bitable as csv").
				WithParam("--sub-id").
				WithHint("remove --sub-id, or retry with --doc-type sheet|bitable --file-extension csv")
		}
		if err := validate.ResourceName(spec.SubID, "--sub-id"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--sub-id")
		}
	}

	if spec.FileExtension == "csv" && (spec.DocType == "sheet" || spec.DocType == "bitable") && strings.TrimSpace(spec.SubID) == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--sub-id is required when exporting sheet/bitable as csv").
			WithParam("--sub-id").
			WithHint("retry with --sub-id <sheet_id_or_table_id>; if you need the whole workbook, use --file-extension xlsx instead")
	}

	return nil
}

func validateDriveExportFormatCompatibility(spec driveExportSpec) error {
	if driveExportFileExtensionAllowedForDocType(spec.DocType, spec.FileExtension) {
		return nil
	}
	allowed := strings.Join(driveExportAllowedFileExtensions(spec.DocType), ", ")
	return errs.NewValidationError(
		errs.SubtypeInvalidArgument,
		"unsupported export format: --doc-type %s cannot be exported as %s",
		spec.DocType,
		spec.FileExtension,
	).
		WithParam("--file-extension").
		WithHint("retry with --file-extension %s. If the token came from a URL, prefer --url so the CLI infers the correct source type before validating the export format", allowed)
}

func driveExportFileExtensionAllowedForDocType(docType, fileExtension string) bool {
	for _, allowed := range driveExportAllowedFileExtensions(docType) {
		if fileExtension == allowed {
			return true
		}
	}
	return false
}

func driveExportAllowedFileExtensions(docType string) []string {
	switch normalizeDriveExportDocType(docType) {
	case "doc":
		return []string{"docx", "pdf"}
	case "docx":
		return []string{"docx", "pdf", "markdown"}
	case "sheet":
		return []string{"xlsx", "csv"}
	case "bitable":
		return []string{"xlsx", "csv", "base"}
	case "slides":
		return []string{"pptx", "pdf"}
	default:
		return []string{"docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx"}
	}
}

func validateDriveExportNormalizedSpecForSource(spec driveExportSpec, source driveExportInputSource) error {
	if source.Type == "wiki" && spec.DocType == "" {
		return validateDriveExportPendingWikiSpec(spec, source)
	}
	return validateDriveExportNormalizedSpec(spec)
}

func validateDriveExportPendingWikiSpec(spec driveExportSpec, source driveExportInputSource) error {
	param := source.Param
	if param == "" {
		param = "--token"
	}
	if err := validate.ResourceName(spec.Token, param); err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam(param)
	}

	switch spec.FileExtension {
	case "docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx":
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --file-extension %q: allowed values are %s", spec.FileExtension, driveExportFileExtensionValues).
			WithParam("--file-extension").
			WithHint("Wiki export format is validated after resolving the Wiki node; choose a format normally supported by the underlying document type")
	}
	if spec.OnlySchema && spec.FileExtension != "base" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--only-schema is only used when exporting bitable as base").
			WithParam("--only-schema").
			WithHint("retry with --file-extension base, or remove --only-schema")
	}
	if strings.TrimSpace(spec.SubID) != "" && spec.FileExtension != "csv" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--sub-id is only used when exporting sheet/bitable as csv").
			WithParam("--sub-id").
			WithHint("remove --sub-id, or retry with --file-extension csv if the Wiki node resolves to a sheet/bitable")
	}
	if strings.TrimSpace(spec.SubID) != "" {
		if err := validate.ResourceName(spec.SubID, "--sub-id"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--sub-id")
		}
	}
	return nil
}

func normalizeDriveExportSpecInput(spec driveExportSpec) (driveExportSpec, driveExportInputSource, error) {
	spec.URL = strings.TrimSpace(spec.URL)
	spec.Token = strings.TrimSpace(spec.Token)
	spec.DocType = strings.ToLower(strings.TrimSpace(spec.DocType))
	spec.FileExtension = strings.ToLower(strings.TrimSpace(spec.FileExtension))

	if spec.Token == "" && spec.URL == "" {
		return spec, driveExportInputSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "either --url or --token is required").WithParam("--url")
	}
	if spec.Token != "" && spec.URL != "" {
		return spec, driveExportInputSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url and --token are mutually exclusive").WithParam("--url")
	}

	source := driveExportInputSource{
		Type:  spec.DocType,
		Token: spec.Token,
		Param: "--token",
	}

	rawInput := spec.Token
	inputParam := "--token"
	if spec.URL != "" {
		rawInput = spec.URL
		inputParam = "--url"
	}

	if ref, ok := common.ParseResourceURL(rawInput); ok {
		refType := normalizeDriveExportDocType(ref.Type)
		source = driveExportInputSource{
			Type:  refType,
			Token: ref.Token,
			Param: inputParam,
		}
		spec.Token = ref.Token
		if refType != "wiki" {
			if !isDriveExportDocType(refType) {
				return spec, source, errs.NewValidationError(
					errs.SubtypeInvalidArgument,
					"%s URL type %q is not supported by drive +export; use a doc/docx/sheet/base/slides/wiki URL or token",
					inputParam,
					ref.Type,
				).WithParam(inputParam)
			}
			if spec.DocType == "wiki" {
				return spec, source, errs.NewValidationError(
					errs.SubtypeInvalidArgument,
					"--doc-type wiki conflicts with %s URL type %q",
					inputParam,
					refType,
				).
					WithParam("--doc-type").
					WithHint("remove --doc-type when passing --url; the CLI will infer %q from the URL", refType)
			}
			if spec.DocType != "" && spec.DocType != refType {
				return spec, source, errs.NewValidationError(
					errs.SubtypeInvalidArgument,
					"--doc-type %q conflicts with %s URL type %q",
					spec.DocType,
					inputParam,
					refType,
				).WithParam("--doc-type")
			}
			spec.DocType = refType
		} else if spec.DocType == "wiki" {
			spec.DocType = ""
		}
		return spec, source, nil
	}

	if strings.Contains(rawInput, "://") {
		return spec, source, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"unsupported %s URL %q: use a recognized Lark document URL",
			inputParam,
			rawInput,
		).WithParam(inputParam)
	}
	if spec.URL != "" {
		return spec, source, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"unsupported --url %q: use a recognized Lark document URL",
			spec.URL,
		).WithParam("--url")
	}
	if spec.DocType == "" {
		return spec, source, errs.NewValidationError(errs.SubtypeInvalidArgument, "--doc-type is required when --token is a bare token (allowed: %s)", driveExportInputDocTypeValues).
			WithParam("--doc-type").
			WithHint("if you have the original document link, prefer --url <document_url>; if this is a Wiki node token, use --doc-type wiki")
	}
	if spec.DocType == "wiki" {
		source.Type = "wiki"
		source.Token = spec.Token
		spec.DocType = ""
	}
	return spec, source, nil
}

func normalizeDriveExportDocType(docType string) string {
	switch strings.ToLower(strings.TrimSpace(docType)) {
	case "base":
		return "bitable"
	default:
		return strings.ToLower(strings.TrimSpace(docType))
	}
}

func isDriveExportDocType(docType string) bool {
	switch normalizeDriveExportDocType(docType) {
	case "doc", "docx", "sheet", "bitable", "slides":
		return true
	default:
		return false
	}
}

func buildDriveExportTaskBody(spec driveExportSpec) map[string]interface{} {
	body := map[string]interface{}{
		"token":          spec.Token,
		"type":           spec.DocType,
		"file_extension": spec.FileExtension,
	}
	if strings.TrimSpace(spec.SubID) != "" {
		body["sub_id"] = spec.SubID
	}
	if spec.OnlySchema {
		body["only_schema"] = true
	}
	return body
}

// createDriveExportTask starts the asynchronous export job and returns its
// ticket for subsequent polling.
func createDriveExportTask(runtime *common.RuntimeContext, spec driveExportSpec) (string, error) {
	data, err := runtime.CallAPITyped("POST", "/open-apis/drive/v1/export_tasks", nil, buildDriveExportTaskBody(spec))
	if err != nil {
		return "", err
	}

	ticket := common.GetString(data, "ticket")
	if ticket == "" {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse, "export task created but ticket is missing")
	}
	return ticket, nil
}

func resolveDriveExportWikiSource(ctx context.Context, runtime *common.RuntimeContext, spec driveExportSpec, wikiToken string) (driveExportSpec, driveExportWikiResolution, error) {
	wikiToken = strings.TrimSpace(wikiToken)
	if err := validate.ResourceName(wikiToken, "--token"); err != nil {
		return spec, driveExportWikiResolution{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--token")
	}

	fmt.Fprintf(runtime.IO().ErrOut, "Resolving wiki node for export: %s\n", common.MaskToken(wikiToken))
	data, err := driveInspectCallWithRetry(ctx, func() (map[string]interface{}, error) {
		return runtime.CallAPITyped(
			"GET",
			"/open-apis/wiki/v2/spaces/get_node",
			map[string]interface{}{"token": wikiToken},
			nil,
		)
	})
	if err != nil {
		return spec, driveExportWikiResolution{}, err
	}

	node := common.GetMap(data, "node")
	objType := normalizeDriveExportDocType(common.GetString(node, "obj_type"))
	objToken := common.GetString(node, "obj_token")
	if objType == "" || objToken == "" {
		return spec, driveExportWikiResolution{}, errs.NewInternalError(errs.SubtypeInvalidResponse, "wiki get_node returned incomplete node data (obj_type=%q, obj_token=%q)", objType, objToken)
	}
	if !isDriveExportDocType(objType) {
		return spec, driveExportWikiResolution{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"wiki resolved to %q, but drive +export only supports doc, docx, sheet, bitable, and slides",
			objType,
		).WithParam("--token")
	}
	if spec.DocType != "" && spec.DocType != objType {
		return spec, driveExportWikiResolution{}, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"wiki resolved to %q, but --doc-type is %q; use --doc-type %s",
			objType,
			spec.DocType,
			objType,
		).WithParam("--doc-type")
	}

	spec.Token = objToken
	spec.DocType = objType
	if err := validateDriveExportNormalizedSpec(spec); err != nil {
		return spec, driveExportWikiResolution{}, err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Resolved wiki to %s: %s\n", objType, common.MaskToken(objToken))
	return spec, driveExportWikiResolution{
		Resolved:  true,
		WikiToken: wikiToken,
		ObjToken:  objToken,
		ObjType:   objType,
	}, nil
}

func createDriveExportTaskResolvingWiki(ctx context.Context, runtime *common.RuntimeContext, spec driveExportSpec, source driveExportInputSource) (string, driveExportSpec, driveExportWikiResolution, error) {
	if source.Type == "wiki" {
		resolvedSpec, resolution, err := resolveDriveExportWikiSource(ctx, runtime, spec, source.Token)
		if err != nil {
			return "", spec, resolution, err
		}
		ticket, err := createDriveExportTask(runtime, resolvedSpec)
		return ticket, resolvedSpec, resolution, err
	}

	ticket, err := createDriveExportTask(runtime, spec)
	if err != nil {
		return "", spec, driveExportWikiResolution{}, err
	}
	return ticket, spec, driveExportWikiResolution{}, nil
}

// getDriveExportStatus fetches the current backend state for a previously
// created export task.
func getDriveExportStatus(runtime *common.RuntimeContext, token, ticket string) (driveExportStatus, error) {
	data, err := runtime.CallAPITyped(
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
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "output file already exists: %s (use --overwrite to replace)", target)
		}
	}

	if _, err := fio.Save(target, fileio.SaveOptions{}, bytes.NewReader(payload)); err != nil {
		return "", driveSaveError(err)
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
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--file-token")
	}

	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
	}, larkcore.WithFileDownload())
	if err != nil {
		return nil, wrapDriveNetworkErr(err, "download failed: %s", err)
	}
	if apiResp.StatusCode >= 400 {
		subtype := errs.SubtypeNetworkTransport
		if apiResp.StatusCode >= 500 {
			subtype = errs.SubtypeNetworkServer
		}
		e := errs.NewNetworkError(subtype, "download failed: HTTP %d: %s", apiResp.StatusCode, string(apiResp.RawBody)).WithCode(apiResp.StatusCode)
		// Mirror internal/client streamLogID: fall back to the request-id header
		// when log-id is absent so the diagnostic ID is still populated.
		logID := strings.TrimSpace(apiResp.Header.Get(larkcore.HttpHeaderKeyLogId))
		if logID == "" {
			logID = strings.TrimSpace(apiResp.Header.Get(larkcore.HttpHeaderKeyRequestId))
		}
		if logID != "" {
			e = e.WithLogID(logID)
		}
		return nil, e
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
	case "pptx":
		return ".pptx"
	default:
		return ""
	}
}
