// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
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
		//"docs:document.content:read",
		"docs:document:export",
		//"drive:drive.metadata:readonly",
		//"drive:export:readonly",
	},
	AuthTypes: []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "token", Desc: "source document token", Required: true},
		{Name: "doc-type", Desc: "source document type: doc | docx | sheet | bitable", Required: true, Enum: []string{"doc", "docx", "sheet", "bitable"}},
		{Name: "file-extension", Desc: "export format: docx | pdf | xlsx | csv | markdown", Required: true, Enum: []string{"docx", "pdf", "xlsx", "csv", "markdown"}},
		{Name: "sub-id", Desc: "sub-table/sheet ID, required when exporting sheet/bitable as csv"},
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
			return common.NewDryRunAPI().
				Desc("2-step orchestration: fetch docx markdown -> write local file").
				GET("/open-apis/docs/v1/content").
				Params(map[string]interface{}{
					"doc_token":    spec.Token,
					"doc_type":     "docx",
					"content_type": "markdown",
				})
		}

		body := map[string]interface{}{
			"token":          spec.Token,
			"type":           spec.DocType,
			"file_extension": spec.FileExtension,
		}
		if strings.TrimSpace(spec.SubID) != "" {
			body["sub_id"] = spec.SubID
		}

		return common.NewDryRunAPI().
			Desc("3-step orchestration: create export task -> limited polling -> download file").
			POST("/open-apis/drive/v1/export_tasks").
			Body(body)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec := driveExportSpec{
			Token:         runtime.Str("token"),
			DocType:       runtime.Str("doc-type"),
			FileExtension: runtime.Str("file-extension"),
			SubID:         runtime.Str("sub-id"),
		}
		outputDir := runtime.Str("output-dir")
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

		title, err := fetchDriveMetaTitle(runtime, spec.Token, spec.DocType)
		if err != nil {
			fmt.Fprintf(runtime.IO().ErrOut, "Warning: could not fetch document title (%v); using token as filename\n", err)
			title = ""
		}
			}
			fileName := ensureExportFileExtension(sanitizeExportFileName(title, spec.Token), spec.FileExtension)
			savedPath, err := saveContentToOutputDir(outputDir, fileName, []byte(common.GetString(data, "content")), overwrite)
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
		// Keep the command responsive by polling for a bounded window. If the task
		// is still running after that, return a resume command instead of blocking.
		for attempt := 1; attempt <= driveExportPollAttempts; attempt++ {
			if attempt > 1 {
				time.Sleep(driveExportPollInterval)
			}

			status, err := getDriveExportStatus(runtime, spec.Token, ticket)
			if err != nil {
				// Treat polling failures as transient so short-lived backend hiccups
				// do not immediately fail an otherwise healthy export task.
				fmt.Fprintf(runtime.IO().ErrOut, "Export status attempt %d/%d failed: %v\n", attempt, driveExportPollAttempts, err)
				continue
			}
			lastStatus = status

			if status.Ready() {
				fmt.Fprintf(runtime.IO().ErrOut, "Export task completed: %s\n", common.MaskToken(status.FileToken))
				fileName := ensureExportFileExtension(sanitizeExportFileName(status.FileName, spec.Token), spec.FileExtension)
				out, err := downloadDriveExportFile(ctx, runtime, status.FileToken, outputDir, fileName, overwrite)
				if err != nil {
					return err
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
		// Return the last observed status so callers can resume from a known task
		// state instead of losing all progress information on timeout.
		runtime.Out(map[string]interface{}{
			"ticket":          ticket,
			"token":           spec.Token,
			"doc_type":        spec.DocType,
			"file_extension":  spec.FileExtension,
			"ready":           false,
			"job_status":      lastStatus.StatusLabel(),
			"job_status_code": lastStatus.JobStatus,
			"timed_out":       true,
			"next_command":    nextCommand,
		}, nil)
		fmt.Fprintf(runtime.IO().ErrOut, "Export task is still in progress. Continue with: %s\n", nextCommand)
		return nil
	},
}
