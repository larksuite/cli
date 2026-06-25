// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// wrapExportContextErr converts a context cancellation / deadline error into a
// typed errs.NetworkError so the cobra layer sees a typed envelope (with cause
// preserved for errors.Is) instead of an untyped context.Canceled /
// context.DeadlineExceeded escaping as a plain string. CR-flagged hole on the
// poll loop: returning ctx.Err() directly bypassed the typed-error contract.
func wrapExportContextErr(err error) error {
	if err == nil {
		return nil
	}
	subtype := errs.SubtypeNetworkTransport
	msg := "drive +export polling cancelled: %s"
	if errors.Is(err, context.DeadlineExceeded) {
		subtype = errs.SubtypeNetworkTimeout
		msg = "drive +export polling deadline exceeded: %s"
	}
	return errs.NewNetworkError(subtype, msg, err).WithCause(err)
}

// DriveExport exports Drive-native documents to local files and falls back to
// a follow-up command when the async export task does not finish in time.
var DriveExport = common.Shortcut{
	Service:     "drive",
	Command:     "+export",
	Description: "Export a doc/docx/sheet/bitable/slides to a local file with limited polling",
	Risk:        "read",
	Scopes: []string{
		"docs:document.content:read",
		"docs:document:export",
		"docx:document:readonly",
		"drive:drive.metadata:readonly",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "source document token", Required: true},
		{Name: "doc-type", Desc: "source document type: doc | docx | sheet | bitable | slides", Required: true, Enum: []string{"doc", "docx", "sheet", "bitable", "slides"}},
		{Name: "file-extension", Desc: "export format: docx | pdf | xlsx | csv | markdown | base (bitable only) | pptx (slides only)", Required: true, Enum: []string{"docx", "pdf", "xlsx", "csv", "markdown", "base", "pptx"}},
		{Name: "sub-id", Desc: "sub-table/sheet ID, required when exporting sheet/bitable as csv"},
		{Name: "only-schema", Type: "bool", Desc: "export only bitable schema when --doc-type bitable --file-extension base"},
		{Name: "file-name", Desc: "preferred output filename (optional)"},
		{Name: "output-dir", Default: ".", Desc: "local output directory (default: current directory)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateExport(exportParamsFromFlags(runtime))
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return PlanExportDryRun(runtime, exportParamsFromFlags(runtime))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return RunExport(ctx, runtime, exportParamsFromFlags(runtime))
	},
}

// ExportParams holds the user-facing inputs for an export flow, decoupled from
// cobra flags so other command groups (e.g. sheets +workbook-export) can reuse
// the drive export implementation. An empty OutputDir means "create the export
// task and poll, but do not download" — callers that only need the ready file
// token / status get it back without writing a local file.
type ExportParams struct {
	Token         string
	DocType       string
	FileExtension string
	SubID         string
	OnlySchema    bool
	OutputDir     string
	FileName      string
	Overwrite     bool
}

func (p ExportParams) spec() driveExportSpec {
	return driveExportSpec{
		Token:         p.Token,
		DocType:       p.DocType,
		FileExtension: p.FileExtension,
		SubID:         p.SubID,
		OnlySchema:    p.OnlySchema,
	}
}

// exportParamsFromFlags reads the standard drive +export flag set.
func exportParamsFromFlags(runtime *common.RuntimeContext) ExportParams {
	// drive +export always downloads; an empty --output-dir historically means
	// the current directory (saveContentToOutputDir maps "" -> "."), so normalize
	// it here to keep behavior identical and stay off the export-only ("" => skip
	// download) path that only sheets +workbook-export uses.
	outputDir := runtime.Str("output-dir")
	if outputDir == "" {
		outputDir = "."
	}
	return ExportParams{
		Token:         runtime.Str("token"),
		DocType:       runtime.Str("doc-type"),
		FileExtension: runtime.Str("file-extension"),
		SubID:         runtime.Str("sub-id"),
		OnlySchema:    runtime.Bool("only-schema"),
		OutputDir:     outputDir,
		FileName:      strings.TrimSpace(runtime.Str("file-name")),
		Overwrite:     runtime.Bool("overwrite"),
	}
}

