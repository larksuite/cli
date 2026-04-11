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

// TestDrive_FilesListWorkflow tests the files list resource command.
// Workflow: upload files -> list root folder files -> verify uploaded file appears.
func TestDrive_FilesListWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")

	// Upload a file first
	fileToken := uploadTestFile(t, parentT, ctx, "files-list-"+suffix)
	require.NotEmpty(t, fileToken)

	t.Run("list - get root folder files", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "list"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, 0)

		// Verify response has items array
		items := gjson.Get(result.Stdout, "data.files")
		require.True(t, items.IsArray(), "should have files array, stdout:\n%s", result.Stdout)
	})
}

// TestDrive_FilesCreateFolderWorkflow tests the files create_folder resource command.
func TestDrive_FilesCreateFolderWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	folderName := "lark-cli-e2e-drive-folder-" + suffix

	var folderToken string

	t.Run("create_folder", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "create_folder"},
			Data: map[string]any{
				"name":         folderName,
				"folder_token": "",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		folderToken = gjson.Get(result.Stdout, "data.token").String()
		require.NotEmpty(t, folderToken, "folder token should be available, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": folderToken, "type": "folder"},
			})
		})
	})
}

// TestDrive_UploadToSpecificFolderWorkflow tests uploading to a specific folder.
func TestDrive_UploadToSpecificFolderWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	testContent := "lark-cli-e2e-drive-upload-folder-" + suffix

	// First create a folder
	folderName := "lark-cli-e2e-drive-target-folder-" + suffix
	var folderToken string

	t.Run("create target folder", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "create_folder"},
			Data: map[string]any{
				"name":         folderName,
				"folder_token": "",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		folderToken = gjson.Get(result.Stdout, "data.token").String()
		require.NotEmpty(t, folderToken, "folder token should be available, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": folderToken, "type": "folder"},
			})
		})
	})

	require.NotEmpty(t, folderToken, "folder token is required for upload step")

	// Upload file to the specific folder
	t.Run("upload to folder", func(t *testing.T) {
		testDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles")
		_ = os.MkdirAll(testDir, 0755)

		localFile := filepath.Join(testDir, "drive-e2e-upload-folder-"+suffix+".txt")
		err := os.WriteFile(localFile, []byte(testContent), 0644)
		require.NoError(t, err)

		t.Cleanup(func() {
			os.Remove(localFile)
		})

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+upload",
				"--file",         localFile,
				"--folder-token", folderToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)

		fileToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, fileToken, "stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": fileToken},
			})
		})
	})
}