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

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// DriveExport exports Drive-native documents to local files and falls back to
// a follow-up command when the async export task does not finish in time.
var DriveExport = common.Shortcut{
	Service:     "drive",
	Command:     "+export",
	Description: "Export a doc/docx/sheet/bitable to a local file with limited polling",
	Risk:        "read",
	Scopes: []string{
		"docs:document.content:read",
		"docs:document:export",
		"drive:drive.metadata:readonly",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "source document token", Required: true},
		{Name: "doc-type", Desc: "source document type: doc | docx | sheet | bitable", Required: true, Enum: []string{"doc", "docx", "sheet", "bitable"}},
		{Name: "file-extension", Desc: "export format: docx | pdf | xlsx | csv | markdown | base (bitable only)", Required: true, Enum: []string{"docx", "pdf", "xlsx", "csv", "markdown", "base"}},
		{Name: "sub-id", Desc: "sub-table/sheet ID, required when exporting sheet/bitable as csv"},
		{Name: "file-name", Desc: "preferred output filename (optional)"},
		{Name: "output-dir", Default: ".", Desc: "local output directory (default: current directory)"},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		return validateDriveExportSpec(driveExportSpec{
			Token:         runtime.Str("token"),
			DocType:       runtime.Str("doc-type"),
			FileExtension: runtime.Str("file-extension"),
			SubID:         runtime.Str("sub-id"),
		})
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec := driveExportSpec{
			Token:         runtime.Str("token"),
			DocType:       runtime.Str("doc-type"),
			FileExtension: runtime.Str("file-extension"),
			SubID:         runtime.Str("sub-id"),
		}
		// Markdown export is a special case: docx markdown comes from docs content
		// directly instead of the Drive export task API.
		if spec.FileExtension == "markdown" {
			dr := common.NewDryRunAPI().
				Desc("2-step orchestration: fetch docx markdown -> write local file").
				GET("/open-apis/docs/v1/content").
				Params(map[string]interface{}{
					"doc_token":    spec.Token,
					"doc_type":     "docx",
					"content_type": "markdown",
				}).
				Set("output_dir", runtime.Str("output-dir"))
			if name := strings.TrimSpace(runtime.Str("file-name")); name != "" {
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

		dr := common.NewDryRunAPI().
			Desc("3-step orchestration: create export task -> limited polling -> download file").
			POST("/open-apis/drive/v1/export_tasks").
			Body(body).
			Set("output_dir", runtime.Str("output-dir"))
		if name := strings.TrimSpace(runtime.Str("file-name")); name != "" {
			dr.Set("file_name", ensureExportFileExtension(sanitizeExportFileName(name, spec.Token), spec.FileExtension))
		}
		return dr
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveExportSpec{
			Token:         runtime.Str("token"),
			DocType:       runtime.Str("doc-type"),
			FileExtension: runtime.Str("file-extension"),
			SubID:         runtime.Str("sub-id"),
		}
		outputDir := runtime.Str("output-dir")
		preferredFileName := strings.TrimSpace(runtime.Str("file-name"))
		overwrite := runtime.Bool("overwrite")

		// Markdown export bypasses the async export task and writes the fetched
		// markdown content directly to disk.
		if spec.FileExtension == "markdown" {
			fmt.Fprintf(runtime.IO().ErrOut, "Exporting docx as markdown: %s\n", common.MaskToken(spec.Token))
			data, err := runtime.CallAPI(
				"GET",
				"/open-apis/docs/v1/content",
				map[string]interface{}{
					"doc_token":    spec.Token,
					"doc_type":     "docx",
					"content_type": "markdown",
				},
				nil,
			)
			if err != nil {
				return err
			}

			fileName := preferredFileName
			if fileName == "" {
				// Prefer the remote title for the exported file name, but still fall
				// back to the token if metadata is empty.
				title, err := fetchDriveMetaTitle(runtime, spec.Token, spec.DocType)
				if err != nil {
					fmt.Fprintf(runtime.IO().ErrOut, "Title lookup failed, using token as filename: %v\n", err)
					title = spec.Token
				}
				fileName = title
			}
			fileName = ensureExportFileExtension(sanitizeExportFileName(fileName, spec.Token), spec.FileExtension)
			savedPath, err := saveContentToOutputDir(runtime.FileIO(), outputDir, fileName, []byte(common.GetString(data, "content")), overwrite)
			if err != nil {
				return err
			}

			runtime.Out(map[string]interface{}{
				"token":          spec.Token,
				"doc_type":       spec.DocType,
				"file_extension": spec.FileExtension,
				"file_name":      filepath.Base(savedPath),
				"saved_path":     savedPath,
				"size_bytes":     len([]byte(common.GetString(data, "content"))),
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
					return ctx.Err()
				case <-time.After(driveExportPollInterval):
				}
			}
			if err := ctx.Err(); err != nil {
				return err
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
					var exitErr *output.ExitError
					if errors.As(err, &exitErr) && exitErr.Detail != nil {
						return output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
					}
					return output.ErrWithHint(output.ExitAPI, "api_error", err.Error(), hint)
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
				return output.Errorf(output.ExitAPI, "api_error", "export task failed: %s (ticket=%s)", msg, ticket)
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
			var exitErr *output.ExitError
			if errors.As(lastPollErr, &exitErr) && exitErr.Detail != nil {
				if strings.TrimSpace(exitErr.Detail.Hint) != "" {
					hint = exitErr.Detail.Hint + "\n" + hint
				}
				return output.ErrWithHint(exitErr.Code, exitErr.Detail.Type, exitErr.Detail.Message, hint)
			}
			return output.ErrWithHint(output.ExitAPI, "api_error", lastPollErr.Error(), hint)
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
	},
}