// validateExport runs the CLI-level export constraint checks. Unexported because
// only drive +export's Validate consumes it directly; sheets +workbook-export
// reuses RunExport / PlanExportDryRun but inlines its own (sheet-specific)
// validation, so there is no cross-package call site to keep exported.
func validateExport(p ExportParams) error {
	return validateDriveExportSpec(p.spec())
}

// PlanExportDryRun builds the dry-run plan for an export without performing I/O.
func PlanExportDryRun(runtime *common.RuntimeContext, p ExportParams) *common.DryRunAPI {
	spec := p.spec()
	// Markdown export is a special case: docx markdown comes from the V2
	// docs_ai fetch API directly instead of the Drive export task API.
	if spec.FileExtension == "markdown" {
		apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", validate.EncodePathSegment(spec.Token))
		dr := common.NewDryRunAPI().
			Desc("2-step orchestration: fetch docx markdown -> write local file").
			POST(apiPath).
			Body(map[string]interface{}{
				"format": "markdown",
			}).
			Set("output_dir", p.OutputDir)
		if name := strings.TrimSpace(p.FileName); name != "" {
			dr.Set("file_name", ensureExportFileExtension(sanitizeExportFileName(name, spec.Token), spec.FileExtension))
		}
		return dr
	}

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

	dr := common.NewDryRunAPI().
		Desc("3-step orchestration: create export task -> limited polling -> download file").
		POST("/open-apis/drive/v1/export_tasks").
		Body(body).
		Set("output_dir", p.OutputDir)
	if name := strings.TrimSpace(p.FileName); name != "" {
		dr.Set("file_name", ensureExportFileExtension(sanitizeExportFileName(name, spec.Token), spec.FileExtension))
	}
	return dr
}

