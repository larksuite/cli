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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_UploadDownloadWorkflow tests the upload and download shortcut methods.
// Workflow: create temp file -> upload to drive -> download from drive -> verify content.
func TestDrive_UploadDownloadWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "lark-cli-e2e-drive-upload-download-" + suffix

	// Create files in relative path since --file requires relative paths
	testDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles")
	_ = os.MkdirAll(testDir, 0755)

	localFile := filepath.Join(testDir, "drive-e2e-upload-"+suffix+".txt")
	err := os.WriteFile(localFile, []byte(testContent), 0644)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(localFile)
	})

	var uploadedFileToken string

	t.Run("upload", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+upload", "--file", localFile},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		uploadedFileToken = gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, uploadedFileToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			// Best-effort delete the uploaded file
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": uploadedFileToken},
			})
		})
	})

	t.Run("download", func(t *testing.T) {
		require.NotEmpty(t, uploadedFileToken, "file token should be set from upload step")

		downloadDir := filepath.Join(testDir, "download-"+suffix)
		_ = os.MkdirAll(downloadDir, 0755)
		downloadPath := filepath.Join(downloadDir, "downloaded-"+suffix+".txt")

		t.Cleanup(func() {
			os.RemoveAll(downloadDir)
		})

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+download", "--file-token", uploadedFileToken, "--output", downloadPath},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		// Verify downloaded content matches original
		downloadedContent, readErr := os.ReadFile(downloadPath)
		require.NoError(t, readErr, "stdout:\n%s", result.Stdout)
		assert.Equal(t, testContent, string(downloadedContent))
	})
}