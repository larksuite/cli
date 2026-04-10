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

// testFileDir is the directory for test files (relative path from project root).
const testFileDir = "tests/cli_e2e/drive/testfiles"

// createTempFile creates a temporary file with given content and returns its relative path.
func createTempFile(t *testing.T, suffix, content string) string {
	t.Helper()

	// Create files in a relative path within the project directory
	// since --file requires relative paths
	testDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles")
	_ = os.MkdirAll(testDir, 0755)

	fileName := suffix + "-" + time.Now().UTC().Format("20060102-150405") + ".txt"
	filePath := filepath.Join(testDir, fileName)
	err := os.WriteFile(filePath, []byte(content), 0644)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(filePath)
	})

	return filePath
}

// uploadTestFile uploads a test file and returns the file token.
// The uploaded file is registered for cleanup via parentT.Cleanup.
func uploadTestFile(t *testing.T, parentT *testing.T, ctx context.Context, suffix string) string {
	t.Helper()

	content := "lark-cli-e2e-drive-" + suffix + "-" + time.Now().UTC().Format("20060102-150405")
	filePath := createTempFile(t, suffix, content)

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"drive", "+upload", "--file", filePath},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	fileToken := gjson.Get(result.Stdout, "data.file_token").String()
	require.NotEmpty(t, fileToken, "stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:   []string{"drive", "files", "delete"},
			Params: map[string]any{"file_token": fileToken, "type": "file"},
		})
	})

	return fileToken
}

// importTestDoc imports a markdown file as docx and returns the doc token.
// The imported document is registered for cleanup via parentT.Cleanup.
func importTestDoc(t *testing.T, parentT *testing.T, ctx context.Context, suffix, content string) string {
	t.Helper()

	testDir := filepath.Join("tests", "cli_e2e", "drive", "testfiles")
	_ = os.MkdirAll(testDir, 0755)

	fileName := "drive-e2e-" + suffix + "-" + time.Now().UTC().Format("20060102-150405") + ".md"
	mdFile := filepath.Join(testDir, fileName)
	err := os.WriteFile(mdFile, []byte(content), 0644)
	require.NoError(t, err)

	t.Cleanup(func() {
		os.Remove(mdFile)
	})

	result, err := clie2e.RunCmd(ctx, clie2e.Request{
		Args: []string{"drive", "+import", "--file", mdFile, "--type", "docx"},
	})
	require.NoError(t, err)
	result.AssertExitCode(t, 0)

	ticket := gjson.Get(result.Stdout, "data.ticket").String()
	docToken := gjson.Get(result.Stdout, "data.token").String()

	if ticket != "" {
		// Poll for import completion
		pollResult, pollErr := clie2e.RunCmd(ctx, clie2e.Request{
			Args: []string{"drive", "+task_result", "--ticket", ticket, "--scenario", "import"},
		})
		require.NoError(t, pollErr)
		pollResult.AssertExitCode(t, 0)
		pollResult.AssertStdoutStatus(t, true)
		docToken = gjson.Get(pollResult.Stdout, "data.token").String()
	}

	require.NotEmpty(t, docToken, "doc_token is required, stdout:\n%s", result.Stdout)

	parentT.Cleanup(func() {
		clie2e.RunCmd(context.Background(), clie2e.Request{
			Args:   []string{"drive", "files", "delete"},
			Params: map[string]any{"file_token": docToken, "type": "docx"},
		})
	})

	return docToken
}
