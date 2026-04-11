// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"context"
	"testing"
	"time"

	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// TestDrive_FilesCopyWorkflow tests the files.copy resource command.
// Workflow: upload a file -> copy the file to a new name -> verify copy exists.
func TestDrive_FilesCopyWorkflow(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")

	// Step 1: Upload a file to copy
	t.Run("upload file", func(t *testing.T) {
		content := "lark-cli-e2e-files-copy-" + suffix
		filePath := createTempFile(t, "copy-source", content)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+upload", "--file", filePath},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		sourceToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, sourceToken, "file_token should be returned, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": sourceToken},
			})
		})
	})

	// Get folder token (root folder)
	var folderToken string
	t.Run("get root folder token", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "list"},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// Root folder token is typically in the response
		folderToken = gjson.Get(result.Stdout, "data.folder_token").String()
	})

	// Step 2: Copy the file
	var copiedToken string
	t.Run("copy file", func(t *testing.T) {
		// Use the file uploaded in first step
		content := "lark-cli-e2e-files-copy-" + suffix
		filePath := createTempFile(t, "copy-source", content)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+upload", "--file", filePath},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		sourceToken := gjson.Get(result.Stdout, "data.file_token").String()
		require.NotEmpty(t, sourceToken, "file_token should be returned, stdout:\n%s", result.Stdout)

		copiedName := "lark-cli-e2e-copy-" + suffix + ".txt"

		result, err = clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "copy"},
			Params: map[string]any{
				"file_token": sourceToken,
			},
			Data: map[string]any{
				"name":        copiedName,
				"folder_token": folderToken,
				"type":        "file",
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		copiedToken = gjson.Get(result.Stdout, "data.file.token").String()
		require.NotEmpty(t, copiedToken, "copied file token should be returned, stdout:\n%s", result.Stdout)

		parentT.Cleanup(func() {
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": sourceToken},
			})
			clie2e.RunCmd(context.Background(), clie2e.Request{
				Args:   []string{"drive", "files", "delete"},
				Params: map[string]any{"file_token": copiedToken},
			})
		})
	})

	// Step 3: Verify copied file exists in folder
	t.Run("verify copy exists", func(t *testing.T) {
		require.NotEmpty(t, copiedToken)

		result, err := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "files", "list"},
			Params: map[string]any{
				"folder_token": folderToken,
			},
		})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)

		// Find the copied file in the listing
		files := gjson.Get(result.Stdout, "data.files")
		require.True(t, files.IsArray(), "files should be an array")

		found := false
		for _, file := range files.Array() {
			token := gjson.Get(file.Raw, "token").String()
			if token == copiedToken {
				found = true
				break
			}
		}
		require.True(t, found, "copied file should exist in folder, stdout:\n%s", result.Stdout)
	})
}