// RunExport drives create export task -> bounded poll -> optional download. It
// is the shared core behind both drive +export and sheets +workbook-export. An
// empty p.OutputDir skips the download step and returns the ready file token.
func RunExport(ctx context.Context, runtime *common.RuntimeContext, p ExportParams) error {
	spec := p.spec()
	outputDir := p.OutputDir
	preferredFileName := strings.TrimSpace(p.FileName)
	overwrite := p.Overwrite

	// Markdown export bypasses the async export task and writes the fetched
	// markdown content directly to disk. Uses the V2 docs_ai fetch API for
	// higher-quality Lark-flavored Markdown output.
	if spec.FileExtension == "markdown" {
		fmt.Fprintf(runtime.IO().ErrOut, "Exporting docx as markdown: %s\n", common.MaskToken(spec.Token))
		apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s/fetch", validate.EncodePathSegment(spec.Token))
		data, err := runtime.CallAPITyped(
			"POST",
			apiPath,
			nil,
			map[string]interface{}{
				"format": "markdown",
			},
		)
		if err != nil {
			return err
		}

		// Extract content from the V2 response: data.document.content
		doc, ok := data["document"].(map[string]interface{})
		if !ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid markdown fetch response: missing document object")
		}
		content, ok := doc["content"].(string)
		if !ok {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "invalid markdown fetch response: missing document.content")
		}

		fileName := preferredFileName
		if fileName == "" {
			// Prefer the remote title for the exported file name, but still fall
			// back to the token if metadata is empty.
			title, err := common.FetchDriveMetaTitle(runtime, spec.Token, spec.DocType)
			if err != nil {
				fmt.Fprintf(runtime.IO().ErrOut, "Title lookup failed, using token as filename: %v\n", err)
				title = spec.Token
			}
			fileName = title
		}
		fileName = ensureExportFileExtension(sanitizeExportFileName(fileName, spec.Token), spec.FileExtension)
		savedPath, err := saveContentToOutputDir(runtime.FileIO(), outputDir, fileName, []byte(content), overwrite)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"token":          spec.Token,
			"doc_type":       spec.DocType,
			"file_extension": spec.FileExtension,
			"file_name":      filepath.Base(savedPath),
			"saved_path":     savedPath,
			"size_bytes":     len(content),
		}, nil)
		return nil
	}

	ticket, err := createDriveExportTask(runtime, spec)
	if err != nil {
		return err
	}
	fmt.Fprintf(runtime.IO().ErrOut, "Created export task: %s\n", ticket)

	var lastStatus driveExportStatus
	var lastPollErr error
	hasObservedStatus := false
	// Keep the command responsive by polling for a bounded window. If the task
	// is still running after that, return a resume command instead of blocking.
	for attempt := 1; attempt <= driveExportPollAttempts; attempt++ {
		if attempt > 1 {
			select {
			case <-ctx.Done():
				return wrapExportContextErr(ctx.Err())
			case <-time.After(driveExportPollInterval):
			}
		}
		if err := ctx.Err(); err != nil {
			return wrapExportContextErr(err)
		}

		status, err := getDriveExportStatus(runtime, spec.Token, ticket)
		if err != nil {
			// Treat polling failures as transient so short-lived backend hiccups
			// do not immediately fail an otherwise healthy export task.
			lastPollErr = err
			fmt.Fprintf(runtime.IO().ErrOut, "Export status attempt %d/%d failed: %v\n", attempt, driveExportPollAttempts, err)
			continue
		}
		lastStatus = status
		hasObservedStatus = true

		if status.Ready() {
			fmt.Fprintf(runtime.IO().ErrOut, "Export task completed: %s\n", common.MaskToken(status.FileToken))

			// Export-only mode: caller wants the ready file token / metadata but
			// no local download (e.g. sheets +workbook-export without an output
			// path). Skip the download and return the status envelope.
			if strings.TrimSpace(outputDir) == "" {
				runtime.Out(map[string]interface{}{
					"ticket":         ticket,
					"token":          spec.Token,
					"doc_type":       spec.DocType,
					"file_extension": spec.FileExtension,
					"file_token":     status.FileToken,
					"file_name":      status.FileName,
					"file_size":      status.FileSize,
					"ready":          true,
					"downloaded":     false,
				}, nil)
				return nil
			}

			fileName := preferredFileName
			if fileName == "" {
				fileName = status.FileName
			}
			fileName = ensureExportFileExtension(sanitizeExportFileName(fileName, spec.Token), spec.FileExtension)
			out, err := downloadDriveExportFile(ctx, runtime, status.FileToken, outputDir, fileName, overwrite)
			if err != nil {
				recoveryCommand := driveExportDownloadCommand(status.FileToken, fileName, outputDir, overwrite)
				hint := fmt.Sprintf(
					"the export artifact is already ready (ticket=%s, file_token=%s)\nretry download with: %s",
					ticket,
					status.FileToken,
					recoveryCommand,
				)
				return appendDriveExportRecoveryHint(err, hint)
			}
			out["ticket"] = ticket
			out["doc_type"] = spec.DocType
			out["file_extension"] = spec.FileExtension
			runtime.Out(out, nil)
			return nil
		}

		if status.Failed() {
			msg := strings.TrimSpace(status.JobErrorMsg)
			if msg == "" {
				msg = status.StatusLabel()
			}
			return errs.NewAPIError(errs.SubtypeServerError, "export task failed: %s (ticket=%s)", msg, ticket)
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Export status %d/%d: %s\n", attempt, driveExportPollAttempts, status.StatusLabel())
	}

	nextCommand := driveExportTaskResultCommand(ticket, spec.Token)
	if !hasObservedStatus && lastPollErr != nil {
		hint := fmt.Sprintf(
			"the export task was created but every status poll failed (ticket=%s)\nretry status lookup with: %s",
			ticket,
			nextCommand,
		)
		return appendDriveExportRecoveryHint(lastPollErr, hint)
	}

	failed := false
	var jobStatus interface{}
	jobStatusLabel := "unknown"
	if hasObservedStatus {
		failed = lastStatus.Failed()
		jobStatus = lastStatus.JobStatus
		jobStatusLabel = lastStatus.StatusLabel()
	}
	// Return the last observed status so callers can resume from a known task
	// state instead of losing all progress information on timeout.
	result := map[string]interface{}{
		"ticket":           ticket,
		"token":            spec.Token,
		"doc_type":         spec.DocType,
		"file_extension":   spec.FileExtension,
		"ready":            false,
		"failed":           failed,
		"job_status":       jobStatus,
		"job_status_label": jobStatusLabel,
		"timed_out":        true,
		"next_command":     nextCommand,
	}
	if preferredFileName != "" {
		result["file_name"] = ensureExportFileExtension(sanitizeExportFileName(preferredFileName, spec.Token), spec.FileExtension)
	}
	runtime.Out(result, nil)
	fmt.Fprintf(runtime.IO().ErrOut, "Export task is still in progress. Continue with: %s\n", nextCommand)
	return nil
}
