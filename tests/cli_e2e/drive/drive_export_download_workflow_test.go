// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_ExportWorkflow tests the export shortcut method.
// Workflow: import a docx -> export it to local file -> verify file content.
func TestDrive_ExportWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Export Test\n\nDocument for testing export shortcut.\nTimestamp: " + suffix

	// Import a doc first
	docToken := importTestDoc(t, parentT, ctx, "export", testContent)
	require.NotEmpty(t, docToken)

	exportDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles", "export-"+suffix)
	_ = os.MkdirAll(exportDir, 0755)
	t.Cleanup(func() {
		os.RemoveAll(exportDir)
	})

	t.Run("export - save to local file", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+export",
				"--token", docToken,
				"--doc-type", "docx",
				"--file-extension", "pdf",
				"--output-dir", exportDir,
				"--overwrite",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// For PDF export, check if file was directly saved or if polling needed
		savedPath := gjson.Get(result.Stdout, "data.saved_path").String()
		if savedPath == "" {
			// Poll for completion if ticket returned (async case)
			ticket := gjson.Get(result.Stdout, "data.ticket").String()
			if ticket != "" {
				pollResult, pollErr := clie2e.RunCmd(ctx, clie2e.Request{
					Args: []string{"drive", "+task_result", "--ticket", ticket, "--scenario", "export", "--file-token", docToken},
				})
				require.NoError(t, pollErr)
				pollResult.AssertExitCode(t, 0)
				pollResult.AssertStdoutStatus(t, true)
			}
		}

		// Verify local file was created
		files, listErr := os.ReadDir(exportDir)
		require.NoError(t, listErr)
		require.NotEmpty(t, files, "export should save file directly to output-dir, stdout:\n%s", result.Stdout)
	})
}

// TestDrive_ExportDownloadWorkflow tests the export-download shortcut method.
// Workflow: import a docx -> export as PDF (creates export task) -> use the returned
// file_token with +export-download to download the exported file.
func TestDrive_ExportDownloadWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "# Lark CLI E2E Export Download Test\n\nDocument for testing export-download shortcut.\nTimestamp: " + suffix

	// Import a doc first
	docToken := importTestDoc(t, parentT, ctx, "export-download", testContent)
	require.NotEmpty(t, docToken)

	// Export as PDF (not markdown) so it goes through the export task and returns a real file_token
	exportDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles", "export-download-"+suffix)
	_ = os.MkdirAll(exportDir, 0755)
	t.Cleanup(func() {
		os.RemoveAll(exportDir)
	})

	var exportedFileToken string

	t.Run("export as PDF to get file_token", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+export",
				"--token", docToken,
				"--doc-type", "docx",
				"--file-extension", "pdf",
				"--output-dir", exportDir,
				"--overwrite",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// Get the exported file token (different from source doc token)
		exportedFileToken = gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, exportedFileToken, "file_token should be returned, stdout:\n%s", result.Stdout)
		t.Logf("Exported file_token: %s", exportedFileToken)
	})

	require.NotEmpty(t, exportedFileToken, "exported file token is required for export-download")

	// Step 2: Use +export-download with the exported file token
	downloadDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles", "export-re-download-"+suffix)
	_ = os.MkdirAll(downloadDir, 0755)
	t.Cleanup(func() {
		os.RemoveAll(downloadDir)
	})

	t.Run("download exported file with export-download", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{
				"drive", "+export-download",
				"--file-token", exportedFileToken,
				"--output-dir", downloadDir,
				"--overwrite",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify downloaded file exists
		downloadedFileName := gjson.Get(result.Stdout, "data.file_name").String()
		require.NotEmpty(t, downloadedFileName, "file_name should be returned, stdout:\n%s", result.Stdout)

		downloadedPath := filepath.Join(downloadDir, downloadedFileName)
		_, statErr := os.Stat(downloadedPath)
		require.NoError(t, statErr, "downloaded file should exist at %s", downloadedPath)
	})
}